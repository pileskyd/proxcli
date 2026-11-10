package bootstrap

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadEnv loads only ProxCli's own settings without overriding process settings.
func LoadEnv(path string) error {
	f, e := os.Open(path)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !strings.HasPrefix(key, "PROXCLI_") {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		if _, set := os.LookupEnv(key); !set {
			if e = os.Setenv(key, value); e != nil {
				return e
			}
		}
	}
	return scanner.Err()
}
func Init(root string) error {
	envPath := filepath.Join(root, ".env")
	configPath := filepath.Join(root, ".data", "cliproxyapi.yaml")
	for _, p := range []string{envPath, configPath} {
		if _, e := os.Stat(p); !errors.Is(e, os.ErrNotExist) {
			if e == nil {
				return fmt.Errorf("%s уже существует; настройки не перезаписаны", filepath.Base(p))
			}
			return e
		}
	}
	key := func() (string, error) { b := make([]byte, 32); _, e := rand.Read(b); return hex.EncodeToString(b), e }
	management, e := key()
	if e != nil {
		return e
	}
	client, e := key()
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(configPath), 0700); e != nil {
		return e
	}
	env := fmt.Sprintf("PROXCLI_UPSTREAM=http://127.0.0.1:8317\nPROXCLI_MANAGEMENT_KEY=%s\nPROXCLI_ACCESS_KEY=%s\nPROXCLI_LISTEN=127.0.0.1:8787\nPROXCLI_DATA_DIR=.data\nPROXCLI_WEB_DIR=web/dist\nPROXCLI_MANAGE_ROUTING=true\n", management, client)
	config := fmt.Sprintf("# ProxCli configuration. Contains secrets; never publish.\nconfig-version: 8\nserver:\n  host: 127.0.0.1\n  port: 8317\nmanagement:\n  allow-remote: false\n  secret-key: %q\n  disable-control-panel: true\naccess:\n  api-keys: [%q]\noauth:\n  auth-dir: \"~/.cli-proxy-api\"\nrouting:\n  strategy: fill-first\n  session-affinity: true\n  session-affinity-ttl: 168h\n  session-affinity-rebind-on: quota-strict\nplugins:\n  enabled: true\n  dir: bin/plugins\n  configs:\n    proxcli-usage:\n      enabled: true\nrequests:\n  proxy-url: \"\"\n", management, client)
	write := func(path, body string) error {
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = f.WriteString(body)
		closeErr := f.Close()
		if e == nil {
			e = closeErr
		}
		return e
	}
	if e = write(envPath, env); e != nil {
		return e
	}
	if e = write(configPath, config); e != nil {
		return e
	}
	return nil
}
