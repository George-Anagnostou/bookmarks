package main

import (
	"errors"
	"strings"
)

type config struct {
	BaseURL string
	Token   string
}

func loadConfig(lookup func(string) (string, bool)) (config, error) {
	var cfg config

	if value, ok := lookup("BOOKMARKS_URL"); ok {
		cfg.BaseURL = strings.TrimSpace(value)
	}

	if value, ok := lookup("BOOKMARKS_TOKEN"); ok {
		cfg.Token = strings.TrimSpace(value)
	}

	if cfg.BaseURL == "" {
		return config{}, errors.New("BOOKMARKS_URL must be set")
	}

	if cfg.Token == "" {
		return config{}, errors.New("BOOKMARKS_TOKEN must be set")
	}

	return cfg, nil
}
