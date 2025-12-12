package googleapi

import (
	"context"
	"time"

	"github.com/99designs/keyring"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/googleauth"
	"github.com/steipete/gogcli/internal/secrets"
)

const defaultHTTPTimeout = 30 * time.Second

func tokenSourceForAccount(ctx context.Context, service googleauth.Service, email string) (oauth2.TokenSource, error) {
	creds, err := config.ReadClientCredentials()
	if err != nil {
		return nil, err
	}

	requiredScopes, err := googleauth.Scopes(service)
	if err != nil {
		return nil, err
	}

	return tokenSourceForAccountScopes(ctx, string(service), email, creds.ClientID, creds.ClientSecret, requiredScopes)
}

func tokenSourceForAccountScopes(ctx context.Context, serviceLabel string, email string, clientID string, clientSecret string, requiredScopes []string) (oauth2.TokenSource, error) {
	store, err := secrets.OpenDefault()
	if err != nil {
		return nil, err
	}
	tok, err := store.GetToken(email)
	if err != nil {
		if err == keyring.ErrKeyNotFound {
			return nil, &AuthRequiredError{Service: serviceLabel, Email: email, Cause: err}
		}
		return nil, err
	}

	if len(tok.Scopes) > 0 {
		have := make(map[string]struct{}, len(tok.Scopes))
		for _, s := range tok.Scopes {
			have[s] = struct{}{}
		}
		missing := make([]string, 0)
		for _, want := range requiredScopes {
			if _, ok := have[want]; !ok {
				missing = append(missing, want)
			}
		}
		if len(missing) > 0 {
			return nil, &MissingScopesError{Service: serviceLabel, Email: email, Missing: missing}
		}
	}

	cfg := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       requiredScopes,
	}

	return cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: tok.RefreshToken}), nil
}

func optionsForAccount(ctx context.Context, service googleauth.Service, email string) ([]option.ClientOption, error) {
	ts, err := tokenSourceForAccount(ctx, service, email)
	if err != nil {
		return nil, err
	}
	c := oauth2.NewClient(ctx, ts)
	c.Timeout = defaultHTTPTimeout
	return []option.ClientOption{option.WithHTTPClient(c)}, nil
}

func optionsForAccountScopes(ctx context.Context, serviceLabel string, email string, scopes []string) ([]option.ClientOption, error) {
	creds, err := config.ReadClientCredentials()
	if err != nil {
		return nil, err
	}
	ts, err := tokenSourceForAccountScopes(ctx, serviceLabel, email, creds.ClientID, creds.ClientSecret, scopes)
	if err != nil {
		return nil, err
	}
	c := oauth2.NewClient(ctx, ts)
	c.Timeout = defaultHTTPTimeout
	return []option.ClientOption{option.WithHTTPClient(c)}, nil
}
