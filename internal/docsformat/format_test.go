package docsformat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildRequests(t *testing.T) {
	requests, err := BuildRequests(Options{
		FontFamily:   "Georgia",
		FontSize:     14,
		TextColor:    "#3366cc",
		Background:   "#fff",
		ClearBold:    true,
		Italic:       true,
		Alignment:    "center",
		LineSpacing:  150,
		HeadingLevel: intPointer(2),
	}, 3, 9, "t.second")
	if err != nil {
		t.Fatalf("BuildRequests: %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}

	text := requests[0].UpdateTextStyle
	if text == nil || text.Fields != "weightedFontFamily,fontSize,foregroundColor,backgroundColor,bold,italic" {
		t.Fatalf("unexpected text request: %#v", requests[0])
	}

	encoded, err := json.Marshal(text.TextStyle)
	if err != nil {
		t.Fatalf("marshal text style: %v", err)
	}

	if !strings.Contains(string(encoded), `"bold":false`) {
		t.Fatalf("clearing bold must force-send false: %s", encoded)
	}

	paragraph := requests[1].UpdateParagraphStyle
	if paragraph == nil || paragraph.ParagraphStyle.Alignment != "CENTER" ||
		paragraph.ParagraphStyle.LineSpacing != 150 ||
		paragraph.ParagraphStyle.NamedStyleType != "HEADING_2" {
		t.Fatalf("unexpected paragraph request: %#v", requests[1])
	}

	if paragraph.Range.TabId != "t.second" {
		t.Fatalf("tab id = %q, want t.second", paragraph.Range.TabId)
	}
}

func TestBuildRequestsValidation(t *testing.T) {
	tests := []Options{
		{TextColor: "oops"},
		{Link: "https://example.com", ClearLink: true},
		{Bold: true, ClearBold: true},
		{Alignment: "sideways"},
		{Code: true, FontFamily: "Arial"},
		{Code: true, Background: "#fff"},
		{HeadingLevel: intPointer(1), NamedStyle: "TITLE"},
	}
	for _, options := range tests {
		if _, err := BuildRequests(options, 1, 2, ""); err == nil {
			t.Fatalf("BuildRequests(%#v) expected error", options)
		}
	}
}

func TestOptionsAny(t *testing.T) {
	if (Options{}).Any() {
		t.Fatal("zero options should be empty")
	}

	if !(Options{NamedStyle: "TITLE"}).Any() {
		t.Fatal("named style should count as formatting")
	}
}

func intPointer(value int) *int {
	return &value
}
