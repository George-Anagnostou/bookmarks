package fetcher

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestFetchTitleRejectsUnsafeLiteralAddresses(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "loopback IPv4", url: "http://127.0.0.1/"},
		{name: "private IPv4", url: "http://10.0.0.1/"},
		{name: "link-local IPv4", url: "http://169.254.169.254/"},
		{name: "unspecified IPv4", url: "http://0.0.0.0/"},
		{name: "loopback IPv6", url: "http://[::1]/"},
		{name: "unique-local IPv6", url: "http://[fc00::1]/"},
		{name: "link-local IPv6", url: "http://[fe80::1]/"},
		{name: "IPv4-mapped loopback", url: "http://[::ffff:127.0.0.1]/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolverCalled := false
			dialCalled := false
			f := NewFetcher(Config{
				LookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
					resolverCalled = true
					return nil, errors.New("literal address must not be resolved")
				},
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					dialCalled = true
					return nil, errors.New("unsafe address was dialed")
				},
			})

			_, err := f.FetchTitle(context.Background(), tt.url)
			if !errors.Is(err, ErrBlockedTarget) {
				t.Fatalf("FetchTitle() error = %v, want %v", err, ErrBlockedTarget)
			}
			if resolverCalled {
				t.Fatal("resolver was called for a literal address")
			}
			if dialCalled {
				t.Fatal("unsafe literal address was dialed")
			}
		})
	}
}

func TestFetchTitleAllowsPublicLiteralAddressWithoutResolving(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<title>Public page</title>`))
	}))
	defer page.Close()

	resolverCalled := false
	var dialedAddress string
	f := NewFetcher(Config{
		LookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
			resolverCalled = true
			return nil, errors.New("literal address must not be resolved")
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialedAddress = address
			return (&net.Dialer{}).DialContext(ctx, network, page.Listener.Addr().String())
		},
	})

	title, err := f.FetchTitle(context.Background(), "http://93.184.216.34/article")
	if err != nil {
		t.Fatalf("FetchTitle() error = %v", err)
	}
	if title != "Public page" {
		t.Fatalf("FetchTitle() title = %q, want %q", title, "Public page")
	}
	if resolverCalled {
		t.Fatal("resolver was called for a literal address")
	}
	if dialedAddress != "93.184.216.34:80" {
		t.Fatalf("dialed address = %q, want public literal address", dialedAddress)
	}
}

func TestFetchTitleRejectsLiteralAddressOnNonWebPort(t *testing.T) {
	dialCalled := false
	f := NewFetcher(Config{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dialCalled = true
			return nil, errors.New("unexpected dial")
		},
	})

	_, err := f.FetchTitle(context.Background(), "http://93.184.216.34:8080/")
	if !errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("FetchTitle() error = %v, want %v", err, ErrBlockedTarget)
	}
	if dialCalled {
		t.Fatal("non-web port was dialed")
	}
}

func TestFetchTitleRejectsHostnameOnNonWebPortBeforeResolving(t *testing.T) {
	resolverCalled := false
	f := NewFetcher(Config{
		LookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
			resolverCalled = true
			return nil, errors.New("unexpected lookup")
		},
	})

	_, err := f.FetchTitle(context.Background(), "http://public.test:8080/")
	if !errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("FetchTitle() error = %v, want %v", err, ErrBlockedTarget)
	}
	if resolverCalled {
		t.Fatal("hostname was resolved for a blocked port")
	}
}

func TestFetchTitleRejectsHostnamesResolvingToUnsafeAddresses(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{name: "loopback", addr: "127.0.0.1"},
		{name: "private IPv4", addr: "10.0.0.1"},
		{name: "cloud metadata", addr: "169.254.169.254"},
		{name: "private IPv6", addr: "fc00::1"},
		{name: "link-local IPv6", addr: "fe80::1"},
		{name: "IPv4-mapped loopback", addr: "::ffff:127.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolverCalled := false
			dialCalled := false
			f := NewFetcher(Config{
				LookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
					resolverCalled = true
					return []netip.Addr{netip.MustParseAddr(tt.addr)}, nil
				},
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					dialCalled = true
					return nil, errors.New("unsafe address was dialed")
				},
			})

			_, err := f.FetchTitle(context.Background(), "http://unsafe.test/")
			if !errors.Is(err, ErrBlockedTarget) {
				t.Fatalf("FetchTitle() error = %v, want %v", err, ErrBlockedTarget)
			}
			if !resolverCalled {
				t.Fatal("hostname was not resolved")
			}
			if dialCalled {
				t.Fatal("unsafe resolved address was dialed")
			}
		})
	}
}

func TestFetchTitleRejectsHostnameWhenAnyResolvedAddressIsUnsafe(t *testing.T) {
	resolverCalled := false
	dialCalled := false
	f := NewFetcher(Config{
		LookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
			resolverCalled = true
			return []netip.Addr{
				netip.MustParseAddr("93.184.216.34"),
				netip.MustParseAddr("127.0.0.1"),
			}, nil
		},
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dialCalled = true
			return nil, errors.New("unsafe address was dialed")
		},
	})

	_, err := f.FetchTitle(context.Background(), "http://mixed.test/")
	if !errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("FetchTitle() error = %v, want %v", err, ErrBlockedTarget)
	}
	if !resolverCalled {
		t.Fatal("hostname was not resolved")
	}
	if dialCalled {
		t.Fatal("hostname with an unsafe DNS result was dialed")
	}
}

func TestFetchTitleFailsClosedWhenHostnameCannotBeResolved(t *testing.T) {
	resolverErr := errors.New("DNS unavailable")
	resolverCalled := false
	dialCalled := false
	f := NewFetcher(Config{
		LookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
			resolverCalled = true
			return nil, resolverErr
		},
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dialCalled = true
			return nil, errors.New("unexpected dial")
		},
	})

	_, err := f.FetchTitle(context.Background(), "http://unavailable.test/")
	if !errors.Is(err, resolverErr) {
		t.Fatalf("FetchTitle() error = %v, want wrapped resolver error", err)
	}
	if !resolverCalled {
		t.Fatal("hostname was not resolved")
	}
	if dialCalled {
		t.Fatal("hostname was dialed after DNS failure")
	}
}

func TestFetchTitleFailsClosedWhenHostnameHasNoAddresses(t *testing.T) {
	resolverCalled := false
	dialCalled := false
	f := NewFetcher(Config{
		LookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
			resolverCalled = true
			return nil, nil
		},
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dialCalled = true
			return nil, errors.New("unexpected dial")
		},
	})

	_, err := f.FetchTitle(context.Background(), "http://empty.test/")
	if !errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("FetchTitle() error = %v, want %v", err, ErrBlockedTarget)
	}
	if !resolverCalled {
		t.Fatal("hostname was not resolved")
	}
	if dialCalled {
		t.Fatal("hostname was dialed without a DNS address")
	}
}

func TestFetchTitleAllowsPublicHostnameWithoutDialingTheHostname(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<title>Public page</title>`))
	}))
	defer page.Close()

	var dialedAddress string
	f := NewFetcher(Config{
		LookupNetIP: func(ctx context.Context, network, host string) ([]netip.Addr, error) {
			if network != "ip" || host != "public.test" {
				t.Fatalf("LookupNetIP() arguments = (%q, %q), want (ip, public.test)", network, host)
			}
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialedAddress = address
			return (&net.Dialer{}).DialContext(ctx, network, page.Listener.Addr().String())
		},
	})

	title, err := f.FetchTitle(context.Background(), "http://public.test/article")
	if err != nil {
		t.Fatalf("FetchTitle() error = %v", err)
	}
	if title != "Public page" {
		t.Fatalf("FetchTitle() title = %q, want %q", title, "Public page")
	}
	if dialedAddress != "93.184.216.34:80" {
		t.Fatalf("dialed address = %q, want validated public address", dialedAddress)
	}
}

func TestFetchTitleDialsValidatedIPv6Address(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<title>IPv6 page</title>`))
	}))
	defer page.Close()

	var dialedAddress string
	f := NewFetcher(Config{
		LookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("2001:4860:4860::8888")}, nil
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialedAddress = address
			return (&net.Dialer{}).DialContext(ctx, network, page.Listener.Addr().String())
		},
	})

	title, err := f.FetchTitle(context.Background(), "http://ipv6.test/article")
	if err != nil {
		t.Fatalf("FetchTitle() error = %v", err)
	}
	if title != "IPv6 page" {
		t.Fatalf("FetchTitle() title = %q, want %q", title, "IPv6 page")
	}
	if dialedAddress != "[2001:4860:4860::8888]:80" {
		t.Fatalf("dialed address = %q, want bracketed IPv6 address", dialedAddress)
	}
}

func TestFetchTitleAllowsHTTPSWithValidatedAddress(t *testing.T) {
	page := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<title>HTTPS page</title>`))
	}))
	defer page.Close()

	var dialedAddress string
	f := NewFetcher(Config{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // test server cert is not for public.test.
		LookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialedAddress = address
			return (&net.Dialer{}).DialContext(ctx, network, page.Listener.Addr().String())
		},
	})

	title, err := f.FetchTitle(context.Background(), "https://public.test/article")
	if err != nil {
		t.Fatalf("FetchTitle() error = %v", err)
	}
	if title != "HTTPS page" {
		t.Fatalf("FetchTitle() title = %q, want %q", title, "HTTPS page")
	}
	if dialedAddress != "93.184.216.34:443" {
		t.Fatalf("dialed address = %q, want validated HTTPS address", dialedAddress)
	}
}

func TestFetchTitleTimesOutWhileWaitingForResponse(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`<title>Too late</title>`))
	}))
	defer page.Close()

	f := fetcherForTestServerWithTimeout(t, page, 20*time.Millisecond)
	_, err := f.FetchTitle(context.Background(), "http://public.test/")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("FetchTitle() error = %v, want context deadline exceeded", err)
	}
}

func TestIsUnsafeAddressRejectsSpecialPurposeRanges(t *testing.T) {
	tests := []string{
		"0.0.0.1",
		"100.64.0.1",
		"192.0.0.1",
		"192.0.2.1",
		"198.18.0.1",
		"198.51.100.1",
		"203.0.113.1",
		"2001:db8::1",
	}

	for _, address := range tests {
		t.Run(address, func(t *testing.T) {
			if !isUnsafeAddress(netip.MustParseAddr(address)) {
				t.Fatalf("isUnsafeAddress(%q) = false, want true", address)
			}
		})
	}
}

func TestFetchTitleRejectsRedirectToUnsafeHostname(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://private.test/secret", http.StatusFound)
	}))
	defer page.Close()

	f := fetcherForRedirectTest(t, page, map[string][]netip.Addr{
		"public.test":  {netip.MustParseAddr("93.184.216.34")},
		"private.test": {netip.MustParseAddr("10.0.0.1")},
	})

	_, err := f.FetchTitle(context.Background(), "http://public.test/")
	if !errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("FetchTitle() error = %v, want %v", err, ErrBlockedTarget)
	}
}

func TestFetchTitleRejectsRedirectToUnsafeLiteralAddress(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/secret", http.StatusFound)
	}))
	defer page.Close()

	f := fetcherForRedirectTest(t, page, map[string][]netip.Addr{
		"public.test": {netip.MustParseAddr("93.184.216.34")},
	})

	_, err := f.FetchTitle(context.Background(), "http://public.test/")
	if !errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("FetchTitle() error = %v, want %v", err, ErrBlockedTarget)
	}
}

func TestFetchTitleRechecksEveryRedirect(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Host {
		case "first.test":
			http.Redirect(w, r, "http://second.test/", http.StatusFound)
		case "second.test":
			http.Redirect(w, r, "http://private.test/", http.StatusFound)
		default:
			t.Fatalf("unexpected request host %q", r.Host)
		}
	}))
	defer page.Close()

	f := fetcherForRedirectTest(t, page, map[string][]netip.Addr{
		"first.test":   {netip.MustParseAddr("93.184.216.34")},
		"second.test":  {netip.MustParseAddr("142.250.72.14")},
		"private.test": {netip.MustParseAddr("10.0.0.1")},
	})

	_, err := f.FetchTitle(context.Background(), "http://first.test/")
	if !errors.Is(err, ErrBlockedTarget) {
		t.Fatalf("FetchTitle() error = %v, want %v", err, ErrBlockedTarget)
	}
}

func TestFetchTitleAllowsRedirectBetweenPublicHostnames(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "first.test" {
			http.Redirect(w, r, "http://second.test/article", http.StatusFound)
			return
		}
		if r.Host != "second.test" {
			t.Fatalf("unexpected request host %q", r.Host)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<title>Redirected title</title>`))
	}))
	defer page.Close()

	f := fetcherForRedirectTest(t, page, map[string][]netip.Addr{
		"first.test":  {netip.MustParseAddr("93.184.216.34")},
		"second.test": {netip.MustParseAddr("142.250.72.14")},
	})

	title, err := f.FetchTitle(context.Background(), "http://first.test/")
	if err != nil {
		t.Fatalf("FetchTitle() error = %v", err)
	}
	if title != "Redirected title" {
		t.Fatalf("FetchTitle() title = %q, want %q", title, "Redirected title")
	}
}

func TestFetchTitleRejectsUnsupportedScheme(t *testing.T) {
	resolverCalled := false
	dialCalled := false
	f := NewFetcher(Config{
		LookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
			resolverCalled = true
			return nil, errors.New("unexpected lookup")
		},
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			dialCalled = true
			return nil, errors.New("unexpected dial")
		},
	})

	_, err := f.FetchTitle(context.Background(), "ftp://public.test/file")
	if err == nil {
		t.Fatal("FetchTitle() error = nil, want unsupported scheme failure")
	}
	if resolverCalled || dialCalled {
		t.Fatal("unsupported scheme reached the network")
	}
}

func fetcherForRedirectTest(t *testing.T, page *httptest.Server, addresses map[string][]netip.Addr) *Fetcher {
	t.Helper()

	f := NewFetcher(Config{
		LookupNetIP: func(ctx context.Context, network, host string) ([]netip.Addr, error) {
			resolved, ok := addresses[host]
			if !ok {
				return nil, errors.New("unexpected hostname: " + host)
			}
			return resolved, nil
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, page.Listener.Addr().String())
		},
	})
	return f
}
