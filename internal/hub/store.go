package hub

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type route struct {
	Prefix    string `json:"prefix"`
	Mode      string `json:"mode"`
	URL       string `json:"url"`
	Confirmed bool   `json:"confirmed"`
}
type login struct {
	Route    route     `json:"route"`
	Provider string    `json:"provider"`
	Created  time.Time `json:"created"`
}
type state struct {
	Routes map[string]route `json:"routes"`
	Logins map[string]login `json:"logins"`
}
type store struct {
	mu    sync.Mutex
	dir   string
	aead  cipher.AEAD
	state state
}

func openStore(dir string) (*store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	keyFile := filepath.Join(dir, "storage.key")
	key, err := os.ReadFile(keyFile)
	if errors.Is(err, os.ErrNotExist) {
		// Never create a replacement key over existing encrypted state.
		if _, e := os.Stat(filepath.Join(dir, "state.enc")); !errors.Is(e, os.ErrNotExist) {
			return nil, errors.New("storage.key отсутствует при существующем state.enc")
		}
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		f, e := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return nil, e
		}
		_, err = f.Write(key)
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	}
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	s := &store{dir: dir, aead: aead, state: state{Routes: map[string]route{}, Logins: map[string]login{}}}
	b, err := os.ReadFile(filepath.Join(dir, "state.enc"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) < aead.NonceSize() {
		return nil, errors.New("повреждён state.enc")
	}
	plain, err := aead.Open(nil, b[:aead.NonceSize()], b[aead.NonceSize():], []byte("proxcli-state-v1"))
	if err != nil {
		return nil, errors.New("не удалось расшифровать state.enc")
	}
	if err = json.Unmarshal(plain, &s.state); err != nil {
		return nil, err
	}
	if s.state.Routes == nil || s.state.Logins == nil {
		return nil, errors.New("неверный формат state.enc")
	}
	return s, nil
}

// Caller holds mu. Atomic replacement keeps either the old or the new state.
func (s *store) save() error {
	for k, v := range s.state.Logins {
		if time.Since(v.Created) > 15*time.Minute {
			delete(s.state.Logins, k)
		}
	}
	plain, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	encrypted := s.aead.Seal(nonce, nonce, plain, []byte("proxcli-state-v1"))
	f, err := os.CreateTemp(s.dir, ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(encrypted)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(s.dir, "state.enc"))
}
