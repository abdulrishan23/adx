// Package webfetch reads a public web page for research. It is the only
// code in ADX that fetches a URL someone typed, so it is where the network
// boundary is enforced: http and https only, standard ports, and never an
// address inside a private, loopback, link-local or otherwise non-public
// range, checked after DNS resolution and again on every redirect, with the
// connection pinned to the address that passed. Pages are read once, size
// capped, and reduced to text. Nothing in a page is ever followed or acted
// on: links, meta refreshes and instructions in the text are just text.
package webfetch

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core/research"
)

type Config struct {
	// MaxBytes caps a response body. Default 2 MiB.
	MaxBytes int64
	// Timeout covers the whole fetch. Default 20 seconds.
	Timeout time.Duration
	// UserAgent identifies ADX to the site. Default "ADX research bot".
	UserAgent string
	// AllowLoopbackForTests lets a test fetch from httptest servers. Never
	// set in production wiring.
	AllowLoopbackForTests bool
	// Resolver is for tests; nil uses the system resolver.
	Resolver func(ctx context.Context, host string) ([]netip.Addr, error)
}

type Fetcher struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Fetcher {
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 2 << 20
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "ADX research bot (+https://reportageuae.com)"
	}
	if cfg.Resolver == nil {
		cfg.Resolver = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}
	f := &Fetcher{cfg: cfg}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil, // never through an environment proxy
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		DisableKeepAlives:     true,
		// Every connection goes to an address Validate has already approved
		// for this host, so a DNS answer can't change between check and use.
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ip, err := f.resolveSafe(ctx, host)
			if err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		},
	}
	f.http = &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return research.FetchError{Kind: research.FetchUnavailable, Message: "too many redirects"}
			}
			return f.Validate(req.Context(), req.URL.String())
		},
	}
	return f
}

var (
	errScheme = research.FetchError{Kind: research.FetchRefused, Message: "only http and https addresses are fetched"}
	errPort   = research.FetchError{Kind: research.FetchRefused, Message: "only the standard web ports are fetched"}
	errHost   = research.FetchError{Kind: research.FetchRefused, Message: "the address is not a public host"}
	errLocal  = research.FetchError{Kind: research.FetchRefused, Message: "the address points inside a private or local network"}
)

// Validate says whether a URL may be fetched at all. It resolves the host and
// refuses when any answer is non-public, so a name that hides a private
// address is refused the same as the address itself.
func (f *Fetcher) Validate(ctx context.Context, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return research.FetchError{Kind: research.FetchRefused, Message: "not a valid web address"}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errScheme
	}
	if u.User != nil {
		return research.FetchError{Kind: research.FetchRefused, Message: "addresses with credentials are not fetched"}
	}
	host := u.Hostname()
	if f.cfg.AllowLoopbackForTests && strings.EqualFold(host, "localhost") {
		// httptest servers listen on a random loopback port.
		return nil
	}
	switch p := u.Port(); p {
	case "", "80", "443":
	default:
		return errPort
	}
	if host == "" || strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") || strings.HasSuffix(strings.ToLower(host), ".local") || strings.HasSuffix(strings.ToLower(host), ".internal") {
		return errHost
	}
	_, err = f.resolveSafe(ctx, host)
	return err
}

// resolveSafe resolves a host and returns one public address, or refuses.
func (f *Fetcher) resolveSafe(ctx context.Context, host string) (netip.Addr, error) {
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		if !f.public(ip) {
			return netip.Addr{}, errLocal
		}
		return ip, nil
	}
	if f.cfg.AllowLoopbackForTests && strings.EqualFold(host, "localhost") {
		return netip.MustParseAddr("127.0.0.1"), nil
	}
	addrs, err := f.cfg.Resolver(ctx, host)
	if err != nil || len(addrs) == 0 {
		return netip.Addr{}, research.FetchError{Kind: research.FetchUnavailable, Message: "the host could not be resolved"}
	}
	for _, a := range addrs {
		if !f.public(a) {
			return netip.Addr{}, errLocal
		}
	}
	return addrs[0], nil
}

func (f *Fetcher) public(ip netip.Addr) bool {
	ip = ip.Unmap()
	if f.cfg.AllowLoopbackForTests && ip.IsLoopback() {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	// Carrier NAT, documentation, benchmarking and the IPv4-mapped block.
	for _, cidr := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "::ffff:0:0/96", "64:ff9b::/96", "2001:db8::/32", "fc00::/7"} {
		if p, err := netip.ParsePrefix(cidr); err == nil && p.Contains(ip) {
			return false
		}
	}
	return true
}

// Fetch reads one page. The result is text, not HTML, and no part of it is
// followed.
func (f *Fetcher) Fetch(ctx context.Context, raw string) (research.Page, error) {
	if err := f.Validate(ctx, raw); err != nil {
		return research.Page{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(raw), nil)
	if err != nil {
		return research.Page{}, research.FetchError{Kind: research.FetchRefused, Message: "not a valid web address"}
	}
	req.Header.Set("User-Agent", f.cfg.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.1")
	req.Header.Set("Accept-Language", "en,ar;q=0.8")
	res, err := f.http.Do(req)
	if err != nil {
		var fe research.FetchError
		if errors.As(err, &fe) {
			return research.Page{}, fe
		}
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "Timeout") || strings.Contains(err.Error(), "timeout") {
			return research.Page{}, research.FetchError{Kind: research.FetchUnavailable, Message: "the site did not answer in time"}
		}
		return research.Page{}, research.FetchError{Kind: research.FetchUnavailable, Message: "the site could not be reached"}
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return research.Page{}, research.FetchError{Kind: research.FetchUnavailable, Message: fmt.Sprintf("the site answered HTTP %d", res.StatusCode)}
	}
	ct := strings.ToLower(res.Header.Get("Content-Type"))
	if ct != "" && !strings.Contains(ct, "html") && !strings.Contains(ct, "text/plain") && !strings.Contains(ct, "xml") {
		return research.Page{}, research.FetchError{Kind: research.FetchUnsupported, Message: "the address is not a web page"}
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, f.cfg.MaxBytes+1))
	if err != nil {
		return research.Page{}, research.FetchError{Kind: research.FetchUnavailable, Message: "the page could not be read"}
	}
	truncated := int64(len(body)) > f.cfg.MaxBytes
	if truncated {
		body = body[:f.cfg.MaxBytes]
	}
	title, text := Extract(string(body))
	return research.Page{
		URL: raw, FinalURL: res.Request.URL.String(), Status: res.StatusCode, ContentType: ct,
		Title: title, Text: text, Truncated: truncated, FetchedAt: time.Now().UTC(),
	}, nil
}

var (
	reDrop   = regexp.MustCompile(`(?is)<(script|style|noscript|svg|template|iframe)\b.*?</\s*(script|style|noscript|svg|template|iframe)\s*>`)
	reBlock  = regexp.MustCompile(`(?i)</?(p|div|br|li|ul|ol|h[1-6]|tr|td|th|section|article|header|footer|nav|blockquote|pre|table|dd|dt|option)\b[^>]*>`)
	reTag    = regexp.MustCompile(`(?s)<[^>]*>`)
	reTitle  = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reSpaces = regexp.MustCompile(`[ \t\r\f\v]+`)
	reLines  = regexp.MustCompile(`\n\s*\n+`)
)

// Extract reduces HTML to a title and plain text. Comments, scripts and
// styles go; block elements become line breaks; entities are decoded.
func Extract(htmlSrc string) (title, text string) {
	if m := reTitle.FindStringSubmatch(htmlSrc); m != nil {
		title = strings.TrimSpace(html.UnescapeString(reTag.ReplaceAllString(m[1], "")))
	}
	s := regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(htmlSrc, "")
	s = reDrop.ReplaceAllString(s, " ")
	s = reBlock.ReplaceAllString(s, "\n")
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = reSpaces.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	s = strings.Join(lines, "\n")
	s = reLines.ReplaceAllString(s, "\n")
	return title, strings.TrimSpace(s)
}
