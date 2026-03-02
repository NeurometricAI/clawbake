package operator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPNotifierSendsCorrectRequest(t *testing.T) {
	var gotBody map[string]string
	var gotContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewHTTPNotifier(server.URL)
	notifier.NotifyInstanceReady(context.Background(), "my-instance", "user-123")

	if gotContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", gotContentType)
	}
	if gotBody["instanceName"] != "my-instance" {
		t.Errorf("expected instanceName 'my-instance', got %q", gotBody["instanceName"])
	}
	if gotBody["userId"] != "user-123" {
		t.Errorf("expected userId 'user-123', got %q", gotBody["userId"])
	}
}

func TestHTTPNotifierDoesNotPanicOnUnreachableServer(t *testing.T) {
	notifier := NewHTTPNotifier("http://127.0.0.1:1")
	// Should not panic
	notifier.NotifyInstanceReady(context.Background(), "my-instance", "user-123")
}
