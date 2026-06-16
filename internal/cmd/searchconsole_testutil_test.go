package cmd

import (
	"context"
	"errors"
	"net/http"
	"testing"

	searchconsoleapi "google.golang.org/api/searchconsole/v1"

	"github.com/steipete/gogcli/internal/app"
)

func newSearchConsoleTestService(t *testing.T, handler http.Handler) *searchconsoleapi.Service {
	t.Helper()
	svc, closeServer := newGoogleTestService(t, handler, searchconsoleapi.NewService)
	t.Cleanup(closeServer)
	return svc
}

func fixedSearchConsoleTestService(svc *searchconsoleapi.Service) app.SearchConsoleServiceFactory {
	return func(context.Context, string) (*searchconsoleapi.Service, error) {
		return svc, nil
	}
}

func unexpectedSearchConsoleTestService(t *testing.T, message string) app.SearchConsoleServiceFactory {
	t.Helper()
	return func(context.Context, string) (*searchconsoleapi.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected search console service call")
	}
}

func executeWithSearchConsoleTestService(t *testing.T, args []string, svc *searchconsoleapi.Service) executeTestResult {
	t.Helper()
	return executeWithSearchConsoleTestServiceFactory(t, args, fixedSearchConsoleTestService(svc))
}

func executeWithSearchConsoleTestServiceFactory(
	t *testing.T,
	args []string,
	factory app.SearchConsoleServiceFactory,
) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		SearchConsole: factory,
	}})
}
