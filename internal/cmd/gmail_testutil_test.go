package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/app"
)

func newGmailServiceForTest(t *testing.T, h http.HandlerFunc) (*gmail.Service, func()) {
	t.Helper()

	return newGoogleTestService(t, h, gmail.NewService)
}

func newGmailServiceFromServer(t *testing.T, srv *httptest.Server) *gmail.Service {
	t.Helper()
	return newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", gmail.NewService)
}

func withGmailTestService(ctx context.Context, svc *gmail.Service) context.Context {
	return withGmailTestServiceFactory(ctx, func(context.Context, string) (*gmail.Service, error) {
		return svc, nil
	})
}

func withGmailTestServiceFactory(ctx context.Context, factory app.GmailServiceFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	runtime.Services.Gmail = factory
	return app.WithRuntime(ctx, runtime)
}

func executeWithGmailTestService(t *testing.T, args []string, svc *gmail.Service) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Gmail: func(context.Context, string) (*gmail.Service, error) { return svc, nil },
	}})
}
