package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/sheets/v4"
)

func TestSheetsMergeCmds(t *testing.T) {
	var gotRequest *sheets.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/sheets/v4"), "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 9, "title": "Sheet1"}},
				},
			})
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 {
				t.Fatalf("expected one request, got %#v", req.Requests)
			}
			gotRequest = req.Requests[0]
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc := newSheetsServiceFromServer(t, srv)

	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSheetsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	t.Run("merge", func(t *testing.T) {
		gotRequest = nil
		cmd := &SheetsMergeCmd{}
		if err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B2", "--type", "merge_rows"}, ctx, flags); err != nil {
			t.Fatalf("merge: %v", err)
		}
		if gotRequest == nil || gotRequest.MergeCells == nil {
			t.Fatalf("expected merge request, got %#v", gotRequest)
		}
		if gotRequest.MergeCells.MergeType != "MERGE_ROWS" {
			t.Fatalf("unexpected merge type: %#v", gotRequest.MergeCells)
		}
	})

	t.Run("unmerge", func(t *testing.T) {
		gotRequest = nil
		cmd := &SheetsUnmergeCmd{}
		if err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B2"}, ctx, flags); err != nil {
			t.Fatalf("unmerge: %v", err)
		}
		if gotRequest == nil || gotRequest.UnmergeCells == nil {
			t.Fatalf("expected unmerge request, got %#v", gotRequest)
		}
	})

	t.Run("invalid merge type is usage", func(t *testing.T) {
		gotRequest = nil
		cmd := &SheetsMergeCmd{}
		err := runKong(t, cmd, []string{"s1", "Sheet1!A1:B2", "--type", "BOGUS"}, ctx, flags)
		if err == nil || !strings.Contains(err.Error(), "invalid --type") {
			t.Fatalf("expected invalid type error, got %v", err)
		}
		if got := ExitCode(err); got != 2 {
			t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
		}
		if gotRequest != nil {
			t.Fatal("did not expect an API request")
		}
	})
}
