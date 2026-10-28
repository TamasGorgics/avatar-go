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
	"PER_IP_MINUTE",
	"GLOBAL_MINUTE",
	"PER_IP_CONCURRENT",
	"GLOBAL_CONCURRENT",
	"TRUST_PROXY",
	"LOG_FORMAT",
	"LOG_LEVEL",
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
		Addr:             ":8080",
		LLMBaseURL:       "https://api.openai.com/v1",
		Model:            "gpt-4o-mini",
		DailyCap:         300,
		PerIPHourly:      20,
		PerIPMinute:      3,
		GlobalMinute:     20,
		PerIPConcurrent:  1,
		GlobalConcurrent: 4,
		LogFormat:        "auto",
		LogLevel:         "info",
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
	t.Setenv("PER_IP_MINUTE", "2")
	t.Setenv("GLOBAL_MINUTE", "9")
	t.Setenv("PER_IP_CONCURRENT", "2")
	t.Setenv("GLOBAL_CONCURRENT", "5")
	t.Setenv("TRUST_PROXY", "1")
	t.Setenv("LOG_FORMAT", "json")
	t.Setenv("LOG_LEVEL", "debug")

	want := Config{
		Addr:             ":9999",
		LLMBaseURL:       "http://localhost:11434/v1",
		APIKey:           "openai-key",
		Model:            "custom-model",
		DailyCap:         7,
		PerIPHourly:      3,
		PerIPMinute:      2,
		GlobalMinute:     9,
		PerIPConcurrent:  2,
		GlobalConcurrent: 5,
		TrustProxy:       true,
		LogFormat:        "json",
		LogLevel:         "debug",
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
	t.Setenv("PER_IP_MINUTE", "-1")
	t.Setenv("GLOBAL_MINUTE", "soon")
	t.Setenv("PER_IP_CONCURRENT", "-2")
	t.Setenv("GLOBAL_CONCURRENT", "lots")

	got := Load()
	if got.DailyCap != 300 || got.PerIPHourly != 20 || got.PerIPMinute != 3 ||
		got.GlobalMinute != 20 || got.PerIPConcurrent != 1 || got.GlobalConcurrent != 4 {
		t.Fatalf("caps = (%d, %d, %d, %d, %d, %d), want (300, 20, 3, 20, 1, 4)",
			got.DailyCap, got.PerIPHourly, got.PerIPMinute, got.GlobalMinute,
			got.PerIPConcurrent, got.GlobalConcurrent)
	}
}

func TestLoadAcceptsZeroCaps(t *testing.T) {
	clearEnv(t)
	t.Setenv("DAILY_CAP", "0")
	t.Setenv("PER_IP_HOURLY", "0")
	t.Setenv("PER_IP_MINUTE", "0")
	t.Setenv("GLOBAL_MINUTE", "0")
	t.Setenv("PER_IP_CONCURRENT", "0")
	t.Setenv("GLOBAL_CONCURRENT", "0")

	got := Load()
	if got.DailyCap != 0 || got.PerIPHourly != 0 || got.PerIPMinute != 0 ||
		got.GlobalMinute != 0 || got.PerIPConcurrent != 0 || got.GlobalConcurrent != 0 {
		t.Fatalf("caps = (%d, %d, %d, %d, %d, %d), want all zero",
			got.DailyCap, got.PerIPHourly, got.PerIPMinute, got.GlobalMinute,
			got.PerIPConcurrent, got.GlobalConcurrent)
	}
}
