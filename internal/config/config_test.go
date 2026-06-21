package config

import (
	"testing"
)

func TestDefaultConfigActiveGroq(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Active != "groq" {
		t.Errorf("expected Active=='groq', got %q", cfg.Active)
	}
	p := cfg.GetActiveProvider()
	if p == nil {
		t.Fatal("GetActiveProvider() returned nil")
	}
	if p.Name != "groq" {
		t.Errorf("expected provider Name=='groq', got %q", p.Name)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "x")
	cfg := DefaultConfig()
	cfg.LoadFromEnv()
	p := cfg.GetProvider("groq")
	if p == nil {
		t.Fatal("GetProvider('groq') returned nil")
	}
	if p.APIKey != "x" {
		t.Errorf("expected APIKey=='x', got %q", p.APIKey)
	}
}

func TestGetActiveProviderFallback(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Active = "does-not-exist"
	p := cfg.GetActiveProvider()
	if p == nil {
		t.Fatal("GetActiveProvider() returned nil when active is missing; expected first provider")
	}
	if len(cfg.Providers) == 0 {
		t.Fatal("no providers in default config")
	}
	if p.Name != cfg.Providers[0].Name {
		t.Errorf("expected first provider %q, got %q", cfg.Providers[0].Name, p.Name)
	}
}
