package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/siravitdell/user-management-api/internal/core/domain"
	"github.com/siravitdell/user-management-api/internal/core/port"
)

func newTestUserService(repo *mockRepo) *UserService {
	svc := NewUserService(repo, &fakeHasher{})
	svc.now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 999_999_999, time.UTC) }
	return svc
}

func TestUserService_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("success normalizes input and hashes password", func(t *testing.T) {
		var stored *domain.User
		repo := &mockRepo{CreateFn: func(_ context.Context, u *domain.User) error {
			u.ID = "id-1"
			stored = u
			return nil
		}}
		svc := newTestUserService(repo)

		got, err := svc.Create(ctx, port.CreateUserInput{Name: "  Alice ", Email: " Alice@Example.COM", Password: "s3cretpass"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "id-1" || got.Name != "Alice" || got.Email != "alice@example.com" {
			t.Fatalf("unexpected user: %+v", got)
		}
		if stored.PasswordHash != "hashed:s3cretpass" {
			t.Fatalf("password not hashed: %q", stored.PasswordHash)
		}
		if want := time.Date(2026, 1, 2, 3, 4, 5, 999_000_000, time.UTC); !got.CreatedAt.Equal(want) {
			t.Fatalf("CreatedAt=%v want %v (ms precision)", got.CreatedAt, want)
		}
	})

	t.Run("validation errors are aggregated and repo not called", func(t *testing.T) {
		repo := &mockRepo{}
		svc := newTestUserService(repo)

		_, err := svc.Create(ctx, port.CreateUserInput{Name: "", Email: "bad", Password: "123"})
		var ve *domain.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("expected ValidationError, got %v", err)
		}
		if len(ve.Fields) != 3 {
			t.Fatalf("expected 3 field errors, got %+v", ve.Fields)
		}
		if repo.calls["Create"] != 0 {
			t.Fatal("repository must not be called on invalid input")
		}
	})

	t.Run("duplicate email propagates", func(t *testing.T) {
		repo := &mockRepo{CreateFn: func(context.Context, *domain.User) error { return domain.ErrEmailAlreadyExists }}
		_, err := newTestUserService(repo).Create(ctx, port.CreateUserInput{Name: "A", Email: "a@b.co", Password: "password1"})
		if !errors.Is(err, domain.ErrEmailAlreadyExists) {
			t.Fatalf("expected ErrEmailAlreadyExists, got %v", err)
		}
	})

	t.Run("hasher failure is wrapped", func(t *testing.T) {
		svc := NewUserService(&mockRepo{}, &fakeHasher{hashErr: errors.New("boom")})
		_, err := svc.Create(ctx, port.CreateUserInput{Name: "A", Email: "a@b.co", Password: "password1"})
		if err == nil || err.Error() != "hash password: boom" {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestUserService_Get(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{GetByIDFn: func(_ context.Context, id string) (*domain.User, error) {
		if id == "known" {
			return &domain.User{ID: id}, nil
		}
		return nil, domain.ErrUserNotFound
	}}
	svc := newTestUserService(repo)

	if u, err := svc.Get(ctx, "known"); err != nil || u.ID != "known" {
		t.Fatalf("got %v, %v", u, err)
	}
	if _, err := svc.Get(ctx, "missing"); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	if _, err := svc.Get(ctx, ""); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("expected not found for empty id, got %v", err)
	}
}

func TestUserService_List(t *testing.T) {
	ctx := context.Background()

	t.Run("returns all users", func(t *testing.T) {
		repo := &mockRepo{ListFn: func(context.Context) ([]domain.User, error) {
			return []domain.User{{ID: "a"}, {ID: "b"}}, nil
		}}
		users, err := newTestUserService(repo).List(ctx)
		if err != nil || len(users) != 2 {
			t.Fatalf("got %+v, %v", users, err)
		}
	})

	t.Run("repository error", func(t *testing.T) {
		repo := &mockRepo{ListFn: func(context.Context) ([]domain.User, error) { return nil, errors.New("db down") }}
		if _, err := newTestUserService(repo).List(ctx); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestUserService_Update(t *testing.T) {
	ctx := context.Background()

	t.Run("updates normalized fields", func(t *testing.T) {
		var got domain.UserUpdate
		repo := &mockRepo{UpdateFn: func(_ context.Context, id string, upd domain.UserUpdate) (*domain.User, error) {
			got = upd
			return &domain.User{ID: id, Name: *upd.Name, Email: *upd.Email}, nil
		}}
		u, err := newTestUserService(repo).Update(ctx, "id-1", port.UpdateUserInput{Name: ptr(" Bob "), Email: ptr("BOB@x.io")})
		if err != nil {
			t.Fatal(err)
		}
		if *got.Name != "Bob" || *got.Email != "bob@x.io" || u.ID != "id-1" {
			t.Fatalf("unexpected update %+v / %+v", got, u)
		}
	})

	t.Run("name only leaves email nil", func(t *testing.T) {
		repo := &mockRepo{UpdateFn: func(_ context.Context, id string, upd domain.UserUpdate) (*domain.User, error) {
			if upd.Email != nil {
				t.Fatal("email should be nil")
			}
			return &domain.User{ID: id}, nil
		}}
		if _, err := newTestUserService(repo).Update(ctx, "id-1", port.UpdateUserInput{Name: ptr("Bob")}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("empty update rejected", func(t *testing.T) {
		repo := &mockRepo{}
		_, err := newTestUserService(repo).Update(ctx, "id-1", port.UpdateUserInput{})
		var ve *domain.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("expected validation error, got %v", err)
		}
		if repo.calls["Update"] != 0 {
			t.Fatal("repository must not be called")
		}
	})

	t.Run("invalid email rejected", func(t *testing.T) {
		_, err := newTestUserService(&mockRepo{}).Update(ctx, "id-1", port.UpdateUserInput{Email: ptr("nope")})
		var ve *domain.ValidationError
		if !errors.As(err, &ve) || ve.Fields[0].Field != "email" {
			t.Fatalf("expected email validation error, got %v", err)
		}
	})

	t.Run("not found propagates", func(t *testing.T) {
		repo := &mockRepo{UpdateFn: func(context.Context, string, domain.UserUpdate) (*domain.User, error) {
			return nil, domain.ErrUserNotFound
		}}
		if _, err := newTestUserService(repo).Update(ctx, "x", port.UpdateUserInput{Name: ptr("Bob")}); !errors.Is(err, domain.ErrUserNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}

func TestUserService_DeleteAndCount(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{
		DeleteFn: func(_ context.Context, id string) error {
			if id != "id-1" {
				return domain.ErrUserNotFound
			}
			return nil
		},
		CountFn: func(context.Context) (int64, error) { return 7, nil },
	}
	svc := newTestUserService(repo)

	if err := svc.Delete(ctx, "id-1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, "other"); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	if n, err := svc.Count(ctx); err != nil || n != 7 {
		t.Fatalf("Count = %d, %v", n, err)
	}
}
