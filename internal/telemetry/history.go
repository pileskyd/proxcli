package telemetry

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Tokens struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	Reasoning  int64 `json:"reasoning"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
	Total      int64 `json:"total"`
}
type Usage struct {
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	AuthID   string    `json:"authId"`
	Provider string    `json:"provider"`
	Model    string    `json:"model"`
	Session  string    `json:"session,omitempty"`
	Failed   bool      `json:"failed"`
	Status   int       `json:"status,omitempty"`
	Tokens   Tokens    `json:"tokens"`
}
type Decision struct {
	At             time.Time `json:"at"`
	Session        string    `json:"session"`
	AuthID         string    `json:"authId"`
	PreviousAuthID string    `json:"previousAuthId,omitempty"`
	Provider       string    `json:"provider"`
	Model          string    `json:"model"`
	Reason         string    `json:"reason"`
}
type journalRecord struct {
	Kind     string    `json:"kind"`
	Usage    *Usage    `json:"usage,omitempty"`
	Quota    *Account  `json:"quota,omitempty"`
	Decision *Decision `json:"decision,omitempty"`
}
type Journal struct {
	dir string
	mu  sync.Mutex
}

func OpenJournal(dir string) (*Journal, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	return &Journal{dir: dir}, nil
}
func SessionHash(namespace, session string) string {
	if session == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(namespace + "\x00" + session))
	return hex.EncodeToString(sum[:])
}
func (j *Journal) append(kind string, at time.Time, record journalRecord) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	raw, e := json.Marshal(record)
	if e != nil {
		return e
	}
	raw = append(raw, '\n')
	f, e := os.OpenFile(filepath.Join(j.dir, kind+"-"+at.UTC().Format("2006-01-02")+".jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	return e
}
func (j *Journal) AddUsage(u Usage) error {
	if u.ID == "" || u.AuthID == "" || u.At.IsZero() {
		return errors.New("usage identity and timestamp required")
	}
	// Clamp malformed counters; never interpret missing values as negative consumption.
	counts := []*int64{&u.Tokens.Input, &u.Tokens.Output, &u.Tokens.Reasoning, &u.Tokens.CacheRead, &u.Tokens.CacheWrite, &u.Tokens.Total}
	for _, n := range counts {
		if *n < 0 {
			*n = 0
		}
	}
	return j.append("usage", u.At, journalRecord{Kind: "usage", Usage: &u})
}
func (j *Journal) AddQuota(a Account) error {
	return j.append("quota", a.ObservedAt, journalRecord{Kind: "quota", Quota: &a})
}
func (j *Journal) AddDecision(d Decision) error {
	return j.append("routing", d.At, journalRecord{Kind: "routing", Decision: &d})
}
func AtomicJSON(path string, value any) error {
	raw, e := json.Marshal(value)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".telemetry-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(raw)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
func ReadSnapshot(path string) (Snapshot, error) {
	var s Snapshot
	f, e := os.Open(path)
	if e != nil {
		return s, e
	}
	defer f.Close()
	e = json.NewDecoder(io.LimitReader(f, 4<<20)).Decode(&s)
	if e == nil && s.Version != 1 {
		e = errors.New("unsupported quota snapshot version")
	}
	return s, e
}
func (j *Journal) Prune(now time.Time) error {
	entries, e := os.ReadDir(j.dir)
	if e != nil {
		return e
	}
	cutoff := now.UTC().AddDate(0, 0, -30).Format("2006-01-02")
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		for _, kind := range []string{"usage", "quota", "routing"} {
			prefix := kind + "-"
			if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".jsonl") {
				continue
			}
			date := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".jsonl")
			if _, e = time.Parse("2006-01-02", date); e == nil && date < cutoff {
				if e = os.Remove(filepath.Join(j.dir, name)); e != nil {
					return e
				}
			}
		}
	}
	return nil
}

type Bucket struct {
	At       time.Time `json:"at"`
	Requests int64     `json:"requests"`
	Failed   int64     `json:"failed"`
	Tokens   Tokens    `json:"tokens"`
}
type History struct {
	From      time.Time  `json:"from"`
	To        time.Time  `json:"to"`
	Buckets   []Bucket   `json:"buckets"`
	Quotas    []Account  `json:"quotas"`
	Decisions []Decision `json:"decisions"`
	Truncated bool       `json:"truncated"`
}

func addTokens(a *Tokens, b Tokens) {
	a.Input += b.Input
	a.Output += b.Output
	a.Reasoning += b.Reasoning
	a.CacheRead += b.CacheRead
	a.CacheWrite += b.CacheWrite
	a.Total += b.Total
}

// History scans bounded local journals. It does not consume the upstream usage queue.
func (j *Journal) History(from, to time.Time, provider, authID string) (History, error) {
	h := History{From: from, To: to, Buckets: []Bucket{}, Quotas: []Account{}, Decisions: []Decision{}}
	if to.Before(from) || to.Sub(from) > 31*24*time.Hour {
		return h, errors.New("history range must be within 31 days")
	}
	buckets := map[int64]*Bucket{}
	seen := map[string]bool{}
	readCount := 0
	matches := func(p, id string) bool {
		return (provider == "" || CanonicalProvider(provider) == CanonicalProvider(p)) && (authID == "" || authID == id)
	}
	for day := time.Date(from.UTC().Year(), from.UTC().Month(), from.UTC().Day(), 0, 0, 0, 0, time.UTC); !day.After(to); day = day.AddDate(0, 0, 1) {
		for _, kind := range []string{"usage", "quota", "routing"} {
			f, e := os.Open(filepath.Join(j.dir, kind+"-"+day.Format("2006-01-02")+".jsonl"))
			if errors.Is(e, os.ErrNotExist) {
				continue
			}
			if e != nil {
				return h, e
			}
			reader := bufio.NewReaderSize(f, 64<<10)
			for {
				raw, e := reader.ReadBytes('\n')
				if e == io.EOF {
					break
				}
				if e != nil {
					f.Close()
					return h, e
				}
				readCount++
				if readCount > 500000 {
					h.Truncated = true
					break
				}
				if len(raw) > 16384 {
					continue
				}
				var r journalRecord
				if json.Unmarshal(raw, &r) != nil {
					continue
				}
				if u := r.Usage; u != nil && u.At.After(from) && !u.At.After(to) && matches(u.Provider, u.AuthID) && !seen[u.ID] {
					seen[u.ID] = true
					key := u.At.Truncate(time.Hour).Unix()
					b := buckets[key]
					if b == nil {
						b = &Bucket{At: time.Unix(key, 0).UTC()}
						buckets[key] = b
					}
					b.Requests++
					if u.Failed {
						b.Failed++
					}
					addTokens(&b.Tokens, u.Tokens)
				}
				if a := r.Quota; a != nil && !a.ObservedAt.Before(from) && !a.ObservedAt.After(to) && matches(a.Provider, a.AuthID) {
					if len(h.Quotas) < 20000 {
						h.Quotas = append(h.Quotas, *a)
					} else {
						h.Truncated = true
					}
				}
				if d := r.Decision; d != nil && !d.At.Before(from) && !d.At.After(to) && matches(d.Provider, d.AuthID) {
					h.Decisions = append(h.Decisions, *d)
					if len(h.Decisions) > 200 {
						h.Decisions = h.Decisions[len(h.Decisions)-200:]
					}
				}
			}
			f.Close()
			if readCount > 500000 {
				return h, fmt.Errorf("history exceeds scan limit; request a shorter range")
			}
		}
	}
	for _, b := range buckets {
		h.Buckets = append(h.Buckets, *b)
	}
	sort.Slice(h.Buckets, func(i, j int) bool { return h.Buckets[i].At.Before(h.Buckets[j].At) })
	sort.Slice(h.Quotas, func(i, j int) bool { return h.Quotas[i].ObservedAt.Before(h.Quotas[j].ObservedAt) })
	sort.Slice(h.Decisions, func(i, j int) bool { return h.Decisions[i].At.Before(h.Decisions[j].At) })
	return h, nil
}
