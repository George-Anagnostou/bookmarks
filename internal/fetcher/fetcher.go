package fetcher

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Max Title bytes to read. Intended to limit reads for title extraction.
const maxTitleBytes = 32 * 1024

const defaultFetchTimeout = 10 * time.Second

var blockedPrefixes = [...]netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:db8::/32"),
}

var ErrBlockedTarget = errors.New("blocked target")

type Config struct {
	LookupNetIP     func(context.Context, string, string) ([]netip.Addr, error)
	DialContext     func(context.Context, string, string) (net.Conn, error)
	TLSClientConfig *tls.Config
	Timeout         time.Duration
}

type Fetcher struct {
	httpClient *http.Client
}

func (f *Fetcher) FetchTitle(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	if !strings.EqualFold(req.URL.Scheme, "http") && !strings.EqualFold(req.URL.Scheme, "https") {
		return "", fmt.Errorf("unsupported scheme: %s", req.URL.Scheme)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return "", fmt.Errorf("parse content type: %w", err)
		}
		if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
			return "", fmt.Errorf("unsupported content type: %s", mediaType)
		}
	}

	limited := io.LimitReader(resp.Body, maxTitleBytes)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("parse html: %w", err)
	}

	if t := findMetaOGTitle(doc); strings.TrimSpace(t) != "" {
		return strings.TrimSpace(t), nil
	}

	if t := findTitleText(doc); strings.TrimSpace(t) != "" {
		return strings.TrimSpace(t), nil
	}

	return "", nil
}

func NewFetcher(cfg Config) *Fetcher {
	lookup := cfg.LookupNetIP
	if lookup == nil {
		lookup = net.DefaultResolver.LookupNetIP
	}

	dial := cfg.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if cfg.TLSClientConfig != nil {
		transport.TLSClientConfig = cfg.TLSClientConfig.Clone()
	}

	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialValidatedTarget(ctx, network, address, lookup, dial)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultFetchTimeout
	}

	return &Fetcher{httpClient: &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}}
}

func dialValidatedTarget(
	ctx context.Context,
	network string,
	address string,
	lookup func(context.Context, string, string) ([]netip.Addr, error),
	dial func(context.Context, string, string) (net.Conn, error),
) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("split target address: %w", err)
	}

	portInt, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("parse target port: %w", err)
	}
	if portInt != 80 && portInt != 443 {
		return nil, ErrBlockedTarget
	}

	ip, err := netip.ParseAddr(host)
	if err == nil {
		ip = ip.Unmap()
		if isUnsafeAddress(ip) {
			return nil, ErrBlockedTarget
		}

		return dial(ctx, network, net.JoinHostPort(ip.String(), port))
	}

	addresses, err := lookup(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve target: %w", err)
	}

	if len(addresses) == 0 {
		return nil, ErrBlockedTarget
	}

	for _, addr := range addresses {
		if isUnsafeAddress(addr) {
			return nil, ErrBlockedTarget
		}
	}

	selected := addresses[0].Unmap()

	return dial(ctx, network, net.JoinHostPort(selected.String(), port))
}

func isUnsafeAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	switch {
	case !addr.IsValid():
		return true
	case addr.Zone() != "":
		return true
	case addr.IsUnspecified():
		return true
	case addr.IsLoopback():
		return true
	case addr.IsPrivate():
		return true
	case addr.IsLinkLocalUnicast():
		return true
	case addr.IsMulticast():
		return true
	}

	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}

	return false
}

func findMetaOGTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "meta" {
		seenOG := false
		var content string
		for _, a := range n.Attr {
			switch a.Key {
			case "property", "name":
				if strings.ToLower(a.Val) == "og:title" {
					seenOG = true
				}
			case "content":
				content = a.Val
			}
		}
		if seenOG && content != "" {
			return content
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := findMetaOGTitle(c); title != "" {
			return title
		}
	}

	return ""
}

func findTitleText(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" {
		if c := n.FirstChild; c != nil && c.Type == html.TextNode {
			return n.FirstChild.Data
		}
		return ""
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := findTitleText(c); title != "" {
			return title
		}
	}

	return ""
}
