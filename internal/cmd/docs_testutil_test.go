package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/ui"
)

func newDocsServiceForTest(t *testing.T, h http.HandlerFunc) (*docs.Service, func()) {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	docSvc, err := docs.NewService(context.Background(),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
		option.WithEndpoint(srv.URL+"/"),
	)
	if err != nil {
		t.Fatalf("NewDocsService: %v", err)
	}
	return docSvc, func() {} // retained for call-site compat; cleanup is via t.Cleanup
}

func withDocsTestService(ctx context.Context, svc *docs.Service) context.Context {
	return withDocsTestServiceFactory(ctx, func(context.Context, string) (*docs.Service, error) {
		return svc, nil
	})
}

func withDocsTestServiceFactory(ctx context.Context, factory app.DocsServiceFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	runtime.Services.Docs = factory
	return app.WithRuntime(ctx, runtime)
}

func withDocsTestHTTPClientFactory(ctx context.Context, factory app.DocsHTTPClientFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	runtime.Services.DocsHTTP = factory
	return app.WithRuntime(ctx, runtime)
}

func newDocsCmdContext(t *testing.T) context.Context {
	t.Helper()
	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u)
}

func newDocsCmdOutputContext(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	u, err := ui.New(ui.Options{Stdout: &out, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	return ui.WithUI(context.Background(), u), &out
}

func newDocsJSONContextWithDrive(t *testing.T, svc *drive.Service) context.Context {
	t.Helper()
	return withDriveTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
}

func newDocsJSONContextWithoutDrive(t *testing.T, message string) context.Context {
	t.Helper()
	return withDriveTestServiceFactory(
		newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard),
		func(context.Context, string) (*drive.Service, error) {
			t.Fatal(message)
			return nil, errors.New("unexpected Drive service call")
		},
	)
}

func docBodyWithText(text string) map[string]any {
	return map[string]any{
		"documentId": "doc1",
		"body": map[string]any{
			"content": []any{
				map[string]any{
					"startIndex":   0,
					"endIndex":     1,
					"sectionBreak": map[string]any{"sectionStyle": map[string]any{}},
				},
				map[string]any{
					"startIndex": 1,
					"endIndex":   1 + len(text),
					"paragraph": map[string]any{
						"elements": []any{
							map[string]any{
								"startIndex": 1,
								"endIndex":   1 + len(text),
								"textRun":    map[string]any{"content": text},
							},
						},
					},
				},
			},
		},
	}
}
