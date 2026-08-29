package telegram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClientDownloadVoice(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		return response(http.StatusOK, "ogg-data"), nil
	})}

	files := &fakeFileURLProvider{url: "https://telegram.test/voice.ogg"}
	client := &Client{files: files, httpClient: httpClient, maxVoiceFileSize: 32}
	audio, err := client.DownloadVoice(context.Background(), "voice-id")
	if err != nil {
		t.Fatalf("DownloadVoice() error = %v", err)
	}
	if string(audio) != "ogg-data" || files.fileID != "voice-id" {
		t.Errorf("audio/file ID = %q/%q", audio, files.fileID)
	}
}

func TestClientDownloadVoiceRejectsLargeFile(t *testing.T) {
	client := &Client{
		files: &fakeFileURLProvider{url: "https://telegram.test/voice.ogg"},
		httpClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, "too-large"), nil
		})},
		maxVoiceFileSize: 4,
	}
	_, err := client.DownloadVoice(context.Background(), "voice-id")
	if !errors.Is(err, ErrVoiceFileTooLarge) {
		t.Fatalf("DownloadVoice() error = %v, want %v", err, ErrVoiceFileTooLarge)
	}
}

func TestClientDownloadVoiceRejectsUnexpectedStatus(t *testing.T) {
	client := &Client{
		files: &fakeFileURLProvider{url: "https://telegram.test/missing.ogg"},
		httpClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return response(http.StatusNotFound, "not found"), nil
		})},
		maxVoiceFileSize: 32,
	}
	if _, err := client.DownloadVoice(context.Background(), "voice-id"); err == nil {
		t.Fatal("DownloadVoice() error = nil")
	}
}

type fakeFileURLProvider struct {
	url    string
	err    error
	fileID string
}

func (p *fakeFileURLProvider) GetFileDirectURL(fileID string) (string, error) {
	p.fileID = fileID
	return p.url, p.err
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
