package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/sheets/v4"
)

func newSheetsTestServer(t *testing.T, batchUpdateCapture *sheets.BatchUpdateSpreadsheetRequest, sheetsCatalog []map[string]any) (*sheets.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")

		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && !strings.Contains(path, ":batchUpdate") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets":        sheetsCatalog,
			})
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(batchUpdateCapture); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	return newSheetsServiceFromServer(t, srv), func() {}
}

func newSheetsCmdContext(t *testing.T, svc *sheets.Service) context.Context {
	t.Helper()
	return withSheetsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
}

func TestSheetsReorderTabCmd_ResolvesByName(t *testing.T) {
	var captured sheets.BatchUpdateSpreadsheetRequest
	svc, cleanup := newSheetsTestServer(t, &captured, []map[string]any{
		{"properties": map[string]any{"sheetId": 11, "title": "First", "index": 0}},
		{"properties": map[string]any{"sheetId": 22, "title": "Second", "index": 1}},
		{"properties": map[string]any{"sheetId": 33, "title": "Third", "index": 2}},
	})
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newSheetsCmdContext(t, svc)

	if err := runKong(t, &SheetsReorderTabCmd{}, []string{"s1", "--tab", "Third", "--to", "0"}, ctx, flags); err != nil {
		t.Fatalf("reorder-tab: %v", err)
	}

	if len(captured.Requests) != 1 {
		t.Fatalf("expected 1 batchUpdate request, got %d", len(captured.Requests))
	}
	req := captured.Requests[0].UpdateSheetProperties
	if req == nil {
		t.Fatalf("expected UpdateSheetProperties, got %#v", captured.Requests[0])
	}
	if req.Properties == nil || req.Properties.SheetId != 33 {
		t.Fatalf("expected sheetId=33 (resolved from \"Third\"), got %#v", req.Properties)
	}
	if req.Properties.Index != 0 {
		t.Fatalf("expected target index 0, got %d", req.Properties.Index)
	}
	if req.Fields != "index" {
		t.Fatalf("expected Fields=\"index\", got %q", req.Fields)
	}
}

func TestSheetsReorderTabCmd_AcceptsNumericSheetID(t *testing.T) {
	var captured sheets.BatchUpdateSpreadsheetRequest
	svc, cleanup := newSheetsTestServer(t, &captured, []map[string]any{
		{"properties": map[string]any{"sheetId": 99, "title": "Only", "index": 0}},
		{"properties": map[string]any{"sheetId": 100, "title": "Next", "index": 1}},
		{"properties": map[string]any{"sheetId": 101, "title": "Last", "index": 2}},
	})
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newSheetsCmdContext(t, svc)

	if err := runKong(t, &SheetsReorderTabCmd{}, []string{"s1", "--tab", "99", "--to", "2"}, ctx, flags); err != nil {
		t.Fatalf("reorder-tab: %v", err)
	}

	req := captured.Requests[0].UpdateSheetProperties
	if req == nil || req.Properties == nil {
		t.Fatalf("expected UpdateSheetProperties, got %#v", captured.Requests[0])
	}
	if req.Properties.SheetId != 99 {
		t.Fatalf("expected numeric sheetId passed through, got %d", req.Properties.SheetId)
	}
	if req.Properties.Index != 3 {
		t.Fatalf("expected rightward move to send API index 3, got %d", req.Properties.Index)
	}
}

func TestSheetsReorderTabCmd_RightwardMoveAdjustsAPIIndex(t *testing.T) {
	var captured sheets.BatchUpdateSpreadsheetRequest
	svc, cleanup := newSheetsTestServer(t, &captured, []map[string]any{
		{"properties": map[string]any{"sheetId": 11, "title": "First", "index": 0}},
		{"properties": map[string]any{"sheetId": 22, "title": "Second", "index": 1}},
		{"properties": map[string]any{"sheetId": 33, "title": "Third", "index": 2}},
	})
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newSheetsCmdContext(t, svc)

	if err := runKong(t, &SheetsReorderTabCmd{}, []string{"s1", "--tab", "First", "--to", "1"}, ctx, flags); err != nil {
		t.Fatalf("reorder-tab: %v", err)
	}

	req := captured.Requests[0].UpdateSheetProperties
	if req == nil || req.Properties == nil {
		t.Fatalf("expected UpdateSheetProperties, got %#v", captured.Requests[0])
	}
	if req.Properties.SheetId != 11 {
		t.Fatalf("expected sheetId=11, got %d", req.Properties.SheetId)
	}
	if req.Properties.Index != 2 {
		t.Fatalf("expected rightward move to final index 1 to send API index 2, got %d", req.Properties.Index)
	}
}

func TestSheetsReorderTabCmd_PrefersNumericTitleBeforeSheetID(t *testing.T) {
	var captured sheets.BatchUpdateSpreadsheetRequest
	svc, cleanup := newSheetsTestServer(t, &captured, []map[string]any{
		{"properties": map[string]any{"sheetId": 11, "title": "2024", "index": 0}},
		{"properties": map[string]any{"sheetId": 2024, "title": "Other", "index": 1}},
		{"properties": map[string]any{"sheetId": 33, "title": "Last", "index": 2}},
	})
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newSheetsCmdContext(t, svc)

	if err := runKong(t, &SheetsReorderTabCmd{}, []string{"s1", "--tab", "2024", "--to", "2"}, ctx, flags); err != nil {
		t.Fatalf("reorder-tab: %v", err)
	}

	req := captured.Requests[0].UpdateSheetProperties
	if req == nil || req.Properties == nil {
		t.Fatalf("expected UpdateSheetProperties, got %#v", captured.Requests[0])
	}
	if req.Properties.SheetId != 11 {
		t.Fatalf("expected numeric-looking title to resolve before sheetId, got sheetId %d", req.Properties.SheetId)
	}
	if req.Properties.Index != 3 {
		t.Fatalf("expected rightward move to send API index 3, got %d", req.Properties.Index)
	}
}

func TestSheetsReorderTabCmd_IndexZeroIsSerialized(t *testing.T) {
	// Index=0 is the leftmost position and is also Go's zero value for int64.
	// Without ForceSendFields the JSON wire format would omit it and the API
	// would treat the call as a no-op move-to-current-position. We can't see
	// ForceSendFields after a round-trip decode, so we capture the raw request
	// body and assert "index":0 appears in the JSON.
	var rawBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && !strings.Contains(path, ":batchUpdate") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 11, "title": "First", "index": 0}},
					{"properties": map[string]any{"sheetId": 22, "title": "Second", "index": 1}},
				},
			})
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			rawBody = body
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := newSheetsServiceFromServer(t, srv)

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newSheetsCmdContext(t, svc)

	if err := runKong(t, &SheetsReorderTabCmd{}, []string{"s1", "--tab", "Second", "--to", "0"}, ctx, flags); err != nil {
		t.Fatalf("reorder-tab: %v", err)
	}

	if !strings.Contains(string(rawBody), `"index":0`) {
		t.Fatalf("expected raw body to contain \"index\":0 (ForceSendFields was not effective); body = %s", rawBody)
	}
	if !strings.Contains(string(rawBody), `"fields":"index"`) {
		t.Fatalf("expected raw body to contain \"fields\":\"index\"; body = %s", rawBody)
	}
}

func TestSheetsReorderTabCmd_SheetIDZeroIsSerialized(t *testing.T) {
	var rawBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && !strings.Contains(path, ":batchUpdate") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 0, "title": "First", "index": 0}},
					{"properties": map[string]any{"sheetId": 22, "title": "Second", "index": 1}},
				},
			})
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			rawBody = body
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	svc := newSheetsServiceFromServer(t, srv)

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newSheetsCmdContext(t, svc)

	if err := runKong(t, &SheetsReorderTabCmd{}, []string{"s1", "--tab", "First", "--to", "1"}, ctx, flags); err != nil {
		t.Fatalf("reorder-tab: %v", err)
	}

	body := string(rawBody)
	if !strings.Contains(body, `"sheetId":0`) {
		t.Fatalf("expected raw body to contain \"sheetId\":0; body = %s", rawBody)
	}
	if !strings.Contains(body, `"index":2`) {
		t.Fatalf("expected rightward final index 1 to send API index 2; body = %s", rawBody)
	}
}

func TestSheetsReorderTabCmd_UnknownTabName(t *testing.T) {
	var captured sheets.BatchUpdateSpreadsheetRequest
	svc, cleanup := newSheetsTestServer(t, &captured, []map[string]any{
		{"properties": map[string]any{"sheetId": 1, "title": "Sheet1", "index": 0}},
	})
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newSheetsCmdContext(t, svc)

	err := runKong(t, &SheetsReorderTabCmd{}, []string{"s1", "--tab", "Nope", "--to", "1"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `unknown tab "Nope"`) {
		t.Fatalf("expected unknown-tab error, got %v", err)
	}
}

func TestSheetsReorderTabCmd_UnknownNumericSheetID(t *testing.T) {
	var captured sheets.BatchUpdateSpreadsheetRequest
	svc, cleanup := newSheetsTestServer(t, &captured, []map[string]any{
		{"properties": map[string]any{"sheetId": 1, "title": "Sheet1", "index": 0}},
	})
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newSheetsCmdContext(t, svc)

	err := runKong(t, &SheetsReorderTabCmd{}, []string{"s1", "--tab", "99", "--to", "0"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "unknown sheetId 99") {
		t.Fatalf("expected unknown-sheetId error, got %v", err)
	}
}

func TestSheetsReorderTabCmd_IndexOutOfRangeRejected(t *testing.T) {
	var captured sheets.BatchUpdateSpreadsheetRequest
	svc, cleanup := newSheetsTestServer(t, &captured, []map[string]any{
		{"properties": map[string]any{"sheetId": 1, "title": "Sheet1", "index": 0}},
		{"properties": map[string]any{"sheetId": 2, "title": "Sheet2", "index": 1}},
	})
	defer cleanup()

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newSheetsCmdContext(t, svc)

	err := runKong(t, &SheetsReorderTabCmd{}, []string{"s1", "--tab", "Sheet1", "--to", "2"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "--to must be between 0 and 1") {
		t.Fatalf("expected range validation error, got %v", err)
	}
}

func TestSheetsReorderTabCmd_NegativeIndexRejected(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	ctx := newCmdRuntimeOutputContext(t, io.Discard, io.Discard)
	err := runKong(t, &SheetsReorderTabCmd{}, []string{"s1", "--tab", "x", "--to=-1"}, ctx, flags)
	if err == nil || !strings.Contains(err.Error(), "--to must be >= 0") {
		t.Fatalf("expected --to validation error, got %v", err)
	}
}
