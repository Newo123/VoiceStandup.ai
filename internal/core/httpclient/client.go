// Package httpclient creates reusable HTTP clients for outbound requests.
package httpclient

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// Config configures an outbound HTTP client.
//
// ProxyURL may contain an HTTP or HTTPS proxy URL. An empty value disables the
// proxy and sends requests directly.
type Config struct {
	ProxyURL string
	Timeout  time.Duration
}

// New creates an HTTP client intended to be reused for multiple requests.
// Reusing the returned client allows its transport to reuse established
// connections.
func New(cfg Config) (*http.Client, error) {
	proxy, err := parseProxyURL(cfg.ProxyURL)
	if err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = proxy
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ExpectContinueTimeout = time.Second

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}, nil
}

func parseProxyURL(rawURL string) (func(*http.Request) (*url.URL, error), error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, nil
	}

	proxyURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse proxy URL: %w", err)
	}
	if proxyURL.Scheme != "http" && proxyURL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported proxy URL scheme %q", proxyURL.Scheme)
	}
	if proxyURL.Host == "" {
		return nil, fmt.Errorf("proxy URL must include a host")
	}

	return http.ProxyURL(proxyURL), nil
}
