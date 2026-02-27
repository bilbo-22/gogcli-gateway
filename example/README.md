# gogcli-gateway — Python example server

This directory contains a self-contained Python webhook gateway server that
demonstrates the full gogcli-gateway architecture. It is meant to be read,
understood, and adapted — not used as-is in production.

---

## What this server does

When `GOG_WEBHOOK_URL` is set, every Google API call made by `gog` is
serialized as a JSON payload and POSTed to your webhook URL instead of
reaching Google directly.

This example server receives those payloads and applies a **three-stage policy
chain**:

1. **Denylist** — certain request patterns are rejected immediately (HTTP 403)
   without any forwarding (returned as inner `status_code: 403`).
2. **Allowlist** — safe read-only operations (and draft operations) are
   forwarded to Google automatically.
3. **Human-in-the-loop** — everything else is queued for operator approval.
   The webhook returns immediately with inner `status_code: 202`, then a
   background worker prompts for `y`/`n` and forwards only if approved.

```
gog  →  POST /webhook  →  policy chain  →  Google APIs
                              |
                    denylist / allowlist / human prompt
```

---

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `GOOGLE_ACCESS_TOKEN` | Yes (for forwarding) | — | Bearer token used to authenticate forwarded requests to Google. |
| `GATEWAY_SECRET` | No | — | If set, incoming webhook requests must carry `Authorization: Bearer <secret>`. Requests without it are rejected with HTTP 401. |
| `PORT` | No | `8080` | TCP port the server listens on. |
| `APPROVAL_TIMEOUT_SECONDS` | No | `30` | Seconds to wait for a human response before auto-denying. |

---

## How to run

Install the one non-stdlib dependency:

```bash
pip install requests
```

Export your Google access token (obtain one with `gog auth status` or via
`gcloud auth print-access-token`):

```bash
export GOOGLE_ACCESS_TOKEN="ya29.your-token-here"
```

Optionally set a shared secret that gogcli must send:

```bash
export GATEWAY_SECRET="change-me"
```

Start the server:

```bash
python python_gateway_server.py
```

The server prints its configuration on startup and then waits for requests.

---

## Example gogcli usage

In a second terminal, point `gog` at the gateway and run any command:

```bash
# Route all gogcli requests through the local gateway
export GOG_WEBHOOK_URL=http://localhost:8080/webhook

# Read-only — auto-approved by the allowlist
gog gmail labels list

# Draft operation — auto-approved
gog gmail drafts list

# Write operation — triggers human-in-the-loop prompt on the server terminal
gog gmail send --to you@example.com --subject "Test" --body "Hello"

# Destructive — immediately blocked by the denylist (DELETE method)
gog drive delete some-file-id

# Blocked by path fragment
# (any request whose URL path contains /trash or /permanently)
```

If `GATEWAY_SECRET` is set, every request to `/webhook` must carry the header
`Authorization: Bearer <secret>`.  gogcli itself does not send this header
automatically, so you must add it at the network layer (e.g. a reverse proxy
that injects the header, or a future gogcli release with built-in secret
support).  Omit `GATEWAY_SECRET` for local development.

---

## How the three policy mechanisms work

### 1. Denylist (auto-deny, no forwarding)

Evaluated first. A request is blocked if **any** of the following is true:

- The HTTP method is `DELETE`.
- The URL path contains `/admin`, `/trash`, or `/permanently`.
- A query parameter matches `force=true` or `debug=true`.

Blocked requests return webhook HTTP 200 with inner `status_code: 403`.
The Google token is never used.

### 2. Allowlist (auto-approve, forward immediately)

Evaluated after the denylist passes. A request is forwarded without human
review if **any** of the following is true:

- The HTTP method is `GET` (read-only, always safe).
- The URL path contains `/drafts/` or `/drafts` (any HTTP method) — this
  covers listing, creating, updating, and fetching drafts.  Note that
  `DELETE` of a draft is still blocked by the denylist (DELETE method).

### 3. Human-in-the-loop (queued async approval)

Any request that passes the denylist but does not match the allowlist is queued
for human review:

1. The webhook immediately returns HTTP 200 with this inner response:
   `status_code: 202`, body `{"status":"pending_approval","message":"Request received and sent for human approval."}`.
2. A background queue worker prints request details and prompts:
   `Approve this request? [y/N]`.
3. If the operator types `y` or `Y` (and presses Enter) within the timeout,
   the worker forwards the request to Google.
4. If the operator denies or times out, the request is dropped.

Because this example returns immediately with `202`, gogcli sees "accepted /
pending approval" for HITL requests rather than a transport failure.

---

## Configuring the policy rules

All policy constants are defined as clearly-labelled module-level variables
near the top of `python_gateway_server.py`:

```python
# Blocked HTTP methods
DENY_METHODS = {"DELETE"}

# Blocked URL path fragments
DENY_PATH_FRAGMENTS = {"/admin", "/trash", "/permanently"}

# Blocked query parameters (key=value)
DENY_QUERY_PARAMS = {"force": "true", "debug": "true"}

# Always-allowed HTTP methods
ALLOW_METHODS = {"GET"}

# Always-allowed URL path fragments (any method)
ALLOW_PATH_FRAGMENTS = {"/drafts/", "/drafts"}

# Method+path combinations that are always allowed
ALLOW_METHOD_PATH = {"POST": {"/drafts"}}
```

Edit these sets to add or remove rules without touching any other logic.

---

## Webhook payload format

**gogcli sends (POST to /webhook):**

```json
{
  "method": "GET",
  "url": "https://www.googleapis.com/gmail/v1/users/me/messages?q=newer_than:7d",
  "headers": {
    "Content-Type": "application/json",
    "Accept": "application/json"
  },
  "body": ""
}
```

The `body` field is base64-encoded. An empty string means no body.

**Your server must return (HTTP 200):**

```json
{
  "status_code": 200,
  "headers": {
    "Content-Type": "application/json"
  },
  "body": "eyJtZXNzYWdlcyI6IFsuLi5dfQ=="
}
```

The response `body` must be base64-encoded.

The outer webhook HTTP status must be **200** for valid responses.
Use inner `status_code` for policy outcomes (for example `403` deny, `202`
pending approval, or Google's status on forwarded responses). gogcli treats
outer non-200 webhook responses as transport errors.

---

## Endpoints

| Path | Method | Description |
|---|---|---|
| `/webhook` | `POST` | Main webhook endpoint — receives gogcli requests. |
| `/health` | `GET` | Returns `ok`. Useful for load-balancer health checks. |
