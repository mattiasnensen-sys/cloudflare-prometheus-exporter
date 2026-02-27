package config

import (
	"reflect"
	"testing"
)

func TestFromEnvParsesAccountTags(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "token")
	t.Setenv("CLOUDFLARE_ZONE_TAGS", "zone-1")
	t.Setenv("CLOUDFLARE_ACCOUNT_TAGS", "acc-1, acc-2")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv returned error: %v", err)
	}

	want := []string{"acc-1", "acc-2"}
	if !reflect.DeepEqual(cfg.CloudflareAccountTags, want) {
		t.Fatalf("unexpected account tags: got %v want %v", cfg.CloudflareAccountTags, want)
	}
}

func TestFromEnvFallsBackToSingleAccountID(t *testing.T) {
	t.Setenv("CLOUDFLARE_API_TOKEN", "token")
	t.Setenv("CLOUDFLARE_ZONE_TAGS", "zone-1")
	t.Setenv("CLOUDFLARE_ACCOUNT_TAGS", "")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "acc-1")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv returned error: %v", err)
	}

	want := []string{"acc-1"}
	if !reflect.DeepEqual(cfg.CloudflareAccountTags, want) {
		t.Fatalf("unexpected account tags: got %v want %v", cfg.CloudflareAccountTags, want)
	}
}
