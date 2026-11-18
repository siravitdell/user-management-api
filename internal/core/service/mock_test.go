package service

import (
	"context"
	"errors"

	"github.com/siravitdell/user-management-api/internal/core/domain"
	"github.com/siravitdell/user-management-api/internal/core/port"
)

type mockRepo struct {
	CreateFn     func(ctx context.Context, u *domain.User) error
	GetByIDFn    func(ctx context.Context, id string) (*domain.User, error)
	GetByEmailFn func(ctx context.Context, email string) (*domain.User, error)
	ListFn       func(ctx context.Context) ([]domain.User, error)
	UpdateFn     func(ctx context.Context, id string, upd domain.UserUpdate) (*domain.User, error)
	DeleteFn     func(ctx context.Context, id string) error
	CountFn      func(ctx context.Context) (int64, error)

	calls map[string]int
}

var errNotStubbed = errors.New("mock method not stubbed")

var _ port.UserRepository = (*mockRepo)(nil)

func (m *mockRepo) record(name string) {
	if m.calls == nil {
		m.calls = map[string]int{}
	}
	m.calls[name]++
}

func (m *mockRepo) Create(ctx context.Context, u *domain.User) error {
	m.record("Create")
	if m.CreateFn == nil {
		return errNotStubbed
	}
	return m.CreateFn(ctx, u)
}

func (m *mockRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	m.record("GetByID")
	if m.GetByIDFn == nil {
		return nil, errNotStubbed
	}
	return m.GetByIDFn(ctx, id)
}

func (m *mockRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	m.record("GetByEmail")
	if m.GetByEmailFn == nil {
		return nil, errNotStubbed
	}
	return m.GetByEmailFn(ctx, email)
}

func (m *mockRepo) List(ctx context.Context) ([]domain.User, error) {
	m.record("List")
	if m.ListFn == nil {
		return nil, errNotStubbed
	}
	return m.ListFn(ctx)
}

func (m *mockRepo) Update(ctx context.Context, id string, upd domain.UserUpdate) (*domain.User, error) {
	m.record("Update")
	if m.UpdateFn == nil {
		return nil, errNotStubbed
	}
	return m.UpdateFn(ctx, id, upd)
}

func (m *mockRepo) Delete(ctx context.Context, id string) error {
	m.record("Delete")
	if m.DeleteFn == nil {
		return errNotStubbed
	}
	return m.DeleteFn(ctx, id)
}

func (m *mockRepo) Count(ctx context.Context) (int64, error) {
	m.record("Count")
	if m.CountFn == nil {
		return 0, errNotStubbed
	}
	return m.CountFn(ctx)
}

type fakeHasher struct {
	hashErr     error
	compareHits int
}

func (h *fakeHasher) Hash(pw string) (string, error) {
	if h.hashErr != nil {
		return "", h.hashErr
	}
	return "hashed:" + pw, nil
}

func (h *fakeHasher) Compare(hash, pw string) error {
	h.compareHits++
	if hash != "hashed:"+pw {
		return errors.New("mismatch")
	}
	return nil
}

type fakeTokens struct {
	genErr error
}

func (f *fakeTokens) Generate(userID, email string) (*port.Token, error) {
	if f.genErr != nil {
		return nil, f.genErr
	}
	return &port.Token{AccessToken: "token-for-" + userID, TokenType: "Bearer"}, nil
}

func (f *fakeTokens) Verify(token string) error {
	if token != "valid" {
		return domain.ErrInvalidToken
	}
	return nil
}

func ptr[T any](v T) *T { return &v }
