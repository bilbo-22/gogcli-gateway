package cmd

import (
	"context"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/app"
)

func newSheetsServiceFromServer(t *testing.T, srv *httptest.Server) *sheets.Service {
	t.Helper()
	return newGoogleTestServiceWithEndpoint(t, srv.Client(), srv.URL+"/", sheets.NewService)
}

func withSheetsTestService(ctx context.Context, svc *sheets.Service) context.Context {
	return withSheetsTestServiceFactory(ctx, func(context.Context, string) (*sheets.Service, error) {
		return svc, nil
	})
}

func withSheetsTestServiceFactory(ctx context.Context, factory app.SheetsServiceFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	runtime.Services.Sheets = factory
	return app.WithRuntime(ctx, runtime)
}

func executeWithSheetsTestService(t *testing.T, args []string, svc *sheets.Service) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Sheets: func(context.Context, string) (*sheets.Service, error) { return svc, nil },
	}})
}

func executeWithSheetsAndDriveTestServices(t *testing.T, args []string, sheetsSvc *sheets.Service, driveSvc *drive.Service) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Drive:  stubDriveService(driveSvc),
		Sheets: func(context.Context, string) (*sheets.Service, error) { return sheetsSvc, nil },
	}})
}
