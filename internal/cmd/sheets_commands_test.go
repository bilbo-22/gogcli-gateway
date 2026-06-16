package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSheetsCommands_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.Contains(path, "/spreadsheets/s1/values/") && strings.Contains(path, ":append") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updates": map[string]any{"updatedCells": 1},
			})
			return
		case strings.Contains(path, "/spreadsheets/s1/values/") && strings.Contains(path, ":clear") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"clearedRange": "Sheet1!A1",
			})
			return
		case strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updatedCells": 2,
			})
			return
		case strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"range":  "Sheet1!A1:B2",
				"values": [][]any{{"a", "b"}},
			})
			return
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "s1",
				"spreadsheetUrl": "http://example.com/s1",
				"properties": map[string]any{
					"title":    "Sheet",
					"locale":   "en",
					"timeZone": "UTC",
				},
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 1, "title": "Sheet1", "gridProperties": map[string]any{"rowCount": 10, "columnCount": 5}}},
				},
			})
			return
		case path == "/spreadsheets" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "s2",
				"spreadsheetUrl": "http://example.com/s2",
				"properties": map[string]any{
					"title": "New Sheet",
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newSheetsServiceFromServer(t, srv)
	commands := []struct {
		name string
		args []string
	}{
		{"get", []string{"--json", "--account", "a@b.com", "sheets", "get", "s1", "Sheet1!A1:B2"}},
		{"update", []string{"--json", "--account", "a@b.com", "sheets", "update", "s1", "Sheet1!A1", "--values-json", `[["a","b"]]`}},
		{"append", []string{"--json", "--account", "a@b.com", "sheets", "append", "s1", "Sheet1!A1", "--values-json", `[["a"]]`}},
		{"clear", []string{"--json", "--account", "a@b.com", "sheets", "clear", "s1", "Sheet1!A1"}},
		{"metadata", []string{"--json", "--account", "a@b.com", "sheets", "metadata", "s1"}},
		{"create", []string{"--json", "--account", "a@b.com", "sheets", "create", "New Sheet", "--sheets", "Sheet1,Sheet2"}},
	}
	for _, command := range commands {
		if result := executeWithSheetsTestService(t, command.args, svc); result.err != nil {
			t.Fatalf("%s: %v", command.name, result.err)
		}
	}
}

func TestSheetsCommands_Text(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sheets/v4")
		path = strings.TrimPrefix(path, "/v4")
		switch {
		case strings.Contains(path, "/spreadsheets/s1/values/") && strings.Contains(path, ":append") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updates": map[string]any{"updatedCells": 1},
			})
			return
		case strings.Contains(path, "/spreadsheets/s1/values/") && strings.Contains(path, ":clear") && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"clearedRange": "Sheet1!A1",
			})
			return
		case strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodPut:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"updatedCells": 2,
			})
			return
		case strings.Contains(path, "/spreadsheets/s1/values/") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"range":  "Sheet1!A1:B2",
				"values": [][]any{{"a", "b"}},
			})
			return
		case strings.HasPrefix(path, "/spreadsheets/s1") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "s1",
				"spreadsheetUrl": "http://example.com/s1",
				"properties": map[string]any{
					"title":    "Sheet",
					"locale":   "en",
					"timeZone": "UTC",
				},
				"sheets": []map[string]any{
					{"properties": map[string]any{"sheetId": 1, "title": "Sheet1", "gridProperties": map[string]any{"rowCount": 10, "columnCount": 5}}},
				},
			})
			return
		case path == "/spreadsheets" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spreadsheetId":  "s2",
				"spreadsheetUrl": "http://example.com/s2",
				"properties": map[string]any{
					"title": "New Sheet",
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer srv.Close()

	svc := newSheetsServiceFromServer(t, srv)
	commands := []struct {
		name string
		args []string
	}{
		{"get", []string{"--plain", "--account", "a@b.com", "sheets", "get", "s1", "Sheet1!A1:B2"}},
		{"update", []string{"--plain", "--account", "a@b.com", "sheets", "update", "s1", "Sheet1!A1", "--values-json", `[["a","b"]]`}},
		{"append", []string{"--plain", "--account", "a@b.com", "sheets", "append", "s1", "Sheet1!A1", "--values-json", `[["a"]]`}},
		{"clear", []string{"--plain", "--account", "a@b.com", "sheets", "clear", "s1", "Sheet1!A1"}},
		{"metadata", []string{"--plain", "--account", "a@b.com", "sheets", "metadata", "s1"}},
		{"create", []string{"--plain", "--account", "a@b.com", "sheets", "create", "New Sheet"}},
	}
	var out strings.Builder
	for _, command := range commands {
		result := executeWithSheetsTestService(t, command.args, svc)
		if result.err != nil {
			t.Fatalf("%s: %v", command.name, result.err)
		}
		out.WriteString(result.stdout)
	}
	if !strings.Contains(out.String(), "Sheet1") || !strings.Contains(out.String(), "Created spreadsheet") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}
