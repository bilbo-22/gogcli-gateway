package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecute_GmailSearch_Text(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/users/me/threads") && !strings.Contains(path, "/users/me/threads/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"threads":       []map[string]any{{"id": "t1"}},
				"nextPageToken": "npt",
			})
			return
		case strings.Contains(path, "/users/me/threads/t1"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "t1",
				"messages": []map[string]any{
					{
						"id":       "m1",
						"labelIds": []string{"INBOX"},
						"payload": map[string]any{
							"headers": []map[string]any{
								{"name": "From", "value": "Me <me@example.com>"},
								{"name": "Subject", "value": "Hello"},
								{"name": "Date", "value": "Mon, 02 Jan 2006 15:04:05 -0700"},
							},
						},
					},
				},
			})
			return
		case strings.Contains(path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{
					{"id": "INBOX", "name": "INBOX", "type": "system"},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "search", "newer_than:7d", "--max", "1"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stdout, "ID") || !strings.Contains(result.stdout, "Hello") {
		t.Fatalf("unexpected output: %q", result.stdout)
	}
}

func TestExecute_GmailSearch_Text_NoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/users/me/threads") && !strings.Contains(path, "/users/me/threads/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"threads": []map[string]any{},
			})
			return
		case strings.Contains(path, "/users/me/labels"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"labels": []map[string]any{},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "search", "newer_than:7d"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}
	if !strings.Contains(result.stderr, "No results") {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
}

func TestExecute_GmailSearch_AppliesSystemLabelFilters(t *testing.T) {
	var gotQuery string
	var gotLabels []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/users/me/threads") && !strings.Contains(path, "/users/me/threads/"):
			gotQuery = r.URL.Query().Get("q")
			gotLabels = r.URL.Query()["labelIds"]
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"threads": []map[string]any{},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	result := executeWithGmailTestService(
		t,
		[]string{"--plain", "--account", "a@b.com", "gmail", "search", "in:spam is:unread", "--max", "1000"},
		newGmailServiceFromServer(t, srv),
	)
	if result.err != nil {
		t.Fatalf("Execute: %v\nstderr=%q", result.err, result.stderr)
	}

	if gotQuery != "in:spam is:unread" {
		t.Fatalf("unexpected query: %q", gotQuery)
	}
	assertSameStrings(t, gotLabels, []string{"SPAM", "UNREAD"})
}
