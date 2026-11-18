package service

import (
	"context"
	"errors"
	"testing"

	"github.com/siravitdell/user-management-api/internal/core/domain"
)

func newTestAuth(t *testing.T, repo *mockRepo, hasher *fakeHasher, tokens *fakeTokens) *AuthService {
	t.Helper()
	auth, err := NewAuthService(repo, hasher, tokens)
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func TestAuthService_Login(t *testing.T) {
	ctx := context.Background()
	alice := &domain.User{ID: "u-1", Email: "alice@example.com", PasswordHash: "hashed:correct-horse"}
	repo := &mockRepo{GetByEmailFn: func(_ context.Context, email string) (*domain.User, error) {
		if email == alice.Email {
			return alice, nil
		}
		return nil, domain.ErrUserNotFound
	}}

	t.Run("success with case-insensitive email", func(t *testing.T) {
		auth := newTestAuth(t, repo, &fakeHasher{}, &fakeTokens{})
		tok, err := auth.Login(ctx, " ALICE@example.com ", "correct-horse")
		if err != nil {
			t.Fatal(err)
		}
		if tok.AccessToken != "token-for-u-1" {
			t.Fatalf("unexpected token %q", tok.AccessToken)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		auth := newTestAuth(t, repo, &fakeHasher{}, &fakeTokens{})
		if _, err := auth.Login(ctx, alice.Email, "wrong"); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials, got %v", err)
		}
	})

	t.Run("unknown email still runs a hash comparison", func(t *testing.T) {
		h := &fakeHasher{}
		auth := newTestAuth(t, repo, h, &fakeTokens{})
		if _, err := auth.Login(ctx, "nobody@example.com", "whatever"); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials, got %v", err)
		}
		if h.compareHits != 1 {
			t.Fatalf("expected 1 dummy comparison, got %d", h.compareHits)
		}
	})

	t.Run("empty credentials", func(t *testing.T) {
		auth := newTestAuth(t, repo, &fakeHasher{}, &fakeTokens{})
		if _, err := auth.Login(ctx, "", ""); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials, got %v", err)
		}
	})

	t.Run("repository failure is not masked as bad credentials", func(t *testing.T) {
		failing := &mockRepo{GetByEmailFn: func(context.Context, string) (*domain.User, error) { return nil, errors.New("db down") }}
		auth := newTestAuth(t, failing, &fakeHasher{}, &fakeTokens{})
		_, err := auth.Login(ctx, alice.Email, "correct-horse")
		if err == nil || errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("expected infrastructure error, got %v", err)
		}
	})

	t.Run("token generation failure", func(t *testing.T) {
		auth := newTestAuth(t, repo, &fakeHasher{}, &fakeTokens{genErr: errors.New("sign failed")})
		if _, err := auth.Login(ctx, alice.Email, "correct-horse"); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestAuthService_VerifyToken(t *testing.T) {
	auth := newTestAuth(t, &mockRepo{}, &fakeHasher{}, &fakeTokens{})
	ctx := context.Background()
	if err := auth.VerifyToken(ctx, "valid"); err != nil {
		t.Fatalf("expected valid token, got %v", err)
	}
	if err := auth.VerifyToken(ctx, ""); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
	if err := auth.VerifyToken(ctx, "garbage"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestNewAuthService_HasherFailure(t *testing.T) {
	_, err := NewAuthService(&mockRepo{}, &fakeHasher{hashErr: errors.New("x")}, &fakeTokens{})
	if err == nil {
		t.Fatal("expected error")
	}
}
