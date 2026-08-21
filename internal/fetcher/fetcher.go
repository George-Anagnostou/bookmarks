package fetcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"golang.org/x/net/html"
)

// Max Title bytes to read. Intended to limit reads for title extraction.
const maxTitleBytes = 32 * 1024

var ErrBlockedTarget = errors.New("blocked target")

type Config struct {
	LookupNetIP func(context.Context, string, string) ([]netip.Addr, error)
	DialContext func(context.Context, string, string) (net.Conn, error)
}

type Fetcher struct {
	httpClient *http.Client
}

func (f *Fetcher) FetchTitle(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
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

	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialValidatedTarget(ctx, network, address, lookup, dial)
	}

	return &Fetcher{httpClient: &http.Client{Transport: transport}}
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

	ip, err := netip.ParseAddr(host)
	if err == nil {
		if isUnsafeAddress(ip) {
			return nil, ErrBlockedTarget
		}

		return dial(ctx, network, net.JoinHostPort(ip.String(), port))
	}

	// hostname handling
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
	default:
		return false
	}
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
