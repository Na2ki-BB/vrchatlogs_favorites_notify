package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTOTPCodeRFC6238Vector(t *testing.T) {
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	got, err := totpCode(secret, time.Unix(59, 0), 8)
	if err != nil {
		t.Fatal(err)
	}
	if got != "94287082" {
		t.Fatalf("totpCode() = %q, want %q", got, "94287082")
	}
}

func TestParseEnvironmentAssignment(t *testing.T) {
	tests := []struct {
		line      string
		wantKey   string
		wantValue string
		wantOK    bool
	}{
		{"# comment", "", "", false},
		{"VRC_USERNAME=user", "VRC_USERNAME", "user", true},
		{`VRC_PASSWORD="pass word"`, "VRC_PASSWORD", "pass word", true},
		{"VRC_TOTP_SECRET='ABCDEF'", "VRC_TOTP_SECRET", "ABCDEF", true},
	}
	for _, tt := range tests {
		key, value, ok, err := parseEnvironmentAssignment(tt.line)
		if err != nil {
			t.Errorf("parseEnvironmentAssignment(%q): %v", tt.line, err)
			continue
		}
		if key != tt.wantKey || value != tt.wantValue || ok != tt.wantOK {
			t.Errorf("parseEnvironmentAssignment(%q) = (%q, %q, %v)", tt.line, key, value, ok)
		}
	}
}

func TestUpdateEnvironmentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vrchat-logger.env")
	original := "VRC_USERNAME=user\nVRC_AUTH_TOKEN=old-auth\n# keep this\nVRC_NOTIFY_FAVORITE_TAG=group_0\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	err := updateEnvironmentFile(path, cookiePair{Auth: "new-auth", TwoFactor: "new-two-factor"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "VRC_USERNAME=user\nVRC_AUTH_TOKEN=new-auth\n# keep this\nVRC_NOTIFY_FAVORITE_TAG=group_0\nVRC_TWO_FACTOR_AUTH_TOKEN=new-two-factor\n"
	if string(data) != want {
		t.Fatalf("updated environment file:\n%s\nwant:\n%s", data, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %04o, want 0600", info.Mode().Perm())
	}
}

func TestAuthenticateReturnsBothCookies(t *testing.T) {
	const (
		username   = "name@example.com"
		password   = "pass word"
		totpSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
		authValue  = "authcookie_test"
		twoFAValue = "twofactor_test"
	)
	fixedNow := time.Unix(59, 0)
	wantCode, err := totpCode(totpSecret, fixedNow, 6)
	if err != nil {
		t.Fatal(err)
	}

	loginCount := 0
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		response := func(body any, cookies ...*http.Cookie) *http.Response {
			encoded, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("encode fake response: %v", err)
			}
			header := make(http.Header)
			for _, cookie := range cookies {
				header.Add("Set-Cookie", cookie.String())
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(string(encoded))),
				Request:    r,
			}
		}

		switch r.URL.Path {
		case "/auth/user":
			loginCount++
			if r.UserAgent() != "test-app/1.0 test@example.com" {
				t.Errorf("User-Agent = %q", r.UserAgent())
			}
			if loginCount == 1 {
				wantBasic := encodeCredential(username) + ":" + encodeCredential(password)
				wantAuthorization := "Basic " + base64.StdEncoding.EncodeToString([]byte(wantBasic))
				if got := r.Header.Get("Authorization"); got != wantAuthorization {
					t.Errorf("Authorization = %q, want %q", got, wantAuthorization)
				}
				return response(
					map[string]any{"requiresTwoFactorAuth": []string{"totp"}},
					&http.Cookie{Name: "auth", Value: authValue, Path: "/"},
				), nil
			}

			if r.Header.Get("Authorization") != "" {
				t.Error("verification request unexpectedly included Basic authorization")
			}
			assertCookie(t, r, "auth", authValue)
			assertCookie(t, r, "twoFactorAuth", twoFAValue)
			return response(map[string]string{"id": "usr_test"}), nil

		case "/auth/twofactorauth/totp/verify":
			assertCookie(t, r, "auth", authValue)
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode TOTP request: %v", err)
			}
			if payload["code"] != wantCode {
				t.Errorf("TOTP code = %q, want %q", payload["code"], wantCode)
			}
			return response(
				map[string]bool{"verified": true},
				&http.Cookie{Name: "twoFactorAuth", Value: twoFAValue, Path: "/"},
			), nil

		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"not found"}}`)),
				Request:    r,
			}, nil
		}
	})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	auth := authenticator{
		client:  &http.Client{Jar: jar, Transport: transport, Timeout: time.Second},
		baseURL: "https://api.test.local",
		now:     func() time.Time { return fixedNow },
	}

	got, err := auth.authenticate(context.Background(), credentials{
		Username:   username,
		Password:   password,
		TOTPSecret: totpSecret,
		UserAgent:  "test-app/1.0 test@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Auth != authValue || got.TwoFactor != twoFAValue {
		t.Fatalf("authenticate() = %#v", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func assertCookie(t *testing.T, r *http.Request, name, want string) {
	t.Helper()
	cookie, err := r.Cookie(name)
	if err != nil {
		t.Errorf("cookie %q missing: %v", name, err)
		return
	}
	if cookie.Value != want {
		t.Errorf("cookie %q = %q, want %q", name, cookie.Value, want)
	}
}
