package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestClassroomCourseworkList_TopicScanPages(t *testing.T) {
	var calls int
	svc, closeService := newClassroomTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/courseWork") {
			http.NotFound(w, r)
			return
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"courseWork":    []map[string]any{{"id": "w1", "topicId": "other"}},
				"nextPageToken": "p2",
			})
		case 2:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"courseWork":    []map[string]any{{"id": "w2", "topicId": "target"}},
				"nextPageToken": "",
			})
		default:
			t.Fatalf("unexpected coursework calls: %d", calls)
		}
	}))
	defer closeService()

	result := executeWithClassroomTestService(t, []string{"--json", "--account", "a@b.com", "classroom", "coursework", "c1", "--topic", "target", "--scan-pages", "2"}, svc)
	if result.err != nil {
		t.Fatalf("execute: %v", result.err)
	}
	var payload struct {
		Coursework []struct {
			ID string `json:"id"`
		} `json:"coursework"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Coursework) != 1 || payload.Coursework[0].ID != "w2" {
		t.Fatalf("expected coursework w2, got %#v", payload.Coursework)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestClassroomMaterialsList_TopicScanPages(t *testing.T) {
	var calls int
	svc, closeService := newClassroomTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/courseWorkMaterials") {
			http.NotFound(w, r)
			return
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"courseWorkMaterial": []map[string]any{{"id": "m1", "topicId": "other"}},
				"nextPageToken":      "p2",
			})
		case 2:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"courseWorkMaterial": []map[string]any{{"id": "m2", "topicId": "target"}},
				"nextPageToken":      "",
			})
		default:
			t.Fatalf("unexpected materials calls: %d", calls)
		}
	}))
	defer closeService()

	result := executeWithClassroomTestService(t, []string{"--json", "--account", "a@b.com", "classroom", "materials", "c1", "--topic", "target", "--scan-pages", "2"}, svc)
	if result.err != nil {
		t.Fatalf("execute: %v", result.err)
	}
	var payload struct {
		Materials []struct {
			ID string `json:"id"`
		} `json:"materials"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Materials) != 1 || payload.Materials[0].ID != "m2" {
		t.Fatalf("expected material m2, got %#v", payload.Materials)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}
