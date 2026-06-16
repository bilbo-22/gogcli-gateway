package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/slides/v1"
)

func readSlidePresResponse() map[string]any {
	return map[string]any{
		"presentationId": "pres1",
		"slides": []any{
			map[string]any{
				"objectId": "slide_1",
				"slideProperties": map[string]any{
					"notesPage": map[string]any{
						"notesProperties": map[string]any{
							"speakerNotesObjectId": "notes_1",
						},
						"pageElements": []any{
							map[string]any{
								"objectId": "notes_1",
								"shape": map[string]any{
									"placeholder": map[string]any{"type": "BODY"},
									"text": map[string]any{
										"textElements": []any{
											map[string]any{
												"textRun": map[string]any{
													"content": "These are speaker notes",
												},
											},
										},
									},
								},
							},
						},
					},
				},
				"pageElements": []any{
					map[string]any{
						"objectId": "text_el_1",
						"shape": map[string]any{
							"text": map[string]any{
								"textElements": []any{
									map[string]any{
										"textRun": map[string]any{
											"content": "Slide Title",
										},
									},
								},
							},
						},
					},
					map[string]any{
						"objectId": "img_el_1",
						"image": map[string]any{
							"contentUrl": "https://example.com/image.png",
						},
					},
				},
			},
		},
	}
}

func newSlidesReadTestService(t *testing.T, response map[string]any) *slides.Service {
	t.Helper()

	svc, closeServer := newGoogleTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/presentations/pres1") && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		http.NotFound(w, r)
	}), slides.NewService)
	t.Cleanup(closeServer)
	return svc
}

func TestSlidesReadSlide(t *testing.T) {
	svc := newSlidesReadTestService(t, readSlidePresResponse())
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesReadSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(out.String(), "Slide 1") {
		t.Errorf("expected slide number, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "These are speaker notes") {
		t.Errorf("expected speaker notes, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "Slide Title") {
		t.Errorf("expected text element, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "img_el_1") {
		t.Errorf("expected image element, got: %q", out.String())
	}
}

func TestSlidesReadSlide_JSON(t *testing.T) {
	svc := newSlidesReadTestService(t, readSlidePresResponse())
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesReadSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %q", err, out.String())
	}
	if result["slideNumber"] != float64(1) {
		t.Errorf("expected slideNumber=1, got %v", result["slideNumber"])
	}
	if result["notes"] != "These are speaker notes" {
		t.Errorf("expected notes text, got %v", result["notes"])
	}
	if result["slideObjectId"] != "slide_1" {
		t.Errorf("expected slideObjectId=slide_1, got %v", result["slideObjectId"])
	}

	textEls, ok := result["textElements"].([]any)
	if !ok || len(textEls) != 1 {
		t.Errorf("expected 1 text element, got %v", result["textElements"])
	}

	imgs, ok := result["images"].([]any)
	if !ok || len(imgs) != 1 {
		t.Errorf("expected 1 image, got %v", result["images"])
	}
}

func TestSlidesReadSlide_JSONEmptyArrays(t *testing.T) {
	presResp := map[string]any{
		"presentationId": "pres1",
		"slides": []any{
			map[string]any{
				"objectId":     "slide_1",
				"pageElements": []any{},
			},
		},
	}

	svc := newSlidesReadTestService(t, presResp)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
	cmd := &SlidesReadSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var result struct {
		TextElements []json.RawMessage `json:"textElements"`
		Images       []json.RawMessage `json:"images"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %q", err, out.String())
	}
	if result.TextElements == nil {
		t.Fatalf("textElements must be an empty array, got nil: %s", out.String())
	}
	if len(result.TextElements) != 0 {
		t.Fatalf("textElements len = %d, want 0", len(result.TextElements))
	}
	if result.Images == nil {
		t.Fatalf("images must be an empty array, got nil: %s", out.String())
	}
	if len(result.Images) != 0 {
		t.Fatalf("images len = %d, want 0", len(result.Images))
	}
}

func TestSlidesReadSlide_NotFound(t *testing.T) {
	svc := newSlidesReadTestService(t, readSlidePresResponse())
	flags := &RootFlags{Account: "a@b.com"}
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, io.Discard, io.Discard), svc)

	cmd := &SlidesReadSlideCmd{
		PresentationID: "pres1",
		SlideID:        "nonexistent",
	}
	err := cmd.Run(ctx, flags)
	if err == nil || !strings.Contains(err.Error(), `slide "nonexistent" not found`) {
		t.Fatalf("expected slide-not-found error, got: %v", err)
	}
}

func TestSlidesReadSlide_NoNotes(t *testing.T) {
	presResp := map[string]any{
		"presentationId": "pres1",
		"slides": []any{
			map[string]any{
				"objectId": "slide_1",
				"slideProperties": map[string]any{
					"notesPage": map[string]any{},
				},
				"pageElements": []any{},
			},
		},
	}

	svc := newSlidesReadTestService(t, presResp)
	flags := &RootFlags{Account: "a@b.com"}
	var out bytes.Buffer
	ctx := withSlidesTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), svc)

	cmd := &SlidesReadSlideCmd{
		PresentationID: "pres1",
		SlideID:        "slide_1",
	}
	if err := cmd.Run(ctx, flags); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(out.String(), "Speaker Notes: (none)") {
		t.Errorf("expected '(none)' for empty notes, got: %q", out.String())
	}
}
