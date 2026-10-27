package config

import "testing"

var configEnv = []string{
	"ADDR",
	"LLM_BASE_URL",
	"OPENAI_API_KEY",
	"LLM_API_KEY",
	"LLM_MODEL",
	"DAILY_CAP",
	"PER_IP_HOURLY",
	"TRUST_PROXY",
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range configEnv {
		t.Setenv(key, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)

	want := Config{
		Addr:        ":8080",
		LLMBaseURL:  "https://api.openai.com/v1",
		Model:       "gpt-4o-mini",
		DailyCap:    300,
		PerIPHourly: 20,
	}
	if got := Load(); got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("ADDR", ":9999")
	t.Setenv("LLM_BASE_URL", "http://localhost:11434/v1")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("LLM_API_KEY", "llm-key")
	t.Setenv("LLM_MODEL", "custom-model")
	t.Setenv("DAILY_CAP", "7")
	t.Setenv("PER_IP_HOURLY", "3")
	t.Setenv("TRUST_PROXY", "1")

	want := Config{
		Addr:        ":9999",
		LLMBaseURL:  "http://localhost:11434/v1",
		APIKey:      "openai-key",
		Model:       "custom-model",
		DailyCap:    7,
		PerIPHourly: 3,
		TrustProxy:  true,
	}
	if got := Load(); got != want {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadFallsBackToLLMAPIKey(t *testing.T) {
	clearEnv(t)
	t.Setenv("LLM_API_KEY", "llm-key")

	if got := Load().APIKey; got != "llm-key" {
		t.Fatalf("APIKey = %q, want %q", got, "llm-key")
	}
}

func TestLoadInvalidCapsUseDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("DAILY_CAP", "many")
	t.Setenv("PER_IP_HOURLY", "1.5")

	got := Load()
	if got.DailyCap != 300 || got.PerIPHourly != 20 {
		t.Fatalf("caps = (%d, %d), want (300, 20)", got.DailyCap, got.PerIPHourly)
	}
}

func TestLoadAcceptsZeroCaps(t *testing.T) {
	clearEnv(t)
	t.Setenv("DAILY_CAP", "0")
	t.Setenv("PER_IP_HOURLY", "0")

	got := Load()
	if got.DailyCap != 0 || got.PerIPHourly != 0 {
		t.Fatalf("caps = (%d, %d), want (0, 0)", got.DailyCap, got.PerIPHourly)
	}
}
