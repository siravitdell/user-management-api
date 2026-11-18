package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoggingMiddleware_RecordsMethodPathStatusDuration(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Millisecond)
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("short and stout"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/teapot?x=1", nil))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log is not JSON: %q", buf.String())
	}
	if entry["method"] != "POST" || entry["path"] != "/teapot" || entry["status"] != float64(418) {
		t.Fatalf("unexpected log entry %v", entry)
	}
	if d, err := time.ParseDuration(entry["duration"].(string)); err != nil || d < 2*time.Millisecond {
		t.Fatalf("duration = %v, want >= 2ms", entry["duration"])
	}
}

func TestLoggingMiddleware_DefaultStatus200(t *testing.T) {
	var buf bytes.Buffer
	h := Logging(slog.New(slog.NewJSONHandler(&buf, nil)))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	var entry map[string]any
	_ = json.Unmarshal(buf.Bytes(), &entry)
	if entry["status"] != float64(200) {
		t.Fatalf("status = %v", entry["status"])
	}
}

func TestRecoverer(t *testing.T) {
	var buf bytes.Buffer
	h := Recoverer(slog.New(slog.NewTextHandler(&buf, nil)))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if !bytes.Contains(buf.Bytes(), []byte("kaboom")) {
		t.Fatal("panic not logged")
	}
}

func TestBearerToken(t *testing.T) {
	cases := map[string]struct {
		token string
		ok    bool
	}{
		"Bearer abc":   {"abc", true},
		"bearer abc":   {"abc", true},
		"Bearer   abc": {"abc", true},
		"Bearer ":      {"", false},
		"Basic abc":    {"", false},
		"abc":          {"", false},
		"":             {"", false},
	}
	for header, want := range cases {
		tok, ok := bearerToken(header)
		if tok != want.token || ok != want.ok {
			t.Errorf("bearerToken(%q) = %q,%v want %q,%v", header, tok, ok, want.token, want.ok)
		}
	}
}
