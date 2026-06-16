package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/calendar/v3"
)

func TestExecuteCalendarUnsubscribeJSON(t *testing.T) {
	deleteCalls := 0
	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/users/me/calendarList/team@example.com") {
			http.NotFound(w, r)
			return
		}
		deleteCalls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer closeSvc()

	result := executeWithCalendarTestService(t, []string{
		"--json",
		"--force",
		"--account", "a@b.com",
		"calendar", "unsubscribe", "team@example.com",
	}, svc)
	if result.err != nil {
		t.Fatalf("unsubscribe: %v", result.err)
	}
	if deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", deleteCalls)
	}

	var payload struct {
		Unsubscribed bool   `json:"unsubscribed"`
		CalendarID   string `json:"calendarId"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !payload.Unsubscribed || payload.CalendarID != "team@example.com" {
		t.Fatalf("unexpected output: %#v", payload)
	}
}

func TestExecuteCalendarDeleteCalendarJSON(t *testing.T) {
	deleteCalls := 0
	svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, "/calendars/owned@example.com") {
			http.NotFound(w, r)
			return
		}
		deleteCalls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer closeSvc()

	result := executeWithCalendarTestService(t, []string{
		"--json",
		"--force",
		"--account", "a@b.com",
		"calendar", "delete-calendar", "owned@example.com",
	}, svc)
	if result.err != nil {
		t.Fatalf("delete calendar: %v", result.err)
	}
	if deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", deleteCalls)
	}

	var payload struct {
		Deleted    bool   `json:"deleted"`
		CalendarID string `json:"calendarId"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !payload.Deleted || payload.CalendarID != "owned@example.com" {
		t.Fatalf("unexpected output: %#v", payload)
	}
}

func TestCalendarLifecycleDryRunSkipsService(t *testing.T) {
	setupCalendarAliasHome(t)
	const resolvedID = "resolved@group.calendar.google.com"
	if err := defaultConfigStoreForTest(t).SetCalendarAlias("shortcut", resolvedID); err != nil {
		t.Fatalf("SetCalendarAlias: %v", err)
	}

	tests := []struct {
		name string
		args []string
		op   string
		id   string
	}{
		{
			name: "unsubscribe",
			args: []string{"calendar", "unsubscribe", "shortcut"},
			op:   "calendar.unsubscribe",
			id:   resolvedID,
		},
		{
			name: "delete calendar",
			args: []string{"calendar", "delete-calendar", "shortcut"},
			op:   "calendar.delete-calendar",
			id:   resolvedID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := executeWithCalendarTestServiceFactory(t,
				append([]string{"--json", "--dry-run", "--account", "a@b.com"}, tc.args...),
				func(context.Context, string) (*calendar.Service, error) {
					t.Fatal("calendar service opened during dry-run")
					return nil, errors.New("unexpected calendar service call")
				},
			)
			if result.err != nil {
				t.Fatalf("dry-run error = %v, want success", result.err)
			}

			var payload struct {
				DryRun  bool   `json:"dry_run"`
				Op      string `json:"op"`
				Request struct {
					CalendarID string `json:"calendar_id"`
				} `json:"request"`
			}
			if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
				t.Fatalf("decode dry-run: %v", err)
			}
			if !payload.DryRun || payload.Op != tc.op || payload.Request.CalendarID != tc.id {
				t.Fatalf("unexpected dry-run output: %#v", payload)
			}
		})
	}
}

func TestCalendarLifecycleRequiresForceNonInteractive(t *testing.T) {
	setupCalendarAliasHome(t)
	const resolvedID = "resolved@group.calendar.google.com"
	if err := defaultConfigStoreForTest(t).SetCalendarAlias("shortcut", resolvedID); err != nil {
		t.Fatalf("SetCalendarAlias: %v", err)
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "unsubscribe",
			args: []string{"--no-input", "--account", "a@b.com", "calendar", "unsubscribe", "shortcut"},
		},
		{
			name: "delete calendar",
			args: []string{"--no-input", "--account", "a@b.com", "calendar", "delete-calendar", "shortcut"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := executeWithCalendarTestServiceFactory(t, tc.args, func(context.Context, string) (*calendar.Service, error) {
				t.Fatal("calendar service opened before confirmation")
				return nil, errors.New("unexpected calendar service call")
			})
			if result.err == nil || ExitCode(result.err) != 2 || !strings.Contains(result.err.Error(), "--force") {
				t.Fatalf("unexpected confirmation error: %v", result.err)
			}
			if !strings.Contains(result.err.Error(), resolvedID) {
				t.Fatalf("confirmation target = %q, want resolved ID %q", result.err, resolvedID)
			}
		})
	}
}

func TestCalendarLifecycleAPIErrors(t *testing.T) {
	tests := []struct {
		name string
		path string
		args []string
	}{
		{
			name: "unsubscribe",
			path: "/users/me/calendarList/team@example.com",
			args: []string{"calendar", "unsubscribe", "team@example.com"},
		},
		{
			name: "delete calendar",
			path: "/calendars/owned@example.com",
			args: []string{"calendar", "delete-calendar", "owned@example.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, closeSvc := newCalendarServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete || !strings.Contains(r.URL.Path, tc.path) {
					http.NotFound(w, r)
					return
				}
				http.Error(w, "denied", http.StatusForbidden)
			}))
			defer closeSvc()

			result := executeWithCalendarTestService(t,
				append([]string{"--force", "--account", "a@b.com"}, tc.args...),
				svc,
			)
			if result.err == nil || ExitCode(result.err) != exitCodePermissionDenied {
				t.Fatalf("error = %v, want permission denied", result.err)
			}
		})
	}
}
