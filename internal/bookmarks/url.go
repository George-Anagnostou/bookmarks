package bookmarks

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// NormalizeURL validates a bookmark URL and returns its normalized form for
// duplicate detection. URLs without a scheme default to HTTPS.
func NormalizeURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrEmptyURL
	}

	if !strings.Contains(s, "://") {
		s = "https://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}

	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", ErrUnsupportedScheme
	}
	if u.User != nil {
		return "", ErrURLUserInfo
	}
	if u.Host == "" {
		return "", ErrMissingHost
	}

	host := strings.ToLower(u.Hostname())
	port := u.Port()
	switch {
	case port == "":
		u.Host = host
	case (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443"):
		u.Host = host
	default:
		u.Host = net.JoinHostPort(host, port)
	}

	if u.Path == "" {
		u.Path = "/"
	}

	return u.String(), nil
}
