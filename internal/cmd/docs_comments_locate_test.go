package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func TestDocsCommentsLocateJSONTabEntitiesAndWhitespace(t *testing.T) {
	driveSvc, driveCleanup := newDriveCommentsTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.TrimPrefix(r.URL.Path, "/drive/v3") != "/files/doc1/comments/c1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "c1",
			"content": "please check",
			"quotedFileContent": map[string]any{
				"value": "Tom &amp; Jerry next",
			},
		})
	}))
	defer driveCleanup()

	var includeTabs string
	docSvc, docsCleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/documents/") {
			http.NotFound(w, r)
			return
		}
		includeTabs = r.URL.Query().Get("includeTabsContent")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&docs.Document{
			DocumentId: "doc1",
			Tabs: []*docs.Tab{
				{
					TabProperties: &docs.TabProperties{TabId: "t.first", Title: "First"},
					DocumentTab:   &docs.DocumentTab{Body: docsFindRangeDoc(docsFindRangeParagraph(1, "not here\n")).Body},
				},
				{
					TabProperties: &docs.TabProperties{TabId: "t.second", Title: "Second"},
					DocumentTab: &docs.DocumentTab{Body: docsFindRangeDoc(
						docsFindRangeParagraph(1, "Tom\t  & Jerry\nnext\n"),
					).Body},
				},
			},
		})
	}))
	defer docsCleanup()

	execResult := runDocsCommentsLocateJSON(t, driveSvc, docSvc, "doc1", "c1", "--tab", "Second")
	if execResult.err != nil {
		t.Fatalf("locate: %v", execResult.err)
	}
	if includeTabs != "true" {
		t.Fatalf("includeTabsContent = %q, want true", includeTabs)
	}

	var result docsCommentLocateResult
	if err := json.Unmarshal([]byte(execResult.stdout), &result); err != nil {
		t.Fatalf("json: %v\n%s", err, execResult.stdout)
	}
	if result.CommentID != "c1" || result.Orphaned || result.Quote != "Tom &amp; Jerry next" {
		t.Fatalf("result metadata = %#v", result)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches = %#v, want one", result.Matches)
	}
	if got := result.Matches[0]; got.StartIndex != 1 || got.EndIndex != 19 || got.TabID != "t.second" {
		t.Fatalf("match = %#v, want range 1..19 tab t.second", got)
	}
}

func TestDocsCommentsLocatePreservesLiteralEntities(t *testing.T) {
	driveSvc, driveCleanup := newDriveCommentsTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.TrimPrefix(r.URL.Path, "/drive/v3") != "/files/doc1/comments/c1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "c1",
			"quotedFileContent": map[string]any{
				"value": "literal &amp; marker",
			},
		})
	}))
	defer driveCleanup()

	docSvc, docsCleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/documents/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(docsFindRangeDoc(docsFindRangeParagraph(1, "literal &amp; marker\n")))
	}))
	defer docsCleanup()

	execResult := runDocsCommentsLocateJSON(t, driveSvc, docSvc, "doc1", "c1")
	if execResult.err != nil {
		t.Fatalf("locate: %v", execResult.err)
	}
	var result docsCommentLocateResult
	if err := json.Unmarshal([]byte(execResult.stdout), &result); err != nil {
		t.Fatalf("json: %v\n%s", err, execResult.stdout)
	}
	if result.Orphaned || result.Quote != "literal &amp; marker" || len(result.Matches) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if got := result.Matches[0]; got.StartIndex != 1 || got.EndIndex != 21 {
		t.Fatalf("match = %#v, want raw entity range 1..21", got)
	}
}

func TestDocsCommentsLocateFallbackDecodesEntitiesOnce(t *testing.T) {
	driveSvc, driveCleanup := newDriveCommentsTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.TrimPrefix(r.URL.Path, "/drive/v3") != "/files/doc1/comments/c1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "c1",
			"quotedFileContent": map[string]any{
				"value": "literal &amp;amp; marker",
			},
		})
	}))
	defer driveCleanup()

	docSvc, docsCleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/documents/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(docsFindRangeDoc(docsFindRangeParagraph(1, "literal &amp; marker\n")))
	}))
	defer docsCleanup()

	execResult := runDocsCommentsLocateJSON(t, driveSvc, docSvc, "doc1", "c1")
	if execResult.err != nil {
		t.Fatalf("locate: %v", execResult.err)
	}
	var result docsCommentLocateResult
	if err := json.Unmarshal([]byte(execResult.stdout), &result); err != nil {
		t.Fatalf("json: %v\n%s", err, execResult.stdout)
	}
	if result.Orphaned || result.Quote != "literal &amp;amp; marker" || len(result.Matches) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if got := result.Matches[0]; got.StartIndex != 1 || got.EndIndex != 21 {
		t.Fatalf("match = %#v, want raw entity range 1..21", got)
	}
}

func TestDocsCommentsLocateDefaultSearchesAllTabs(t *testing.T) {
	driveSvc, driveCleanup := newDriveCommentsTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.TrimPrefix(r.URL.Path, "/drive/v3") != "/files/doc1/comments/c1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "c1",
			"quotedFileContent": map[string]any{
				"value": "second tab quote",
			},
		})
	}))
	defer driveCleanup()

	var includeTabs string
	docSvc, docsCleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/documents/") {
			http.NotFound(w, r)
			return
		}
		includeTabs = r.URL.Query().Get("includeTabsContent")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&docs.Document{
			DocumentId: "doc1",
			Tabs: []*docs.Tab{
				{
					TabProperties: &docs.TabProperties{TabId: "t.first", Title: "First"},
					DocumentTab:   &docs.DocumentTab{Body: docsFindRangeDoc(docsFindRangeParagraph(1, "first tab only\n")).Body},
				},
				{
					TabProperties: &docs.TabProperties{TabId: "t.second", Title: "Second"},
					DocumentTab:   &docs.DocumentTab{Body: docsFindRangeDoc(docsFindRangeParagraph(1, "second tab quote\n")).Body},
				},
			},
		})
	}))
	defer docsCleanup()

	execResult := runDocsCommentsLocateJSON(t, driveSvc, docSvc, "doc1", "c1")
	if execResult.err != nil {
		t.Fatalf("locate: %v", execResult.err)
	}
	if includeTabs != "true" {
		t.Fatalf("includeTabsContent = %q, want true", includeTabs)
	}

	var result docsCommentLocateResult
	if err := json.Unmarshal([]byte(execResult.stdout), &result); err != nil {
		t.Fatalf("json: %v\n%s", err, execResult.stdout)
	}
	if result.Orphaned || len(result.Matches) != 1 || result.Matches[0].TabID != "t.second" {
		t.Fatalf("result = %#v, want one match in second tab", result)
	}
}

func TestDocsCommentsLocatePlainOrphanedExit(t *testing.T) {
	driveSvc, driveCleanup := newDriveCommentsTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.TrimPrefix(r.URL.Path, "/drive/v3") != "/files/doc1/comments/c1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "c1",
			"quotedFileContent": map[string]any{
				"value": "missing quote",
			},
		})
	}))
	defer driveCleanup()

	docSvc, docsCleanup := newDocsServiceForTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/documents/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(docsFindRangeDoc(docsFindRangeParagraph(1, "present text\n")))
	}))
	defer docsCleanup()

	var out bytes.Buffer
	ctx := withDocsTestService(
		withDriveTestService(newCmdRuntimeOutputContext(t, &out, io.Discard), driveSvc),
		docSvc,
	)
	err := runKong(t, &DocsCommentsLocateCmd{}, []string{"doc1", "c1"}, ctx, &RootFlags{Account: "a@b.com"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != exitCodeOrphaned {
		t.Fatalf("err = %#v, want orphaned exit code %d", err, exitCodeOrphaned)
	}
	if got := out.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestDocsCommentsLocateJSONNoQuote(t *testing.T) {
	driveSvc, driveCleanup := newDriveCommentsTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || strings.TrimPrefix(r.URL.Path, "/drive/v3") != "/files/doc1/comments/c1" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "c1", "content": "unanchored"})
	}))
	defer driveCleanup()

	execResult := runDocsCommentsLocateJSON(t, driveSvc, nil, "doc1", "c1")
	if execResult.err != nil {
		t.Fatalf("locate no quote: %v", execResult.err)
	}
	var result docsCommentLocateResult
	if err := json.Unmarshal([]byte(execResult.stdout), &result); err != nil {
		t.Fatalf("json: %v\n%s", err, execResult.stdout)
	}
	if result.CommentID != "c1" || !result.Orphaned || result.Quote != "" || len(result.Matches) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func runDocsCommentsLocateJSON(t *testing.T, driveSvc *drive.Service, docsSvc *docs.Service, args ...string) executeTestResult {
	t.Helper()

	var stdout, stderr bytes.Buffer
	ctx := withDriveTestService(newCmdRuntimeJSONOutputContext(t, &stdout, &stderr), driveSvc)
	if docsSvc == nil {
		ctx = withDocsTestServiceFactory(ctx, func(context.Context, string) (*docs.Service, error) {
			t.Fatal("unanchored comment must not create Docs service")
			return nil, errors.New("unexpected Docs service creation")
		})
	} else {
		ctx = withDocsTestService(ctx, docsSvc)
	}
	err := runKong(t, &DocsCommentsLocateCmd{}, args, ctx, &RootFlags{Account: "a@b.com"})
	return executeTestResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    err,
	}
}

func newDriveCommentsTestService(t *testing.T, h http.HandlerFunc) (*drive.Service, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	svc, err := drive.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDriveService: %v", err)
	}
	return svc, func() {}
}
