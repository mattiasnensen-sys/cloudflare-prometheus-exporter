package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime configuration for the exporter.
type Config struct {
	CloudflareAPIToken    string
	CloudflareZoneTags    []string
	CloudflareAccountTags []string
	CloudflareGraphQL     string
	PollInterval          time.Duration
	WindowDuration        time.Duration
	ScrapeDelay           time.Duration
	RequestTimeout        time.Duration
	QueryLimit            int
	Port                  int
	MetricsPath           string
	HealthPath            string
}

// FromEnv loads configuration from environment variables.
func FromEnv() (Config, error) {
	cfg := Config{
		CloudflareGraphQL: getEnv("CLOUDFLARE_GRAPHQL_ENDPOINT", "https://api.cloudflare.com/client/v4/graphql"),
		PollInterval:      getDurationEnv("POLL_INTERVAL", 60*time.Second),
		WindowDuration:    getDurationEnv("WINDOW_DURATION", 60*time.Second),
		ScrapeDelay:       getDurationEnv("SCRAPE_DELAY", 120*time.Second),
		RequestTimeout:    getDurationEnv("REQUEST_TIMEOUT", 20*time.Second),
		QueryLimit:        getIntEnv("QUERY_LIMIT", 10000),
		Port:              getIntEnv("PORT", 9103),
		MetricsPath:       getEnv("METRICS_PATH", "/metrics"),
		HealthPath:        getEnv("HEALTH_PATH", "/healthz"),
	}

	cfg.CloudflareAPIToken = strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN"))
	if cfg.CloudflareAPIToken == "" {
		return Config{}, fmt.Errorf("CLOUDFLARE_API_TOKEN is required")
	}

	zoneTags := strings.Split(getEnv("CLOUDFLARE_ZONE_TAGS", ""), ",")
	for _, tag := range zoneTags {
		t := strings.TrimSpace(tag)
		if t != "" {
			cfg.CloudflareZoneTags = append(cfg.CloudflareZoneTags, t)
		}
	}
	if len(cfg.CloudflareZoneTags) == 0 {
		return Config{}, fmt.Errorf("CLOUDFLARE_ZONE_TAGS is required")
	}

	accountTags := strings.Split(getEnv("CLOUDFLARE_ACCOUNT_TAGS", ""), ",")
	for _, tag := range accountTags {
		t := strings.TrimSpace(tag)
		if t != "" {
			cfg.CloudflareAccountTags = append(cfg.CloudflareAccountTags, t)
		}
	}
	if len(cfg.CloudflareAccountTags) == 0 {
		if accountTag := strings.TrimSpace(getEnv("CLOUDFLARE_ACCOUNT_ID", "")); accountTag != "" {
			cfg.CloudflareAccountTags = append(cfg.CloudflareAccountTags, accountTag)
		}
	}

	if cfg.QueryLimit <= 0 {
		return Config{}, fmt.Errorf("QUERY_LIMIT must be > 0")
	}
	if cfg.PollInterval <= 0 {
		return Config{}, fmt.Errorf("POLL_INTERVAL must be > 0")
	}
	if cfg.WindowDuration <= 0 {
		return Config{}, fmt.Errorf("WINDOW_DURATION must be > 0")
	}
	if cfg.RequestTimeout <= 0 {
		return Config{}, fmt.Errorf("REQUEST_TIMEOUT must be > 0")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return Config{}, fmt.Errorf("PORT must be 1..65535")
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func getIntEnv(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getDurationEnv(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
