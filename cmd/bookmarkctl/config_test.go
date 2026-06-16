package main

import "testing"

func TestLoadConfigReadsEnvironment(t *testing.T) {
	cfg, err := loadConfig(mapLookup(map[string]string{
		"BOOKMARKS_URL":   " http://localhost:8080/ ",
		"BOOKMARKS_TOKEN": " test-token\n",
	}))
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	if cfg.BaseURL != "http://localhost:8080/" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Token != "test-token" {
		t.Fatalf("Token = %q", cfg.Token)
	}
}

func TestLoadConfigRequiresBaseURL(t *testing.T) {
	_, err := loadConfig(mapLookup(map[string]string{
		"BOOKMARKS_TOKEN": "test-token",
	}))
	if err == nil {
		t.Fatal("loadConfig() error = nil, want error")
	}
}

func TestLoadConfigRequiresToken(t *testing.T) {
	_, err := loadConfig(mapLookup(map[string]string{
		"BOOKMARKS_URL": "http://localhost:8080",
	}))
	if err == nil {
		t.Fatal("loadConfig() error = nil, want error")
	}
}

func TestLoadConfigRejectsBlankValues(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "blank base url",
			env: map[string]string{
				"BOOKMARKS_URL":   " ",
				"BOOKMARKS_TOKEN": "test-token",
			},
		},
		{
			name: "blank token",
			env: map[string]string{
				"BOOKMARKS_URL":   "http://localhost:8080",
				"BOOKMARKS_TOKEN": " ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadConfig(mapLookup(tt.env))
			if err == nil {
				t.Fatal("loadConfig() error = nil, want error")
			}
		})
	}
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
