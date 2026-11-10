package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	void* call;
	void* free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);
*/
import "C"

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"proxcli/internal/telemetry"
)

const abiVersion = 1

var lifecycle sync.Mutex
var active *workerState
var dropped atomic.Uint64
var writeErrors atomic.Uint64
var lastHandled atomic.Int64

type workerState struct {
	queue    chan event
	done     chan struct{}
	finished chan struct{}
	journal  *telemetry.Journal
	dataDir  string
}

type event struct {
	Usage telemetry.Usage
	Quota *telemetry.Account
}
type usageInput struct {
	RequestID       string
	AuthID          string
	Provider        string
	Model           string
	SessionID       string
	APIKey          string
	RequestedAt     time.Time
	Latency         time.Duration
	Failed          bool
	Failure         struct{ StatusCode int }
	Detail          struct{ InputTokens, OutputTokens, ReasoningTokens, CacheReadTokens, CacheCreationTokens, CachedTokens, TotalTokens int64 }
	ResponseHeaders map[string][]string
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(_ *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	plugin.abi_version = abiVersion
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response == nil {
		return 1
	}
	response.ptr = nil
	response.len = 0
	if method == nil || uint64(requestLen) > 8<<20 {
		return writeError(response)
	}
	var raw []byte
	if request != nil && requestLen > 0 {
		raw = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	result, e := handle(C.GoString(method), raw)
	if e != nil {
		return writeError(response)
	}
	encoded, _ := json.Marshal(map[string]any{"ok": true, "result": result})
	response.ptr = C.CBytes(encoded)
	response.len = C.size_t(len(encoded))
	return 0
}
func writeError(response *C.cliproxy_buffer) C.int {
	raw := []byte(`{"ok":false,"error":{"code":"proxcli_usage_error","message":"ProxCli telemetry could not handle this event"}}`)
	response.ptr = C.CBytes(raw)
	response.len = C.size_t(len(raw))
	return 1
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {
	lifecycle.Lock()
	defer lifecycle.Unlock()
	if active == nil {
		return
	}
	close(active.done)
	<-active.finished
	active = nil
}

func handle(method string, raw []byte) (any, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		lifecycle.Lock()
		if active == nil {
			dir := os.Getenv("PROXCLI_DATA_DIR")
			if dir == "" {
				dir = ".data"
			}
			dir = filepath.Join(dir, "telemetry")
			journal, e := telemetry.OpenJournal(dir)
			if e != nil {
				lifecycle.Unlock()
				return nil, e
			}
			active = &workerState{queue: make(chan event, 4096), done: make(chan struct{}), finished: make(chan struct{}), journal: journal, dataDir: dir}
			go worker(active)
		}
		lifecycle.Unlock()
		return map[string]any{"schema_version": 1, "metadata": map[string]any{"Name": "proxcli-usage", "Version": "0.2.0", "Author": "ProxCli"}, "capabilities": map[string]bool{"usage_plugin": true}}, nil
	case "usage.handle":
		var input usageInput
		if e := json.Unmarshal(raw, &input); e != nil {
			return nil, e
		}
		u := sanitize(input)
		if u.ID == "" || u.AuthID == "" {
			return map[string]bool{"accepted": false}, nil
		}
		ev := event{Usage: u}
		signals := map[string]string{}
		for k, values := range input.ResponseHeaders {
			if len(values) > 0 {
				signals[k] = values[0]
			}
		}
		windows := telemetry.FromHeaders(input.Provider, signals)
		if len(windows) > 0 {
			ev.Quota = &telemetry.Account{AuthID: input.AuthID, Provider: telemetry.CanonicalProvider(input.Provider), ObservedAt: time.Now().UTC(), Source: "usage-observer", Windows: windows}
		}
		// Raw API keys, error bodies, headers and prompt contents never enter the queue.
		lifecycle.Lock()
		defer lifecycle.Unlock()
		if active == nil {
			return map[string]bool{"accepted": false}, nil
		}
		select {
		case active.queue <- ev:
		default:
			dropped.Add(1)
		}
		lastHandled.Store(time.Now().Unix())
		return map[string]bool{"accepted": true}, nil
	default:
		return map[string]any{}, nil
	}
}
func sanitize(in usageInput) telemetry.Usage {
	cached := in.Detail.CacheReadTokens
	if cached < 0 {
		cached = 0
	}
	provider := telemetry.CanonicalProvider(in.Provider)
	input := in.Detail.InputTokens
	if provider == "codex" {
		if cached == 0 {
			cached = in.Detail.CachedTokens
			if cached < 0 {
				cached = 0
			}
		}
		input -= cached
		if input < 0 {
			input = 0
		}
	}
	// Claude CachedTokens can represent creation on a cold cache, never use it as a read.

	at := in.RequestedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	// Hash includes provider and client namespace; neither the client key nor session ID is persisted.
	session := telemetry.SessionHash(in.Provider+"\x00"+in.APIKey, in.SessionID)
	return telemetry.Usage{ID: in.RequestID, At: at, AuthID: in.AuthID, Provider: telemetry.CanonicalProvider(in.Provider), Model: in.Model, Session: session, Failed: in.Failed, Status: in.Failure.StatusCode, Tokens: telemetry.Tokens{Input: input, Output: in.Detail.OutputTokens, Reasoning: in.Detail.ReasoningTokens, CacheRead: cached, CacheWrite: in.Detail.CacheCreationTokens, Total: in.Detail.TotalTokens}}
}
func worker(state *workerState) {
	queue, done, finished, journal, dataDir := state.queue, state.done, state.finished, state.journal, state.dataDir
	defer close(finished)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	observed := map[string]string{}
	quotas := map[string]telemetry.Account{}
	persist := func() {
		list := []telemetry.Account{}
		for _, a := range quotas {
			list = append(list, a)
		}
		if e := telemetry.AtomicJSON(filepath.Join(dataDir, "usage-quotas.json"), telemetry.Snapshot{Version: 1, UpdatedAt: time.Now().UTC(), Accounts: list}); e != nil {
			writeErrors.Add(1)
		}
		if e := telemetry.AtomicJSON(filepath.Join(dataDir, "usage-status.json"), map[string]any{"updatedAt": time.Now().UTC(), "dropped": dropped.Load(), "writeErrors": writeErrors.Load(), "queued": len(queue), "lastHandledUnix": lastHandled.Load()}); e != nil {
			writeErrors.Add(1)
		}
	}
	process := func(ev event) {
		if e := journal.AddUsage(ev.Usage); e != nil {
			writeErrors.Add(1)
		}
		if ev.Quota != nil {
			quotas[ev.Quota.AuthID] = *ev.Quota
			if e := journal.AddQuota(*ev.Quota); e != nil {
				writeErrors.Add(1)
			}
		}
		u := ev.Usage
		key := u.Provider + "\x00" + u.Model + "\x00" + u.Session
		if u.Session != "" && !u.Failed {
			if previous := observed[key]; previous != u.AuthID {
				reason := "first-observed-session"
				if previous != "" {
					reason = "observed-account-change"
				}
				if e := journal.AddDecision(telemetry.Decision{At: time.Now().UTC(), Session: u.Session, AuthID: u.AuthID, PreviousAuthID: previous, Provider: u.Provider, Model: u.Model, Reason: reason}); e != nil {
					writeErrors.Add(1)
				}
				observed[key] = u.AuthID
			}
		}
		if len(observed) > 100000 {
			observed = map[string]string{}
		}
	}
	persist()
	for {
		select {
		case ev := <-queue:
			process(ev)
		case <-ticker.C:
			persist()
		case <-done:
			for {
				select {
				case ev := <-queue:
					process(ev)
				default:
					persist()
					return
				}
			}
		}
	}
}
