package bookmarks

import (
	"errors"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "lowercases scheme and host",
			raw:  " HTTPS://Example.COM/Path?x=1#section ",
			want: "https://example.com/Path?x=1#section",
		},
		{
			name: "defaults bare domains to https",
			raw:  "example.com/articles/1",
			want: "https://example.com/articles/1",
		},
		{
			name: "removes default https port",
			raw:  "https://example.com:443/a",
			want: "https://example.com/a",
		},
		{
			name: "keeps non-default ports",
			raw:  "http://example.com:8080/a",
			want: "http://example.com:8080/a",
		},
		{
			name: "adds root path for host-only urls",
			raw:  "https://example.com",
			want: "https://example.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeURL(tt.raw)
			if err != nil {
				t.Fatalf("NormalizeURL() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeURLRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want error
	}{
		{
			name: "empty",
			raw:  " ",
			want: ErrEmptyURL,
		},
		{
			name: "unsupported scheme",
			raw:  "ftp://example.com/file",
			want: ErrUnsupported,
		},
		{
			name: "missing host",
			raw:  "https:///path",
			want: ErrMissingHost,
		},
		{
			name: "credentials",
			raw:  "https://me:secret@example.com/",
			want: ErrURLUserInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NormalizeURL(tt.raw)
			if !errors.Is(err, tt.want) {
				t.Fatalf("NormalizeURL() error = %v, want %v", err, tt.want)
			}
		})
	}
}
