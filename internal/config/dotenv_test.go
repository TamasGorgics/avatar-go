package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	return path
}

func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		old, ok := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		t.Cleanup(func() {
			if ok {
				_ = os.Setenv(key, old)
				return
			}
			_ = os.Unsetenv(key)
		})
	}
}

func TestLoadDotEnv(t *testing.T) {
	keys := []string{
		"AVATAR_DOTENV_PLAIN",
		"AVATAR_DOTENV_SPACED",
		"AVATAR_DOTENV_EXPORTED",
		"AVATAR_DOTENV_TABBED",
		"AVATAR_DOTENV_SINGLE",
		"AVATAR_DOTENV_INLINE",
		"AVATAR_DOTENV_EMPTY",
		"AVATAR_DOTENV_ESCAPED",
		"AVATAR_DOTENV_HASH",
		"AVATAR_DOTENV_QUOTED_COMMENT",
		"AVATAR_DOTENV_DUP",
	}
	unsetEnv(t, keys...)

	path := writeEnvFile(t, `
# comment
AVATAR_DOTENV_PLAIN=plain
AVATAR_DOTENV_SPACED =  value with spaces  
export AVATAR_DOTENV_EXPORTED="exported value"
export	AVATAR_DOTENV_TABBED=tabbed
AVATAR_DOTENV_SINGLE='single # value'
AVATAR_DOTENV_INLINE=before # trailing comment
AVATAR_DOTENV_EMPTY=
AVATAR_DOTENV_ESCAPED="line\nbreak"
AVATAR_DOTENV_HASH=foo#bar
AVATAR_DOTENV_QUOTED_COMMENT="value" # trailing comment
AVATAR_DOTENV_DUP=one
AVATAR_DOTENV_DUP=two
`)

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}

	want := map[string]string{
		"AVATAR_DOTENV_PLAIN":          "plain",
		"AVATAR_DOTENV_SPACED":         "value with spaces",
		"AVATAR_DOTENV_EXPORTED":       "exported value",
		"AVATAR_DOTENV_TABBED":         "tabbed",
		"AVATAR_DOTENV_SINGLE":         "single # value",
		"AVATAR_DOTENV_INLINE":         "before",
		"AVATAR_DOTENV_EMPTY":          "",
		"AVATAR_DOTENV_ESCAPED":        "line\nbreak",
		"AVATAR_DOTENV_HASH":           "foo#bar",
		"AVATAR_DOTENV_QUOTED_COMMENT": "value",
		"AVATAR_DOTENV_DUP":            "two",
	}
	for key, wantValue := range want {
		got, ok := os.LookupEnv(key)
		if !ok || got != wantValue {
			t.Errorf("%s = %q, %v; want %q, true", key, got, ok, wantValue)
		}
	}
}

func TestLoadDotEnvKeepsExistingEnv(t *testing.T) {
	t.Setenv("AVATAR_DOTENV_EXISTING", "real")
	path := writeEnvFile(t, "AVATAR_DOTENV_EXISTING=file\n")

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if got := os.Getenv("AVATAR_DOTENV_EXISTING"); got != "real" {
		t.Fatalf("existing env was overridden: %q", got)
	}
}

func TestLoadDotEnvMissingFile(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), ".env")); err != nil {
		t.Fatalf("missing file returned error: %v", err)
	}
}

func TestLoadDotEnvParseErrorDoesNotApply(t *testing.T) {
	const key = "AVATAR_DOTENV_PARTIAL"
	unsetEnv(t, key)
	path := writeEnvFile(t, key+"=value\nnot a valid line\n")

	if err := LoadDotEnv(path); err == nil {
		t.Fatal("LoadDotEnv returned nil for invalid syntax")
	}
	if _, ok := os.LookupEnv(key); ok {
		t.Fatal("values were applied before the parse error")
	}
}

func TestLoadDotEnvRejectsInvalidSyntax(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "missing equals", content: "AVATAR_DOTENV_BAD\n"},
		{name: "invalid key", content: "1AVATAR_DOTENV_BAD=x\n"},
		{name: "unterminated double quote", content: `AVATAR_DOTENV_BAD="x` + "\n"},
		{name: "unterminated single quote", content: "AVATAR_DOTENV_BAD='x\n"},
		{name: "trailing after double quote", content: `AVATAR_DOTENV_BAD="x" y` + "\n"},
		{name: "trailing after single quote", content: "AVATAR_DOTENV_BAD='x' y\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := LoadDotEnv(writeEnvFile(t, tc.content)); err == nil {
				t.Fatal("LoadDotEnv returned nil, want an error")
			}
		})
	}
}

func TestLoadDotEnvBOMAndCRLF(t *testing.T) {
	const key = "AVATAR_DOTENV_CRLF"
	unsetEnv(t, key)
	path := writeEnvFile(t, "\ufeff# comment\r\n"+key+"=crlf\r\n\r\n")

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if got := os.Getenv(key); got != "crlf" {
		t.Fatalf("%s = %q, want crlf", key, got)
	}
}

func TestLoadDotEnvReadError(t *testing.T) {
	if err := LoadDotEnv(t.TempDir()); err == nil {
		t.Fatal("LoadDotEnv returned nil for a directory")
	}
}

func TestExampleDotEnvIsValid(t *testing.T) {
	data, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	values, err := parseDotEnv(string(data))
	if err != nil {
		t.Fatalf("parse .env.example: %v", err)
	}

	keys := []string{
		"ADDR",
		"LLM_BASE_URL",
		"OPENAI_API_KEY",
		"LLM_API_KEY",
		"LLM_MODEL",
		"DAILY_CAP",
		"PER_IP_HOURLY",
		"TRUST_PROXY",
		"LOG_FORMAT",
		"LOG_LEVEL",
	}
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			t.Errorf(".env.example is missing %s", key)
			continue
		}
		if value != "" {
			t.Errorf(".env.example sets %s=%q, want no value", key, value)
		}
	}
}
