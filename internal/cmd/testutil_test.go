package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/steipete/gogcli/internal/googleauth"
)

// withPrimaryCalendar wraps an http.Handler to also respond to primary calendar requests
// with a default timezone. This is needed because time-aware commands now fetch the
// user's timezone from their primary calendar.
func withPrimaryCalendar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle primary calendar list request for timezone
		if strings.Contains(r.URL.Path, "/calendarList/primary") && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "primary",
				"summary":  "Test Calendar",
				"timeZone": "UTC",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = orig
	b, _ := io.ReadAll(r)
	_ = r.Close()
	return string(b)
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	fn()

	_ = w.Close()
	os.Stderr = orig
	b, _ := io.ReadAll(r)
	_ = r.Close()
	return string(b)
}

func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()

	orig := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r

	_, _ = io.WriteString(w, input)
	_ = w.Close()

	fn()

	_ = r.Close()
	os.Stdin = orig
}

func runKong(t *testing.T, cmd any, args []string, ctx context.Context, flags *RootFlags) (err error) {
	t.Helper()

	parser, err := kong.New(
		cmd,
		kong.Vars(kong.Vars{
			"auth_services": googleauth.UserServiceCSV(),
		}),
		kong.Writers(io.Discard, io.Discard),
		kong.Exit(func(code int) { panic(exitPanic{code: code}) }),
	)
	if err != nil {
		return err
	}

	defer func() {
		if r := recover(); r != nil {
			if ep, ok := r.(exitPanic); ok {
				if ep.code == 0 {
					err = nil
					return
				}
				err = &ExitError{Code: ep.code, Err: errors.New("exited")}
				return
			}
			panic(r)
		}
	}()

	kctx, err := parser.Parse(args)
	if err != nil {
		return err
	}

	if ctx != nil {
		kctx.BindTo(ctx, (*context.Context)(nil))
	}
	if flags != nil {
		kctx.Bind(flags)
	}

	return kctx.Run()
}
