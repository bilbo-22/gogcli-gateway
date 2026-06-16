package cmd

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSlidesListSlidesUsesRuntimeOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/presentations/deck") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"presentationId":"deck","title":"Deck","slides":[{"objectId":"slide-1"},{"objectId":"slide-2"}]}`)
	}))
	defer server.Close()

	var output bytes.Buffer
	ctx := withSlidesTestService(
		newCmdRuntimeOutputContext(t, &output, io.Discard),
		newMockSlidesService(t, server),
	)
	if err := (&SlidesListSlidesCmd{PresentationID: "deck"}).Run(ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "Presentation: Deck (2 slides)") || !strings.Contains(got, "1  slide-1") {
		t.Fatalf("output = %q", got)
	}
}
