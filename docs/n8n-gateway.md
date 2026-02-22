# n8n Webhook Gateway — Workflow Guide

This guide explains how to build the n8n workflow that powers the gogcli webhook gateway. When `GOG_WEBHOOK_URL` is set, **every** Google API request from gogcli is serialized as JSON and sent to your n8n webhook. The webhook handles authentication, executes the real API call, and returns the response.

## Architecture

```
gogcli command
    → Google API client library
    → RetryTransport (handles 429/5xx retries)
    → WebhookTransport (serializes request → JSON → n8n)
    → your n8n webhook (adds auth, calls Google, returns response)
    → Google APIs
```

## Request / Response Format

### Request (sent TO your webhook)

```json
{
  "method": "GET",
  "url": "https://www.googleapis.com/gmail/v1/users/me/labels",
  "headers": {
    "Content-Type": "application/json"
  },
  "body": ""
}
```

| Field     | Type                | Notes |
|-----------|---------------------|-------|
| `method`  | `string`            | HTTP method (GET, POST, PUT, PATCH, DELETE) |
| `url`     | `string`            | Full Google API URL including query parameters |
| `headers` | `map[string]string` | Flattened — first value per header key |
| `body`    | `string`            | Base64-encoded request body (empty string if no body) |

### Response (expected FROM your webhook)

```json
{
  "status_code": 200,
  "headers": {
    "Content-Type": "application/json"
  },
  "body": "eyJpZCI6ICIxNjU4YWE1YzU0Li4uIn0="
}
```

| Field         | Type                | Notes |
|---------------|---------------------|-------|
| `status_code` | `int`               | HTTP status code (200, 400, 403, 404, 500, etc.) |
| `headers`     | `map[string]string` | Response headers as a flat map |
| `body`        | `string`            | Base64-encoded response body (falls back to raw text if not valid base64) |

## n8n Workflow — Step by Step

### 1. Webhook Trigger

- **Node type:** Webhook
- **HTTP Method:** POST
- **Path:** e.g. `/google-api-proxy`
- **Response Mode:** "Using 'Respond to Webhook' Node" (you must control the JSON response)

### 2. Decode the Request (Code node)

```javascript
const input = $input.first().json;

let decodedBody = null;
if (input.body && input.body.length > 0) {
  decodedBody = Buffer.from(input.body, 'base64');
}

return [{
  json: {
    method: input.method,
    url: input.url,
    headers: input.headers || {},
    body: decodedBody ? decodedBody.toString('utf-8') : null,
    rawBody: decodedBody,
  }
}];
```

### 3. Add Google Authentication

This is the most important step — you need to inject a valid Bearer token. Pick the approach that fits your setup:

**Option A — n8n Google OAuth2 credential (recommended)**

Use n8n's built-in Google OAuth2 credential node. In a Code node, retrieve the access token and set the `Authorization` header.

**Option B — Service Account**

Store a Google service account JSON key in n8n credentials and mint tokens programmatically.

**Option C — Static token (dev/testing only)**

```javascript
const headers = $input.first().json.headers;
headers['Authorization'] = 'Bearer ya29.your-token-here';
return [$input.first()];
```

### 4. HTTP Request Node (call the real Google API)

- **Method:** `{{ $json.method }}` (expression)
- **URL:** `{{ $json.url }}`
- **Headers:** Forward all headers from the previous step
- **Body:** Forward the decoded body (for POST/PUT/PATCH)
- **Options:**
  - **Full Response:** ON (you need status code + headers)
  - **Never Error:** ON (you must return 4xx/5xx responses, not throw)

### 5. Encode the Response (Code node)

```javascript
const response = $input.first().json;

let encodedBody = '';
if (response.body != null) {
  const bodyStr = typeof response.body === 'string'
    ? response.body
    : JSON.stringify(response.body);
  encodedBody = Buffer.from(bodyStr).toString('base64');
}

const flatHeaders = {};
if (response.headers) {
  for (const [key, value] of Object.entries(response.headers)) {
    flatHeaders[key] = Array.isArray(value) ? value[0] : String(value);
  }
}

return [{
  json: {
    status_code: response.statusCode || 200,
    headers: flatHeaders,
    body: encodedBody,
  }
}];
```

### 6. Respond to Webhook Node

- **Respond With:** JSON
- **Response Body:** `{{ $json }}` (the encoded response from Step 5)

## Visual Flow

```
Webhook Trigger → Decode Request → Add Auth → HTTP Request → Encode Response → Respond to Webhook
```

## Important Considerations

| Concern | Detail |
|---------|--------|
| **Request body** | Base64-decoded before forwarding to Google |
| **Response body** | Base64-encoded before returning to gogcli |
| **Headers** | Flat `map[string]string` in both directions |
| **Google API errors** | Return as-is (status_code + body) — do NOT throw as n8n errors |
| **Timeout** | gogcli waits 30 seconds per request — keep your workflow fast |
| **Retries** | gogcli retries 429s (5x) and 5xx (3x) automatically — don't double-retry in n8n |
| **All services** | Gmail, Calendar, Drive, Sheets, Docs, Chat, etc. all flow through this single webhook |

## Activate It

```bash
export GOG_WEBHOOK_URL=https://your-n8n-instance.com/webhook/google-api-proxy
gog gmail labels list   # routed through n8n
```

To disable (revert to normal OAuth):

```bash
unset GOG_WEBHOOK_URL
```

## Related Files

| File | Purpose |
|------|---------|
| `internal/googleapi/gateway.go` | `WebhookTransport` implementation (`WebhookRequest`, `WebhookResponse` types) |
| `internal/googleapi/gateway_test.go` | Unit tests for the transport |
| `internal/googleapi/client.go` | Gateway activation point in `optionsForAccountScopes()` |
