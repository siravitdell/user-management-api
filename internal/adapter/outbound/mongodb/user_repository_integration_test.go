package mongodb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/siravitdell/user-management-api/internal/core/domain"
)

func newTestRepo(t *testing.T) *UserRepository {
	t.Helper()
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		t.Skip("MONGO_TEST_URI not set; skipping MongoDB integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := Connect(ctx, uri)
	if err != nil {
		t.Fatal(err)
	}
	db := client.Database(fmt.Sprintf("user_api_test_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	repo, err := NewUserRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestUserRepository_CRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	u := &domain.User{Name: "Alice", Email: "alice@example.com", PasswordHash: "h", CreatedAt: time.Now().UTC().Truncate(time.Millisecond)}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if u.ID == "" {
		t.Fatal("ID not set")
	}

	dup := &domain.User{Name: "Alice2", Email: "alice@example.com", PasswordHash: "h", CreatedAt: time.Now()}
	if err := repo.Create(ctx, dup); !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil || got.Email != u.Email || !got.CreatedAt.Equal(u.CreatedAt) {
		t.Fatalf("GetByID = %+v, %v", got, err)
	}
	if _, err := repo.GetByEmail(ctx, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByID(ctx, "not-an-object-id"); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}

	bob := &domain.User{Name: "Bob", Email: "bob@example.com", PasswordHash: "h", CreatedAt: time.Now().UTC()}
	if err := repo.Create(ctx, bob); err != nil {
		t.Fatal(err)
	}

	newName := "Alice Updated"
	upd, err := repo.Update(ctx, u.ID, domain.UserUpdate{Name: &newName})
	if err != nil || upd.Name != newName || upd.Email != u.Email {
		t.Fatalf("Update = %+v, %v", upd, err)
	}
	taken := "bob@example.com"
	if _, err := repo.Update(ctx, u.ID, domain.UserUpdate{Email: &taken}); !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("expected duplicate on update, got %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil || len(list) != 2 || list[0].ID != u.ID || list[1].ID != bob.ID {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if n, err := repo.Count(ctx); err != nil || n != 2 {
		t.Fatalf("Count = %d, %v", n, err)
	}

	if err := repo.Delete(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, u.ID); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("expected not found on second delete, got %v", err)
	}
}
