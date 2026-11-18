package config

import (
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := load(env(map[string]string{"JWT_SECRET": strings.Repeat("s", 32)}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.GRPCAddr != ":50051" ||
		cfg.JWTTTL != time.Hour || cfg.UserCountInterval != 10*time.Second {
		t.Fatalf("unexpected defaults %+v", cfg)
	}
}

func TestLoad_Overrides(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"JWT_SECRET":          strings.Repeat("s", 40),
		"HTTP_ADDR":           ":9000",
		"JWT_TTL":             "15m",
		"USER_COUNT_INTERVAL": "1s",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":9000" || cfg.JWTTTL != 15*time.Minute || cfg.UserCountInterval != time.Second {
		t.Fatalf("unexpected config %+v", cfg)
	}
}

func TestLoad_Errors(t *testing.T) {
	_, err := load(env(map[string]string{
		"JWT_SECRET": "too-short",
		"JWT_TTL":    "forever",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"JWT_SECRET", "JWT_TTL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s: %v", want, err)
		}
	}
}
