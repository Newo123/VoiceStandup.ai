package redis

import (
	"context"
	"testing"
	"time"
)

func TestNewRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{name: "empty address", config: Config{ConnectTimeout: time.Second}},
		{name: "negative db", config: Config{Address: "localhost:6379", DB: -1, ConnectTimeout: time.Second}},
		{name: "invalid timeout", config: Config{Address: "localhost:6379"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := New(context.Background(), test.config)
			if err == nil {
				if client != nil {
					_ = client.Close()
				}
				t.Fatal("New() error = nil")
			}
		})
	}
}
