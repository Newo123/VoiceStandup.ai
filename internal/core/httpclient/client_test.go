package httpclient

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestNewConfiguresProxy(t *testing.T) {
	client, err := New(Config{ProxyURL: "http://user:pass@proxy.example:8080"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport type = %T, want *http.Transport", client.Transport)
	}

	req := &http.Request{URL: &url.URL{Scheme: "https", Host: "api.example"}}
	proxy, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	if got, want := proxy.String(), "http://user:pass@proxy.example:8080"; got != want {
		t.Errorf("Proxy() = %q, want %q", got, want)
	}
	if got, want := client.Timeout, defaultTimeout; got != want {
		t.Errorf("Timeout = %v, want %v", got, want)
	}
}

func TestNewWithoutProxy(t *testing.T) {
	client, err := New(Config{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	transport := client.Transport.(*http.Transport)
	if transport.Proxy != nil {
		t.Error("Proxy must be nil when ProxyURL is empty")
	}
	if got, want := client.Timeout, 5*time.Second; got != want {
		t.Errorf("Timeout = %v, want %v", got, want)
	}
}

func TestNewRejectsInvalidProxyURL(t *testing.T) {
	_, err := New(Config{ProxyURL: "socks5://proxy.example:1080"})
	if err == nil {
		t.Fatal("New() error = nil, want error")
	}
}
