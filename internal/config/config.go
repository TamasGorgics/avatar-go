package config

import (
	"os"
	"strconv"
)

type Config struct {
	Addr             string
	LLMBaseURL       string
	APIKey           string
	Model            string
	DailyCap         int
	PerIPHourly      int
	PerIPMinute      int
	PerIPDailyTokens int
	GlobalMinute     int
	PerIPConcurrent  int
	GlobalConcurrent int
	TrustProxy       bool
	LogFormat        string
	LogLevel         string
}

func Load() Config {
	return Config{
		Addr:             env("ADDR", ":8080"),
		LLMBaseURL:       env("LLM_BASE_URL", "https://api.openai.com/v1"),
		APIKey:           env("OPENAI_API_KEY", os.Getenv("LLM_API_KEY")),
		Model:            env("LLM_MODEL", "gpt-4o-mini"),
		DailyCap:         envInt("DAILY_CAP", 300),
		PerIPHourly:      envInt("PER_IP_HOURLY", 20),
		PerIPMinute:      envInt("PER_IP_MINUTE", 3),
		PerIPDailyTokens: envInt("PER_IP_DAILY_TOKENS", 20_000),
		GlobalMinute:     envInt("GLOBAL_MINUTE", 20),
		PerIPConcurrent:  envInt("PER_IP_CONCURRENT", 1),
		GlobalConcurrent: envInt("GLOBAL_CONCURRENT", 4),
		TrustProxy:       os.Getenv("TRUST_PROXY") == "1",
		LogFormat:        env("LOG_FORMAT", "auto"),
		LogLevel:         env("LOG_LEVEL", "info"),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	n, err := strconv.Atoi(os.Getenv(key))
	if err != nil || n < 0 {
		return def
	}
	return n
}
