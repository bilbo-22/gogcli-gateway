"""
python_gateway_server.py — gogcli-gateway example webhook server
=================================================================

This server receives JSON webhook payloads from gogcli (when GOG_WEBHOOK_URL is
set) and applies a three-stage policy chain before deciding whether to forward
the request on to Google APIs.

Policy chain (evaluated in order):
  1. DENYLIST  — return an inner HTTP 403 (outer webhook response is still 200)
  2. ALLOWLIST — forward to Google without human review
  3. HUMAN IN THE LOOP — return inner HTTP 202 immediately, then process
                         approval asynchronously in a background worker

Incoming payload (from gogcli):
    {
        "method": "GET",
        "url": "https://www.googleapis.com/gmail/v1/users/me/messages?q=...",
        "headers": {"Content-Type": "application/json"},
        "body": ""          # base64-encoded; empty string means no body
    }

Expected response (to gogcli):
    {
        "status_code": 200,
        "headers": {"Content-Type": "application/json"},
        "body": "<base64-encoded response body>"
    }

For policy denials, return webhook HTTP 200 with an inner response
`status_code` of 403.

Environment variables
---------------------
GOOGLE_ACCESS_TOKEN      Required when forwarding. Bearer token sent to Google.
GATEWAY_SECRET           Optional. If set, incoming requests must carry
                         "Authorization: Bearer <secret>".
PORT                     TCP port to listen on. Default: 8080.
APPROVAL_TIMEOUT_SECONDS Seconds to wait for human input. Default: 30.

Quick start
-----------
    pip install requests
    export GOOGLE_ACCESS_TOKEN="ya29.your-token-here"
    export GATEWAY_SECRET="my-secret"          # optional
    python python_gateway_server.py

    # In another terminal / shell session:
    export GOG_WEBHOOK_URL=http://localhost:8080/webhook
    gog gmail labels list

Dependencies
------------
    requests  (pip install requests)
    All other imports are Python 3.8+ stdlib.

Python version: 3.8+
"""

import base64
import json
import os
import queue
import sys
import threading
import urllib.parse
from http.server import BaseHTTPRequestHandler, HTTPServer

import requests as req_lib

# ---------------------------------------------------------------------------
# POLICY CONFIGURATION
# Edit these constants to customise the gateway's behaviour.
# ---------------------------------------------------------------------------

# --- DENYLIST ---------------------------------------------------------------
# Requests matching ANY of these rules are immediately rejected (HTTP 403).

# HTTP methods that are never allowed.
DENY_METHODS = {"DELETE"}

# URL path fragments that trigger a denial.
DENY_PATH_FRAGMENTS = {
    "/admin",
    "/trash",
    "/permanently",
}

# Query-parameter key=value pairs that trigger a denial.
# If the key is present with the given value the request is denied.
DENY_QUERY_PARAMS = {
    "force": "true",
    "debug": "true",
}

# --- ALLOWLIST --------------------------------------------------------------
# Requests matching ANY of these rules are auto-approved and forwarded to
# Google without human review.

# HTTP methods that are always allowed (read-only, safe).
ALLOW_METHODS = {"GET"}

# URL path fragments that are always allowed.
ALLOW_PATH_FRAGMENTS = {
    "/drafts/",   # individual draft operations
    "/drafts",    # draft listing / creation (also caught by ALLOW_POST_PATH_FRAGMENTS)
}

# HTTP methods paired with path fragments that are always allowed.
# Key: HTTP method string.  Value: set of path fragments.
ALLOW_METHOD_PATH = {
    "POST": {"/drafts"},   # creating a new draft
}

# ---------------------------------------------------------------------------
# RUNTIME CONFIGURATION (from environment variables)
# ---------------------------------------------------------------------------

PORT = int(os.environ.get("PORT", "8080"))
GOOGLE_ACCESS_TOKEN = os.environ.get("GOOGLE_ACCESS_TOKEN", "")
GATEWAY_SECRET = os.environ.get("GATEWAY_SECRET", "")
APPROVAL_TIMEOUT_SECONDS = int(os.environ.get("APPROVAL_TIMEOUT_SECONDS", "30"))


# ---------------------------------------------------------------------------
# WEBHOOK RESPONSE HELPERS
# ---------------------------------------------------------------------------

def build_webhook_response(
    status_code: int, body_obj: dict, headers: dict | None = None
) -> dict:
    """Build a WebhookResponse payload with a base64-encoded JSON body."""
    response_headers = {"Content-Type": "application/json"}
    if headers:
        response_headers.update(headers)

    encoded_body = base64.b64encode(json.dumps(body_obj).encode("utf-8")).decode("ascii")
    return {
        "status_code": status_code,
        "headers": response_headers,
        "body": encoded_body,
    }


def pending_webhook_response() -> dict:
    """Return an inner 202 response used when human approval is pending."""
    return build_webhook_response(
        status_code=202,
        body_obj={
            "status": "pending_approval",
            "message": "Request received and sent for human approval.",
        },
    )


def denied_webhook_response(reason: str) -> dict:
    """Return an inner 403 response explaining why the gateway denied a request."""
    return build_webhook_response(
        status_code=403,
        body_obj={
            "status": "denied",
            "message": reason,
        },
    )


# Queue used by the HITL flow so HTTP responses can return immediately.
APPROVAL_QUEUE: queue.Queue = queue.Queue()


def enqueue_approval_task(method: str, url: str, headers: dict, body_b64: str) -> None:
    """Store a request for asynchronous human approval processing."""
    APPROVAL_QUEUE.put(
        {
            "method": method,
            "url": url,
            "headers": headers,
            "body_b64": body_b64,
        }
    )


def process_approval_queue() -> None:
    """Background worker that handles queued approval tasks one at a time."""
    while True:
        task = APPROVAL_QUEUE.get()
        method = task["method"]
        url = task["url"]
        headers = task["headers"]
        body_b64 = task["body_b64"]

        print(f"[QUEUE] Processing pending approval: {method} {url}")
        approved = ask_human_approval(method, url, headers, body_b64)
        if not approved:
            print(f"[QUEUE] Request denied by operator: {method} {url}")
            APPROVAL_QUEUE.task_done()
            continue

        if not GOOGLE_ACCESS_TOKEN:
            print(
                "[QUEUE] GOOGLE_ACCESS_TOKEN is not set; cannot forward approved request."
            )
            APPROVAL_QUEUE.task_done()
            continue

        try:
            result = forward_to_google(method, url, headers, body_b64)
        except Exception as exc:
            print(f"[QUEUE] Failed to forward approved request: {exc}")
            APPROVAL_QUEUE.task_done()
            continue

        print(
            f"[QUEUE] Forwarded approved request: HTTP {result['status_code']} "
            f"({len(base64.b64decode(result['body'])) if result['body'] else 0} bytes)"
        )
        APPROVAL_QUEUE.task_done()


# ---------------------------------------------------------------------------
# POLICY ENGINE
# ---------------------------------------------------------------------------

def _parsed_url(url: str):
    """Return a urllib.parse.ParseResult for *url*."""
    return urllib.parse.urlparse(url)


def _query_params(url: str) -> dict:
    """Return query parameters as a flat dict (first value per key)."""
    parsed = _parsed_url(url)
    return {k: v[0] for k, v in urllib.parse.parse_qs(parsed.query).items()}


def check_denylist(method: str, url: str) -> tuple[bool, str]:
    """
    Check whether the request matches the denylist.

    Returns (denied, reason).  If denied is True the request must be blocked.
    """
    # 1a. Blocked HTTP methods.
    if method.upper() in DENY_METHODS:
        return True, f"HTTP method '{method}' is on the denylist"

    path = _parsed_url(url).path

    # 1b. Blocked URL path fragments.
    for fragment in DENY_PATH_FRAGMENTS:
        if fragment in path:
            return True, f"URL path contains denied fragment '{fragment}'"

    # 1c. Blocked query parameters.
    params = _query_params(url)
    for key, value in DENY_QUERY_PARAMS.items():
        if params.get(key) == value:
            return True, f"Query parameter '{key}={value}' is on the denylist"

    return False, ""


def check_allowlist(method: str, url: str) -> tuple[bool, str]:
    """
    Check whether the request matches the allowlist.

    Returns (allowed, reason).  If allowed is True the request can be
    forwarded immediately without human review.
    """
    method_upper = method.upper()

    # 2a. Safe HTTP methods (read-only).
    if method_upper in ALLOW_METHODS:
        return True, f"HTTP method '{method}' is on the allowlist (read-only)"

    path = _parsed_url(url).path

    # 2b. Allowed URL path fragments (any HTTP method).
    for fragment in ALLOW_PATH_FRAGMENTS:
        if fragment in path:
            return True, f"URL path contains allowed fragment '{fragment}'"

    # 2c. Method-specific path allowlist.
    allowed_paths_for_method = ALLOW_METHOD_PATH.get(method_upper, set())
    for fragment in allowed_paths_for_method:
        if fragment in path:
            return True, (
                f"HTTP method '{method}' with path fragment '{fragment}' "
                "is on the allowlist"
            )

    return False, ""


# ---------------------------------------------------------------------------
# GOOGLE FORWARDING
# ---------------------------------------------------------------------------

def forward_to_google(method: str, url: str, headers: dict, body_b64: str) -> dict:
    """
    Forward the request to Google and return a gateway response dict.

    Adds the Authorization header from GOOGLE_ACCESS_TOKEN.
    The response body is base64-encoded before being returned.
    """
    # Decode the base64-encoded body from gogcli.
    body_bytes = b""
    if body_b64:
        try:
            body_bytes = base64.b64decode(body_b64)
        except Exception:
            # If decoding fails, treat it as raw bytes.
            body_bytes = body_b64.encode("utf-8")

    # Build the forwarded headers.  We add Authorization, but preserve
    # everything else the CLI sent (e.g. Content-Type, Accept).
    forward_headers = dict(headers)
    if GOOGLE_ACCESS_TOKEN:
        forward_headers["Authorization"] = f"Bearer {GOOGLE_ACCESS_TOKEN}"

    # Forward the request to Google.
    google_resp = req_lib.request(
        method=method,
        url=url,
        headers=forward_headers,
        data=body_bytes if body_bytes else None,
        # Follow redirects so that the final resolved response is returned to
        # gogcli (e.g. Google occasionally issues 302s for uploads/downloads).
        allow_redirects=True,
        timeout=60,
    )

    # Base64-encode the response body before returning.
    encoded_body = base64.b64encode(google_resp.content).decode("ascii")

    # Flatten the response headers to a plain dict.
    resp_headers = dict(google_resp.headers)

    return {
        "status_code": google_resp.status_code,
        "headers": resp_headers,
        "body": encoded_body,
    }


# ---------------------------------------------------------------------------
# HUMAN-IN-THE-LOOP PROMPT
# ---------------------------------------------------------------------------

def _format_request_for_human(method: str, url: str, headers: dict, body_b64: str) -> str:
    """Build a human-readable summary of the intercepted request."""
    lines = [
        "",
        "=" * 70,
        "  INTERCEPTED GOOGLE API REQUEST — HUMAN APPROVAL REQUIRED",
        "=" * 70,
        f"  Method : {method}",
        f"  URL    : {url}",
    ]

    if headers:
        lines.append("  Headers:")
        for k, v in sorted(headers.items()):
            # Truncate long values (e.g. bearer tokens).
            display_v = v if len(v) <= 80 else v[:77] + "..."
            lines.append(f"    {k}: {display_v}")

    if body_b64:
        try:
            decoded = base64.b64decode(body_b64).decode("utf-8", errors="replace")
            # Show at most 500 characters of the body.
            preview = decoded[:500]
            if len(decoded) > 500:
                preview += f"  ... [{len(decoded) - 500} more bytes]"
            lines.append(f"  Body   : {preview}")
        except Exception:
            lines.append(f"  Body   : <base64: {body_b64[:80]}...>")

    lines.append("=" * 70)
    return "\n".join(lines)


def ask_human_approval(method: str, url: str, headers: dict, body_b64: str) -> bool:
    """
    Print the request to stdout and prompt the operator for approval.

    Returns True if the human approves, False on denial or timeout.
    The prompt automatically times out after APPROVAL_TIMEOUT_SECONDS.
    """
    summary = _format_request_for_human(method, url, headers, body_b64)
    print(summary, flush=True)
    print(
        f"  Approve this request? [y/N] "
        f"(auto-deny in {APPROVAL_TIMEOUT_SECONDS}s): ",
        end="",
        flush=True,
    )

    # Use a threading.Event + daemon thread so we can implement the timeout
    # without depending on select() (which would break on Windows).
    answer_container: list[str] = []
    event = threading.Event()

    def _read_input():
        try:
            line = sys.stdin.readline()
            answer_container.append(line.strip())
        except Exception:
            answer_container.append("")
        finally:
            event.set()

    reader = threading.Thread(target=_read_input, daemon=True)
    reader.start()

    got_input = event.wait(timeout=APPROVAL_TIMEOUT_SECONDS)

    if not got_input:
        print("\n  [TIMEOUT] No response received — denying request.", flush=True)
        return False

    answer = answer_container[0].lower() if answer_container else ""
    approved = answer in ("y", "yes")

    if approved:
        print("  [APPROVED] Forwarding to Google.", flush=True)
    else:
        print("  [DENIED] Request blocked.", flush=True)

    return approved


# ---------------------------------------------------------------------------
# HTTP REQUEST HANDLER
# ---------------------------------------------------------------------------

class GatewayHandler(BaseHTTPRequestHandler):
    """
    Handles POST /webhook requests from gogcli.

    Every other path or method returns an appropriate error.

    NOTE — single-threaded request handling: this server uses plain HTTPServer
    (not ThreadingMixIn), so request handling itself is serialized. HITL prompt
    work is offloaded to a background queue worker so webhook responses return
    immediately with an inner 202 pending response.
    """

    # Silence the default per-request access log lines; we print our own.
    def log_message(self, fmt, *args):  # noqa: D401
        pass

    def _send_json_response(self, status: int, body: dict) -> None:
        """Serialise *body* as JSON and write it to the response."""
        encoded = json.dumps(body).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def _send_plain_response(self, status: int, text: str) -> None:
        """Write a plain-text response."""
        encoded = text.encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def _authenticate(self) -> bool:
        """
        Check the Authorization header when GATEWAY_SECRET is configured.

        Returns True if auth passes (or is not required), False otherwise.
        """
        if not GATEWAY_SECRET:
            # No secret configured — auth check is disabled.
            return True

        auth_header = self.headers.get("Authorization", "")
        expected = f"Bearer {GATEWAY_SECRET}"
        if auth_header != expected:
            return False
        return True

    def _read_body(self) -> bytes:
        """Read the full request body using the Content-Length header."""
        length = int(self.headers.get("Content-Length", "0"))
        if length <= 0:
            return b""
        return self.rfile.read(length)

    # --- Routing ------------------------------------------------------------

    def do_GET(self):  # noqa: N802
        """Health-check endpoint."""
        if self.path == "/health":
            self._send_plain_response(200, "ok\n")
        else:
            self._send_plain_response(404, "not found\n")

    def do_POST(self):  # noqa: N802
        """Handle POST /webhook — the main gateway entry point."""
        if self.path != "/webhook":
            self._send_plain_response(404, "not found\n")
            return

        # --- Authentication -------------------------------------------------
        if not self._authenticate():
            print("[AUTH] Request rejected: missing or invalid Authorization header.")
            self._send_plain_response(401, "Unauthorized\n")
            return

        # --- Parse the incoming payload -------------------------------------
        raw_body = self._read_body()
        try:
            payload = json.loads(raw_body)
        except json.JSONDecodeError as exc:
            print(f"[ERROR] Failed to parse JSON payload: {exc}")
            self._send_plain_response(400, f"Bad JSON: {exc}\n")
            return

        method = payload.get("method", "").upper()
        url = payload.get("url", "")
        headers = payload.get("headers", {})
        body_b64 = payload.get("body", "")

        print(f"[RECV] {method} {url}")

        # --- Stage 1: Denylist check ----------------------------------------
        denied, deny_reason = check_denylist(method, url)
        if denied:
            print(f"[DENY] {deny_reason}")
            self._send_json_response(
                200, denied_webhook_response(f"Forbidden: {deny_reason}")
            )
            return

        # --- Stage 2: Allowlist check ---------------------------------------
        allowed, allow_reason = check_allowlist(method, url)
        if allowed:
            print(f"[ALLOW] {allow_reason}")
            self._forward_and_respond(method, url, headers, body_b64)
            return

        # --- Stage 3: Human-in-the-loop ------------------------------------
        print(f"[HOLD] Request requires human approval: {method} {url}")
        enqueue_approval_task(method, url, headers, body_b64)
        self._send_json_response(200, pending_webhook_response())

    # --- Helpers ------------------------------------------------------------

    def _forward_and_respond(
        self, method: str, url: str, headers: dict, body_b64: str
    ) -> None:
        """
        Forward the request to Google and write the result back to gogcli.

        On any forwarding error, return HTTP 502 so gogcli surfaces a clear
        message rather than a silent hang.
        """
        if not GOOGLE_ACCESS_TOKEN:
            msg = (
                "GOOGLE_ACCESS_TOKEN is not set. "
                "Set it to a valid Bearer token to forward requests."
            )
            print(f"[ERROR] {msg}")
            self._send_plain_response(502, f"Bad Gateway: {msg}\n")
            return

        try:
            result = forward_to_google(method, url, headers, body_b64)
        except Exception as exc:
            print(f"[ERROR] Failed to forward to Google: {exc}")
            self._send_plain_response(502, f"Bad Gateway: {exc}\n")
            return

        print(
            f"[RESP] HTTP {result['status_code']} "
            f"({len(base64.b64decode(result['body'])) if result['body'] else 0} bytes)"
        )

        # Return the gateway response as JSON to gogcli.
        self._send_json_response(200, result)


# ---------------------------------------------------------------------------
# SERVER ENTRY POINT
# ---------------------------------------------------------------------------

def run_server() -> None:
    """Start the blocking HTTP server."""
    server_address = ("", PORT)
    httpd = HTTPServer(server_address, GatewayHandler)
    approval_worker = threading.Thread(target=process_approval_queue, daemon=True)
    approval_worker.start()

    print("=" * 70)
    print(" gogcli-gateway Python example server")
    print("=" * 70)
    print(f"  Listening on       : http://0.0.0.0:{PORT}")
    print(f"  Webhook endpoint   : http://0.0.0.0:{PORT}/webhook")
    print(f"  Health endpoint    : http://0.0.0.0:{PORT}/health")
    print(f"  Auth required      : {'yes' if GATEWAY_SECRET else 'no (GATEWAY_SECRET not set)'}")
    print(f"  Google token set   : {'yes' if GOOGLE_ACCESS_TOKEN else 'NO — set GOOGLE_ACCESS_TOKEN'}")
    print(f"  Approval timeout   : {APPROVAL_TIMEOUT_SECONDS}s")
    print()
    print("  Set GOG_WEBHOOK_URL=http://localhost:{} to route gogcli here.".format(PORT))
    print("=" * 70)
    print()

    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down.")
        httpd.server_close()


if __name__ == "__main__":
    run_server()
