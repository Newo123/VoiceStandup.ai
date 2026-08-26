package postgres

import (
	"context"
	"testing"
	"time"
)

func TestNewRejectsEmptyURL(t *testing.T) {
	pool, err := New(context.Background(), "", time.Second)
	if err == nil {
		if pool != nil {
			pool.Close()
		}
		t.Fatal("New() error = nil")
	}
}
