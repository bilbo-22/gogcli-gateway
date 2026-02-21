package googleapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookTransport_RoundTrip_GET(t *testing.T) {
	var received WebhookRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		resp := WebhookResponse{
			StatusCode: 200,
			Headers:    map[string]string{"X-Test": "ok"},
			Body:       base64.StdEncoding.EncodeToString([]byte(`{"result":"success"}`)),
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))

	defer srv.Close()

	tr := NewWebhookTransport(srv.URL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://www.googleapis.com/test", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer token123")

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	defer resp.Body.Close()

	// Verify the serialized request.
	if received.Method != "GET" {
		t.Fatalf("expected GET, got %q", received.Method)
	}

	if received.URL != "https://www.googleapis.com/test" {
		t.Fatalf("unexpected URL: %q", received.URL)
	}

	if received.Headers["Authorization"] != "Bearer token123" {
		t.Fatalf("expected Authorization header, got %q", received.Headers["Authorization"])
	}

	if received.Body != "" {
		t.Fatalf("expected empty body for GET, got %q", received.Body)
	}

	// Verify the reconstructed response.
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if resp.Header.Get("X-Test") != "ok" {
		t.Fatalf("expected X-Test header, got %q", resp.Header.Get("X-Test"))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	if string(respBody) != `{"result":"success"}` {
		t.Fatalf("unexpected response body: %q", string(respBody))
	}
}

func TestWebhookTransport_RoundTrip_POST_WithBody(t *testing.T) {
	var received WebhookRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		resp := WebhookResponse{
			StatusCode: 201,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       base64.StdEncoding.EncodeToString([]byte(`{"id":"123"}`)),
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))

	defer srv.Close()

	tr := NewWebhookTransport(srv.URL)
	reqBody := `{"name":"test"}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://www.googleapis.com/create", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	defer resp.Body.Close()

	// Verify the body was base64-encoded.
	expectedBody := base64.StdEncoding.EncodeToString([]byte(reqBody))
	if received.Body != expectedBody {
		t.Fatalf("expected base64 body %q, got %q", expectedBody, received.Body)
	}

	if received.Method != "POST" {
		t.Fatalf("expected POST, got %q", received.Method)
	}

	// Verify reconstructed response.
	if resp.StatusCode != 201 {
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	if string(respBody) != `{"id":"123"}` {
		t.Fatalf("unexpected response body: %q", string(respBody))
	}
}

func TestWebhookTransport_WebhookError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))

	defer srv.Close()

	tr := NewWebhookTransport(srv.URL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://www.googleapis.com/test", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	_, err = tr.RoundTrip(req)
	if err == nil {
		t.Fatalf("expected error for non-200 webhook response")
	}

	if !IsWebhookError(err) {
		t.Fatalf("expected WebhookError, got %T: %v", err, err)
	}

	var we *WebhookError

	if !errors.As(err, &we) {
		t.Fatalf("expected errors.As to match WebhookError")
	}

	if we.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", we.StatusCode)
	}

	if we.Body != "bad gateway" {
		t.Fatalf("expected body %q, got %q", "bad gateway", we.Body)
	}
}

func TestWebhookTransport_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow handler; context should cancel before this completes.
		<-r.Context().Done()
	}))

	defer srv.Close()

	tr := NewWebhookTransport(srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/test", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	// Cancel context immediately.
	cancel()

	_, err = tr.RoundTrip(req)
	if err == nil {
		t.Fatalf("expected error from canceled context")
	}

	if !errors.Is(err, context.Canceled) {
		// The error may be wrapped; check the string as a fallback.
		if !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("expected context.Canceled, got: %v", err)
		}
	}
}

func TestGatewayWebhookURL(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		t.Setenv(n8nWebhookURLEnv, "")

		if got := gatewayWebhookURL(); got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("trimmed", func(t *testing.T) {
		t.Setenv(n8nWebhookURLEnv, "  https://n8n.example.com/webhook/abc  \n")

		got := gatewayWebhookURL()
		if got != "https://n8n.example.com/webhook/abc" {
			t.Fatalf("expected trimmed URL, got %q", got)
		}
	})

	t.Run("no_whitespace", func(t *testing.T) {
		t.Setenv(n8nWebhookURLEnv, "https://n8n.example.com/webhook/def")

		got := gatewayWebhookURL()
		if got != "https://n8n.example.com/webhook/def" {
			t.Fatalf("expected exact URL, got %q", got)
		}
	})
}

func TestWebhookTransport_FallbackRawBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a body that is NOT valid base64.
		resp := WebhookResponse{
			StatusCode: 200,
			Headers:    map[string]string{"Content-Type": "text/plain"},
			Body:       "this is not base64!!!",
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))

	defer srv.Close()

	tr := NewWebhookTransport(srv.URL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://www.googleapis.com/test", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}

	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	// Should fall back to raw text since it's not valid base64.
	if string(respBody) != "this is not base64!!!" {
		t.Fatalf("expected raw fallback body, got %q", string(respBody))
	}
}
