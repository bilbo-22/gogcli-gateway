package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSheetsResizeCmds(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/sheets/v4"), "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 0, "title": "Sheet1"}},
				},
			})
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode body: %v", err)
			}
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

	t.Run("resize columns force-sends zero sheet and start index", func(t *testing.T) {
		gotBody = nil
		cmd := &SheetsResizeColumnsCmd{}
		if err := runKong(t, cmd, []string{"s1", "A:C", "--width", "120"}, ctx, flags); err != nil {
			t.Fatalf("resize-columns: %v", err)
		}
		requests := gotBody["requests"].([]any)
		update := requests[0].(map[string]any)["updateDimensionProperties"].(map[string]any)
		rng := update["range"].(map[string]any)
		if _, ok := rng["sheetId"]; !ok {
			t.Fatalf("expected sheetId to be sent: %#v", rng)
		}
		if v, ok := rng["startIndex"]; !ok || v != float64(0) {
			t.Fatalf("expected startIndex=0, got %#v", rng)
		}
		if v, ok := rng["endIndex"]; !ok || v != float64(3) {
			t.Fatalf("expected endIndex=3, got %#v", rng)
		}
	})

	t.Run("resize rows auto", func(t *testing.T) {
		gotBody = nil
		cmd := &SheetsResizeRowsCmd{}
		if err := runKong(t, cmd, []string{"s1", "1:3", "--auto"}, ctx, flags); err != nil {
			t.Fatalf("resize-rows: %v", err)
		}
		requests := gotBody["requests"].([]any)
		auto := requests[0].(map[string]any)["autoResizeDimensions"].(map[string]any)
		rng := auto["dimensions"].(map[string]any)
		if _, ok := rng["sheetId"]; !ok {
			t.Fatalf("expected sheetId to be sent: %#v", rng)
		}
		if v, ok := rng["startIndex"]; !ok || v != float64(0) {
			t.Fatalf("expected startIndex=0, got %#v", rng)
		}
		if v, ok := rng["endIndex"]; !ok || v != float64(3) {
			t.Fatalf("expected endIndex=3, got %#v", rng)
		}
	})
}

func TestSheetsResizeCmds_InvalidRangesAreUsageErrors(t *testing.T) {
	flags := &RootFlags{Account: "a@b.com"}
	for _, tc := range []struct {
		name string
		cmd  any
		args []string
	}{
		{
			name: "columns",
			cmd:  &SheetsResizeColumnsCmd{},
			args: []string{"s1", "1:3", "--width", "120"},
		},
		{
			name: "rows",
			cmd:  &SheetsResizeRowsCmd{},
			args: []string{"s1", "A:C", "--height", "24"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := runKong(t, tc.cmd, tc.args, context.Background(), flags)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := ExitCode(err); got != 2 {
				t.Fatalf("ExitCode = %d, want 2 (err=%v)", got, err)
			}
		})
	}
}
