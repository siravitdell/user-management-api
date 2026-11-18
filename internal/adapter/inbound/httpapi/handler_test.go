package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/siravitdell/user-management-api/internal/adapter/outbound/security"
	"github.com/siravitdell/user-management-api/internal/core/domain"
	"github.com/siravitdell/user-management-api/internal/core/port"
	"github.com/siravitdell/user-management-api/internal/core/service"
)

type testAPI struct {
	t       *testing.T
	handler http.Handler
	logs    *bytes.Buffer
}

func newTestAPI(t *testing.T) *testAPI {
	t.Helper()
	repo := &fakeRepo{}
	hasher, _ := security.NewBcryptHasher(bcrypt.MinCost)
	tokens, err := security.NewJWTManager("test-secret-test-secret-test-secret!", "user-api", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	users := service.NewUserService(repo, hasher)
	auth, err := service.NewAuthService(repo, hasher, tokens)
	if err != nil {
		t.Fatal(err)
	}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	return &testAPI{t: t, handler: NewRouter(users, auth, logger), logs: logs}
}

func (a *testAPI) do(method, path, token string, body any) *httptest.ResponseRecorder {
	a.t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		r = strings.NewReader(b)
	default:
		buf, _ := json.Marshal(b)
		r = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func expectStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, want, rec.Body.String())
	}
}

func (a *testAPI) registerAndLogin(email string) (userResponse, string) {
	a.t.Helper()
	rec := a.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"name": "Test User", "email": email, "password": "password123",
	})
	expectStatus(a.t, rec, http.StatusCreated)
	user := decode[userResponse](a.t, rec)

	rec = a.do(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": email, "password": "password123"})
	expectStatus(a.t, rec, http.StatusOK)
	return user, decode[tokenResponse](a.t, rec).AccessToken
}

func TestRegister(t *testing.T) {
	api := newTestAPI(t)

	rec := api.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"name": "Alice", "email": "Alice@Example.com", "password": "password123",
	})
	expectStatus(t, rec, http.StatusCreated)
	u := decode[userResponse](t, rec)
	if u.ID == "" || u.Email != "alice@example.com" || u.CreatedAt.IsZero() {
		t.Fatalf("unexpected user %+v", u)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatal("password (hash) must never be returned")
	}

	t.Run("duplicate email is 409", func(t *testing.T) {
		rec := api.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{
			"name": "Alice 2", "email": "alice@EXAMPLE.com", "password": "password123",
		})
		expectStatus(t, rec, http.StatusConflict)
	})

	t.Run("validation errors are 400 with details", func(t *testing.T) {
		rec := api.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{"email": "bad"})
		expectStatus(t, rec, http.StatusBadRequest)
		body := decode[errorResponse](t, rec)
		if body.Error.Code != "validation_failed" || len(body.Error.Details) != 3 {
			t.Fatalf("unexpected error body %+v", body)
		}
	})

	t.Run("malformed and unknown-field JSON are 400", func(t *testing.T) {
		for _, b := range []string{`{"name":`, `{"name":"a","email":"a@b.co","password":"password1","admin":true}`, ``, `{"name":1}`, `{} {}`} {
			rec := api.do(http.MethodPost, "/api/v1/auth/register", "", b)
			expectStatus(t, rec, http.StatusBadRequest)
			if decode[errorResponse](t, rec).Error.Code != "invalid_json" {
				t.Fatalf("body %q: expected invalid_json, got %s", b, rec.Body.String())
			}
		}
	})
}

func TestLogin(t *testing.T) {
	api := newTestAPI(t)
	api.registerAndLogin("bob@example.com")

	rec := api.do(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": "bob@example.com", "password": "password123"})
	expectStatus(t, rec, http.StatusOK)
	tok := decode[tokenResponse](t, rec)
	if tok.TokenType != "Bearer" || tok.AccessToken == "" || tok.ExpiresIn <= 0 {
		t.Fatalf("unexpected token %+v", tok)
	}

	for name, creds := range map[string]map[string]string{
		"wrong password": {"email": "bob@example.com", "password": "nope-nope"},
		"unknown email":  {"email": "ghost@example.com", "password": "password123"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := api.do(http.MethodPost, "/api/v1/auth/login", "", creds)
			expectStatus(t, rec, http.StatusUnauthorized)
			if decode[errorResponse](t, rec).Error.Code != "invalid_credentials" {
				t.Fatal(rec.Body.String())
			}
		})
	}
}

func TestProtectedRoutesRequireJWT(t *testing.T) {
	api := newTestAPI(t)
	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/users"},
		{http.MethodGet, "/api/v1/users"},
		{http.MethodGet, "/api/v1/users/abc"},
		{http.MethodPatch, "/api/v1/users/abc"},
		{http.MethodDelete, "/api/v1/users/abc"},
	}
	for _, rt := range routes {
		for name, token := range map[string]string{"missing": "", "garbage": "not-a-jwt"} {
			t.Run(rt.method+" "+rt.path+" "+name, func(t *testing.T) {
				rec := api.do(rt.method, rt.path, token, nil)
				expectStatus(t, rec, http.StatusUnauthorized)
			})
		}
	}

	t.Run("wrong scheme", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
		rec := httptest.NewRecorder()
		api.handler.ServeHTTP(rec, req)
		expectStatus(t, rec, http.StatusUnauthorized)
	})
}

func TestUserCRUD(t *testing.T) {
	api := newTestAPI(t)
	me, token := api.registerAndLogin("admin@example.com")

	// Create
	rec := api.do(http.MethodPost, "/api/v1/users", token, map[string]string{
		"name": "Carol", "email": "carol@example.com", "password": "password123",
	})
	expectStatus(t, rec, http.StatusCreated)
	carol := decode[userResponse](t, rec)

	// Get
	rec = api.do(http.MethodGet, "/api/v1/users/"+carol.ID, token, nil)
	expectStatus(t, rec, http.StatusOK)
	if got := decode[userResponse](t, rec); got.Email != "carol@example.com" {
		t.Fatalf("unexpected %+v", got)
	}
	expectStatus(t, api.do(http.MethodGet, "/api/v1/users/does-not-exist", token, nil), http.StatusNotFound)

	// List
	rec = api.do(http.MethodGet, "/api/v1/users", token, nil)
	expectStatus(t, rec, http.StatusOK)
	if list := decode[[]userResponse](t, rec); len(list) != 2 {
		t.Fatalf("unexpected list %+v", list)
	}

	// Update name only
	rec = api.do(http.MethodPatch, "/api/v1/users/"+carol.ID, token, map[string]string{"name": "Carol B"})
	expectStatus(t, rec, http.StatusOK)
	if got := decode[userResponse](t, rec); got.Name != "Carol B" || got.Email != "carol@example.com" {
		t.Fatalf("unexpected %+v", got)
	}
	// Update email
	rec = api.do(http.MethodPatch, "/api/v1/users/"+carol.ID, token, map[string]string{"email": "carol.b@example.com"})
	expectStatus(t, rec, http.StatusOK)
	// Email taken
	expectStatus(t, api.do(http.MethodPatch, "/api/v1/users/"+carol.ID, token, map[string]string{"email": me.Email}), http.StatusConflict)
	// Empty body / invalid email
	expectStatus(t, api.do(http.MethodPatch, "/api/v1/users/"+carol.ID, token, map[string]string{}), http.StatusBadRequest)
	expectStatus(t, api.do(http.MethodPatch, "/api/v1/users/"+carol.ID, token, map[string]string{"email": "x"}), http.StatusBadRequest)
	// Password is not updatable through this endpoint
	expectStatus(t, api.do(http.MethodPatch, "/api/v1/users/"+carol.ID, token, map[string]string{"password": "newpassword"}), http.StatusBadRequest)
	// Unknown user
	expectStatus(t, api.do(http.MethodPatch, "/api/v1/users/nope", token, map[string]string{"name": "x"}), http.StatusNotFound)

	rec = api.do(http.MethodDelete, "/api/v1/users/"+carol.ID, token, nil)
	expectStatus(t, rec, http.StatusNoContent)
	if rec.Body.Len() != 0 {
		t.Fatal("204 must have no body")
	}
	expectStatus(t, api.do(http.MethodDelete, "/api/v1/users/"+carol.ID, token, nil), http.StatusNotFound)
	expectStatus(t, api.do(http.MethodGet, "/api/v1/users/"+carol.ID, token, nil), http.StatusNotFound)
}

func TestMethodNotAllowed(t *testing.T) {
	api := newTestAPI(t)
	expectStatus(t, api.do(http.MethodPut, "/api/v1/users/abc", "", nil), http.StatusMethodNotAllowed)
}

type failingUsers struct{ port.UserService }

func (failingUsers) Get(context.Context, string) (*domain.User, error) {
	return nil, errors.New("mongo: connection refused at 10.0.0.5")
}

type allowAllAuth struct{ port.AuthService }

func (allowAllAuth) VerifyToken(context.Context, string) error {
	return nil
}

func TestInternalErrorsAreNotLeaked(t *testing.T) {
	logs := &bytes.Buffer{}
	h := NewRouter(failingUsers{}, allowAllAuth{}, slog.New(slog.NewTextHandler(logs, nil)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/x", nil)
	req.Header.Set("Authorization", "Bearer anything")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	expectStatus(t, rec, http.StatusInternalServerError)
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Fatal("internal error details leaked to client")
	}
	if !strings.Contains(logs.String(), "10.0.0.5") {
		t.Fatal("internal error should be logged server-side")
	}
}
