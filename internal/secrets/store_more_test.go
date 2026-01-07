package secrets

import (
	"errors"
	"testing"

	"github.com/99designs/keyring"
)

func TestKeyringStore_SetToken_Validation(t *testing.T) {
	s := &KeyringStore{ring: keyring.NewArrayKeyring(nil)}

	if err := s.SetToken("", Token{RefreshToken: "rt"}); err == nil {
		t.Fatalf("expected error for missing email")
	}

	if err := s.SetToken("a@b.com", Token{}); err == nil {
		t.Fatalf("expected error for missing refresh token")
	}
}

func TestKeyringStore_GetToken_Validation(t *testing.T) {
	s := &KeyringStore{ring: keyring.NewArrayKeyring(nil)}

	if _, err := s.GetToken(""); err == nil {
		t.Fatalf("expected error for missing email")
	}
}

func TestParseTokenKey_RejectsEmpty(t *testing.T) {
	if _, ok := ParseTokenKey("token:"); ok {
		t.Fatalf("expected not ok")
	}

	if _, ok := ParseTokenKey("token:   "); ok {
		t.Fatalf("expected not ok")
	}
}

func TestFileKeyringPasswordFuncFrom(t *testing.T) {
	pf := fileKeyringPasswordFuncFrom("secret", false)
	res := func() struct {
		got string
		err error
	} {
		got, err := pf("prompt")

		return struct {
			got string
			err error
		}{got: got, err: err}
	}()

	if res.err != nil || res.got != "secret" {
		t.Fatalf("expected secret, got %q err=%v", res.got, res.err)
	}

	pf = fileKeyringPasswordFuncFrom("", true)

	if pf == nil {
		t.Fatalf("expected terminal prompt func")
	}

	pf = fileKeyringPasswordFuncFrom("", false)

	if _, err := pf("prompt"); err == nil {
		t.Fatalf("expected error without tty")
	}
}

func TestFileKeyringPasswordFunc(t *testing.T) {
	t.Setenv(keyringPasswordEnv, "secret")
	pf := fileKeyringPasswordFunc()
	res := func() struct {
		got string
		err error
	} {
		got, err := pf("prompt")

		return struct {
			got string
			err error
		}{got: got, err: err}
	}()

	if res.err != nil || res.got != "secret" {
		t.Fatalf("expected secret, got %q err=%v", res.got, res.err)
	}
}

func TestResolveKeyringBackendInfo_EnvNormalizesWhitespaceAndCase(t *testing.T) {
	t.Setenv(keyringBackendEnv, "  KEYCHAIN  ")

	info, err := ResolveKeyringBackendInfo()
	if err != nil {
		t.Fatalf("ResolveKeyringBackendInfo: %v", err)
	}

	if info.Value != "keychain" {
		t.Fatalf("expected keychain, got %q", info.Value)
	}

	if info.Source != keyringBackendSourceEnv {
		t.Fatalf("expected source env, got %q", info.Source)
	}
}

func TestAllowedBackends_NormalizesAndValidates(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantLen int
		wantErr bool
	}{
		{"empty defaults to nil", "", 0, false},
		{"auto defaults to nil", "auto", 0, false},
		{"keychain returns one backend", "keychain", 1, false},
		{"file returns one backend", "file", 1, false},
		{"invalid returns error", "invalid", 0, true},
		{"whitespace trimmed", "  keychain  ", 1, false},
		{"case insensitive", "KEYCHAIN", 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backends, err := allowedBackends(KeyringBackendInfo{Value: tt.value})

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}

				if !errors.Is(err, errInvalidKeyringBackend) {
					t.Errorf("expected errInvalidKeyringBackend, got %v", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(backends) != tt.wantLen {
				t.Errorf("expected %d backends, got %d", tt.wantLen, len(backends))
			}
		})
	}
}
