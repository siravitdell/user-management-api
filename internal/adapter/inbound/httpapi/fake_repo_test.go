package httpapi

import (
	"context"
	"strconv"
	"sync"

	"github.com/siravitdell/user-management-api/internal/core/domain"
)

// fakeRepo is an in-memory port.UserRepository used to test the HTTP layer
// without MongoDB.
type fakeRepo struct {
	mu     sync.Mutex
	users  []domain.User
	nextID int
}

func (r *fakeRepo) find(id string) int {
	for i, u := range r.users {
		if u.ID == id {
			return i
		}
	}
	return -1
}

func (r *fakeRepo) emailTaken(email string) bool {
	for _, u := range r.users {
		if u.Email == email {
			return true
		}
	}
	return false
}

func (r *fakeRepo) Create(_ context.Context, u *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.emailTaken(u.Email) {
		return domain.ErrEmailAlreadyExists
	}
	r.nextID++
	u.ID = strconv.Itoa(r.nextID)
	r.users = append(r.users, *u)
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.find(id)
	if i < 0 {
		return nil, domain.ErrUserNotFound
	}
	u := r.users[i]
	return &u, nil
}

func (r *fakeRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

func (r *fakeRepo) List(context.Context) ([]domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.User(nil), r.users...), nil
}

func (r *fakeRepo) Update(_ context.Context, id string, upd domain.UserUpdate) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.find(id)
	if i < 0 {
		return nil, domain.ErrUserNotFound
	}
	if upd.Email != nil && *upd.Email != r.users[i].Email {
		if r.emailTaken(*upd.Email) {
			return nil, domain.ErrEmailAlreadyExists
		}
		r.users[i].Email = *upd.Email
	}
	if upd.Name != nil {
		r.users[i].Name = *upd.Name
	}
	u := r.users[i]
	return &u, nil
}

func (r *fakeRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.find(id)
	if i < 0 {
		return domain.ErrUserNotFound
	}
	r.users = append(r.users[:i], r.users[i+1:]...)
	return nil
}

func (r *fakeRepo) Count(context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return int64(len(r.users)), nil
}
