package config

import (
	"os"
	"strconv"
)

type Config struct {
	Addr        string
	LLMBaseURL  string
	APIKey      string
	Model       string
	DailyCap    int
	PerIPHourly int
	TrustProxy  bool
	LogFormat   string
	LogLevel    string
}

func Load() Config {
	return Config{
		Addr:        env("ADDR", ":8080"),
		LLMBaseURL:  env("LLM_BASE_URL", "https://api.openai.com/v1"),
		APIKey:      env("OPENAI_API_KEY", os.Getenv("LLM_API_KEY")),
		Model:       env("LLM_MODEL", "gpt-4o-mini"),
		DailyCap:    envInt("DAILY_CAP", 300),
		PerIPHourly: envInt("PER_IP_HOURLY", 20),
		TrustProxy:  os.Getenv("TRUST_PROXY") == "1",
		LogFormat:   env("LOG_FORMAT", "auto"),
		LogLevel:    env("LOG_LEVEL", "info"),
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
	if err != nil {
		return def
	}
	return n
}
