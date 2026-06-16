package cmd

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"google.golang.org/api/chat/v1"

	"github.com/steipete/gogcli/internal/app"
)

func newChatTestService(t *testing.T, handler http.Handler) *chat.Service {
	t.Helper()
	svc, closeServer := newGoogleTestService(t, handler, chat.NewService)
	t.Cleanup(closeServer)
	return svc
}

func fixedChatTestService(svc *chat.Service) app.ChatServiceFactory {
	return func(context.Context, string) (*chat.Service, error) {
		return svc, nil
	}
}

func unexpectedChatTestService(t *testing.T, message string) app.ChatServiceFactory {
	t.Helper()
	return func(context.Context, string) (*chat.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected chat service call")
	}
}

func executeWithChatTestService(t *testing.T, args []string, svc *chat.Service) executeTestResult {
	t.Helper()
	return executeWithChatTestServiceFactory(t, args, fixedChatTestService(svc))
}

func executeWithChatTestServiceFactory(t *testing.T, args []string, factory app.ChatServiceFactory) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Chat: factory,
	}})
}
