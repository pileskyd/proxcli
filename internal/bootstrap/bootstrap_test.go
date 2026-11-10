package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitDoesNotOverwriteAndDoesNotUsePlaceholderKeys(t *testing.T) {
	dir := t.TempDir()
	if e := Init(dir); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if !strings.Contains(string(before), "PROXCLI_MANAGEMENT_KEY=") {
		t.Fatal("missing key")
	}
	if strings.Contains(string(before), "REPLACE") {
		t.Fatal("placeholder key")
	}
	if e := Init(dir); e == nil {
		t.Fatal("existing config overwritten")
	}
	after, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if string(before) != string(after) {
		t.Fatal("existing env changed")
	}
}
func TestLoadEnvDoesNotOverrideOrExecuteAnything(t *testing.T) {
	t.Setenv("PROXCLI_UPSTREAM", "existing")
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	_ = os.WriteFile(p, []byte("PROXCLI_UPSTREAM=replace\nPROXCLI_TEST_LITERAL='$(echo secret)'\nPATH=not-an-environment-loader\n"), 0600)
	oldPath := os.Getenv("PATH")
	if e := LoadEnv(p); e != nil {
		t.Fatal(e)
	}
	defer os.Unsetenv("PROXCLI_TEST_LITERAL")
	if os.Getenv("PROXCLI_UPSTREAM") != "existing" || os.Getenv("PROXCLI_TEST_LITERAL") != "$(echo secret)" || os.Getenv("PATH") != oldPath {
		t.Fatal("env loader must be literal and scoped")
	}
}
