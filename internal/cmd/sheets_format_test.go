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

func newSheetsFormatTestContext(t *testing.T, handler http.Handler) context.Context {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	svc := newSheetsServiceFromServer(t, srv)
	return withSheetsTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)
}

func TestSheetsFormatCmd(t *testing.T) {
	var gotRepeat *sheets.RepeatCellRequest

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 42, "title": "Sheet1"}},
				},
			})
			return
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].RepeatCell == nil {
				t.Fatalf("expected repeatCell request, got %#v", req.Requests)
			}
			gotRepeat = req.Requests[0].RepeatCell
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
			return
		}
	})

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newSheetsFormatTestContext(t, handler)
	cmd := &SheetsFormatCmd{}
	if err := runKong(t, cmd, []string{
		"s1",
		"Sheet1!B2:C3",
		"--format-json", `{"textFormat":{"bold":true}}`,
	}, ctx, flags); err != nil {
		t.Fatalf("format: %v", err)
	}

	if gotRepeat == nil {
		t.Fatal("expected repeatCell request")
	}
	if gotRepeat.Fields != "userEnteredFormat.textFormat.bold" {
		t.Fatalf("unexpected fields: %s", gotRepeat.Fields)
	}
	if gotRepeat.Range == nil {
		t.Fatalf("missing range")
	}
	if gotRepeat.Range.SheetId != 42 {
		t.Fatalf("unexpected sheet id: %d", gotRepeat.Range.SheetId)
	}
	if gotRepeat.Range.StartRowIndex != 1 || gotRepeat.Range.EndRowIndex != 3 {
		t.Fatalf("unexpected row range: %#v", gotRepeat.Range)
	}
	if gotRepeat.Range.StartColumnIndex != 1 || gotRepeat.Range.EndColumnIndex != 3 {
		t.Fatalf("unexpected column range: %#v", gotRepeat.Range)
	}
	if gotRepeat.Cell == nil || gotRepeat.Cell.UserEnteredFormat == nil || gotRepeat.Cell.UserEnteredFormat.TextFormat == nil {
		t.Fatalf("missing format data: %#v", gotRepeat.Cell)
	}
	if !gotRepeat.Cell.UserEnteredFormat.TextFormat.Bold {
		t.Fatalf("expected bold text format, got %#v", gotRepeat.Cell.UserEnteredFormat.TextFormat)
	}
}

func TestSheetsFormatCmdNamedRange(t *testing.T) {
	var gotRepeat *sheets.RepeatCellRequest

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 42, "title": "Sheet1"}},
				},
				"namedRanges": []map[string]any{
					{
						"namedRangeId": "nr1",
						"name":         "MyNamedRange",
						"range": map[string]any{
							"sheetId":          42,
							"startRowIndex":    0,
							"endRowIndex":      2,
							"startColumnIndex": 1,
							"endColumnIndex":   3,
						},
					},
				},
			})
			return
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].RepeatCell == nil {
				t.Fatalf("expected repeatCell request, got %#v", req.Requests)
			}
			gotRepeat = req.Requests[0].RepeatCell
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
			return
		}
	})

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newSheetsFormatTestContext(t, handler)
	cmd := &SheetsFormatCmd{}
	if err := runKong(t, cmd, []string{
		"s1",
		"MyNamedRange",
		"--format-json", `{"textFormat":{"bold":true}}`,
		"--format-fields", "textFormat.bold",
	}, ctx, flags); err != nil {
		t.Fatalf("format: %v", err)
	}

	if gotRepeat == nil || gotRepeat.Range == nil {
		t.Fatalf("expected repeatCell range, got %#v", gotRepeat)
	}
	if gotRepeat.Range.SheetId != 42 {
		t.Fatalf("unexpected sheet id: %d", gotRepeat.Range.SheetId)
	}
	if gotRepeat.Range.StartRowIndex != 0 || gotRepeat.Range.EndRowIndex != 2 {
		t.Fatalf("unexpected row range: %#v", gotRepeat.Range)
	}
	if gotRepeat.Range.StartColumnIndex != 1 || gotRepeat.Range.EndColumnIndex != 3 {
		t.Fatalf("unexpected column range: %#v", gotRepeat.Range)
	}
}

func TestSheetsFormatCmd_BordersTopStyle(t *testing.T) {
	var gotRepeat *sheets.RepeatCellRequest

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId": "s1",
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 42, "title": "Sheet1"}},
				},
			})
			return
		case strings.Contains(path, "/spreadsheets/s1:batchUpdate") && r.Method == http.MethodPost:
			var req sheets.BatchUpdateSpreadsheetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode batchUpdate: %v", err)
			}
			if len(req.Requests) != 1 || req.Requests[0].RepeatCell == nil {
				t.Fatalf("expected repeatCell request, got %#v", req.Requests)
			}
			gotRepeat = req.Requests[0].RepeatCell
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
			return
		default:
			http.NotFound(w, r)
			return
		}
	})

	flags := &RootFlags{Account: "a@b.com"}
	ctx := newSheetsFormatTestContext(t, handler)
	cmd := &SheetsFormatCmd{}
	if err := runKong(t, cmd, []string{
		"s1",
		"Sheet1!B2:C3",
		"--format-json", `{"borders":{"top":{"style":"SOLID"}}}`,
		"--format-fields", "borders.top.style",
	}, ctx, flags); err != nil {
		t.Fatalf("format: %v", err)
	}

	if gotRepeat == nil {
		t.Fatal("expected repeatCell request")
	}
	if gotRepeat.Fields != "userEnteredFormat.borders.top.style" {
		t.Fatalf("unexpected fields: %s", gotRepeat.Fields)
	}
	if gotRepeat.Cell == nil || gotRepeat.Cell.UserEnteredFormat == nil || gotRepeat.Cell.UserEnteredFormat.Borders == nil || gotRepeat.Cell.UserEnteredFormat.Borders.Top == nil {
		t.Fatalf("missing border data: %#v", gotRepeat.Cell)
	}
	if gotRepeat.Cell.UserEnteredFormat.Borders.Top.Style != "SOLID" {
		t.Fatalf("expected SOLID top border, got %#v", gotRepeat.Cell.UserEnteredFormat.Borders.Top)
	}
}
