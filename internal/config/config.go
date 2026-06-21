package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Provider struct {
	Name    string   `json:"name"`
	BaseURL string   `json:"base_url"`
	APIKey  string   `json:"api_key"`
	Models  []string `json:"models"`
	EnvKey  string   `json:"-"`
}

type Config struct {
	Providers []Provider `json:"providers"`
	Active    string     `json:"active"`
}

func DefaultConfig() *Config {
	return &Config{
		Providers: []Provider{
			{
				Name:    "groq",
				BaseURL: "https://api.groq.com/openai/v1",
				Models:  []string{"llama-3.3-70b-versatile", "llama-3.1-70b-versatile", "mixtral-8x7b-32768"},
				EnvKey:  "GROQ_API_KEY",
			},
			{
				Name:    "openai",
				BaseURL: "https://api.openai.com/v1",
				Models:  []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo"},
				EnvKey:  "OPENAI_API_KEY",
			},
			{
				Name:    "anthropic",
				BaseURL: "https://api.anthropic.com/v1",
				Models:  []string{"claude-3-5-sonnet-20241022", "claude-3-opus-20240229"},
				EnvKey:  "ANTHROPIC_API_KEY",
			},
		},
		Active: "groq",
	}
}

func (c *Config) LoadFromEnv() {
	for i := range c.Providers {
		if key := os.Getenv(c.Providers[i].EnvKey); key != "" {
			c.Providers[i].APIKey = key
		}
	}
}

func (c *Config) GetActiveProvider() *Provider {
	for i := range c.Providers {
		if c.Providers[i].Name == c.Active {
			return &c.Providers[i]
		}
	}
	if len(c.Providers) > 0 {
		return &c.Providers[0]
	}
	return nil
}

func (c *Config) GetProvider(name string) *Provider {
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			return &c.Providers[i]
		}
	}
	return nil
}

func (c *Config) SetActive(name string) {
	c.Active = name
}

func (c *Config) UpdateProvider(name string, apiKey, baseURL string) {
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			if apiKey != "" {
				c.Providers[i].APIKey = apiKey
			}
			if baseURL != "" {
				c.Providers[i].BaseURL = baseURL
			}
			break
		}
	}
}

func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "herald", "config.json")
}

func Load() (*Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	defaultCfg := DefaultConfig()
	for i := range cfg.Providers {
		for j := range defaultCfg.Providers {
			if cfg.Providers[i].Name == defaultCfg.Providers[j].Name {
				cfg.Providers[i].EnvKey = defaultCfg.Providers[j].EnvKey
				break
			}
		}
	}

	return &cfg, nil
}

func (c *Config) Save() error {
	path := ConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
