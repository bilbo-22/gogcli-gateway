package googleapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

const n8nWebhookURLEnv = "N8N_GOG_WEBHOOK_URL"

// WebhookRequest is the JSON payload sent to the n8n webhook.
type WebhookRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

// WebhookResponse is the JSON payload returned by the n8n webhook.
type WebhookResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

// WebhookTransport implements http.RoundTripper by forwarding requests
// through an n8n webhook gateway.
type WebhookTransport struct {
	WebhookURL string
	HTTPClient *http.Client
}

// NewWebhookTransport creates a WebhookTransport targeting the given webhook URL.
func NewWebhookTransport(webhookURL string) *WebhookTransport {
	return &WebhookTransport{
		WebhookURL: webhookURL,
		HTTPClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}
}

// RoundTrip implements http.RoundTripper by serializing the request,
// sending it to the webhook, and reconstructing the response.
func (t *WebhookTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Read and base64-encode the request body.
	var bodyEncoded string

	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
		_ = req.Body.Close()

		if len(bodyBytes) > 0 {
			bodyEncoded = base64.StdEncoding.EncodeToString(bodyBytes)
		}
	}

	// Flatten headers to map[string]string (first value per key).
	headers := make(map[string]string, len(req.Header))
	for k, vals := range req.Header {
		if len(vals) > 0 {
			headers[k] = vals[0]
		}
	}

	webhookReq := WebhookRequest{
		Method:  req.Method,
		URL:     req.URL.String(),
		Headers: headers,
		Body:    bodyEncoded,
	}

	payload, err := json.Marshal(webhookReq)
	if err != nil {
		return nil, fmt.Errorf("marshal webhook request: %w", err)
	}

	slog.Debug("sending request via webhook gateway",
		"method", req.Method,
		"url", req.URL.String(),
		"webhook", t.WebhookURL)

	httpReq, err := http.NewRequestWithContext(req.Context(), http.MethodPost, t.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create webhook HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := t.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("webhook request: %w", err)
	}

	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read webhook response: %w", err)
	}

	// If the webhook itself returns non-200, return a WebhookError.
	if httpResp.StatusCode != http.StatusOK {
		return nil, &WebhookError{
			StatusCode: httpResp.StatusCode,
			Body:       string(respBody),
		}
	}

	var webhookResp WebhookResponse
	if err := json.Unmarshal(respBody, &webhookResp); err != nil {
		return nil, fmt.Errorf("unmarshal webhook response: %w", err)
	}

	// Base64-decode body; fall back to raw text if decode fails.
	var decodedBody []byte

	if webhookResp.Body != "" {
		if decoded, decErr := base64.StdEncoding.DecodeString(webhookResp.Body); decErr != nil {
			decodedBody = []byte(webhookResp.Body)
		} else {
			decodedBody = decoded
		}
	}

	// Reconstruct the http.Response.
	respHeader := make(http.Header, len(webhookResp.Headers))
	for k, v := range webhookResp.Headers {
		respHeader.Set(k, v)
	}

	return &http.Response{
		StatusCode: webhookResp.StatusCode,
		Header:     respHeader,
		Body:       io.NopCloser(bytes.NewReader(decodedBody)),
		Request:    req,
	}, nil
}

// gatewayWebhookURL reads and trims the N8N_GOG_WEBHOOK_URL environment variable.
func gatewayWebhookURL() string {
	return strings.TrimSpace(os.Getenv(n8nWebhookURLEnv))
}

// WebhookError indicates the webhook endpoint returned a non-200 status.
type WebhookError struct {
	StatusCode int
	Body       string
}

func (e *WebhookError) Error() string {
	return fmt.Sprintf("webhook returned status %d: %s", e.StatusCode, e.Body)
}

// IsWebhookError checks if the error is a webhook error.
func IsWebhookError(err error) bool {
	var e *WebhookError
	return errors.As(err, &e)
}
