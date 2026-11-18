package scheduler

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockCounter struct {
	calls atomic.Int32
	n     int64
	err   error
}

func (m *mockCounter) Count(context.Context) (int64, error) {
	m.calls.Add(1)
	return m.n, m.err
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestUserCountReporter_LogsPeriodicallyAndStops(t *testing.T) {
	logs := &syncBuffer{}
	counter := &mockCounter{n: 42}
	r := NewUserCountReporter(counter, 10*time.Millisecond, slog.New(slog.NewTextHandler(logs, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()

	time.Sleep(55 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reporter did not stop after context cancellation")
	}

	if c := counter.calls.Load(); c < 3 {
		t.Fatalf("expected at least 3 ticks, got %d", c)
	}
	out := logs.String()
	if !strings.Contains(out, "total users in database") || !strings.Contains(out, "count=42") {
		t.Fatalf("expected count log, got:\n%s", out)
	}
	if !strings.Contains(out, "user count reporter stopped") {
		t.Fatal("expected stop log")
	}
}

func TestUserCountReporter_LogsErrors(t *testing.T) {
	logs := &syncBuffer{}
	r := NewUserCountReporter(&mockCounter{err: errors.New("db down")}, time.Hour, slog.New(slog.NewTextHandler(logs, nil)))
	r.report(context.Background())
	if !strings.Contains(logs.String(), "failed to count users") || !strings.Contains(logs.String(), "db down") {
		t.Fatalf("expected error log, got %s", logs.String())
	}
}
