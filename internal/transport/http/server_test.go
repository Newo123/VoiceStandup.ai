package httptransport

import (
	"net/http"
	"testing"
)

func TestNewServerValidatesDependencies(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if _, err := NewServer("", handler); err == nil {
		t.Fatal("NewServer() with empty address error = nil")
	}
	if _, err := NewServer(":8080", nil); err == nil {
		t.Fatal("NewServer() with nil handler error = nil")
	}
}
