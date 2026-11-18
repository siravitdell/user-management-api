package service

import (
	"context"
	"fmt"
	"time"

	"github.com/siravitdell/user-management-api/internal/core/domain"
	"github.com/siravitdell/user-management-api/internal/core/port"
)

type UserService struct {
	repo   port.UserRepository
	hasher port.PasswordHasher
	now    func() time.Time
}

var _ port.UserService = (*UserService)(nil)

func NewUserService(repo port.UserRepository, hasher port.PasswordHasher) *UserService {
	return &UserService{repo: repo, hasher: hasher, now: func() time.Time { return time.Now().UTC() }}
}

func (s *UserService) Create(ctx context.Context, in port.CreateUserInput) (*domain.User, error) {
	name := domain.NormalizeName(in.Name)
	email := domain.NormalizeEmail(in.Email)
	if err := domain.NewValidationError(
		domain.ValidateName(name),
		domain.ValidateEmail(email),
		domain.ValidatePassword(in.Password),
	); err != nil {
		return nil, err
	}

	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &domain.User{
		Name:         name,
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    s.now().Truncate(time.Millisecond),
	}
	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) Get(ctx context.Context, id string) (*domain.User, error) {
	if id == "" {
		return nil, domain.ErrUserNotFound
	}
	return s.repo.GetByID(ctx, id)
}

func (s *UserService) List(ctx context.Context) ([]domain.User, error) {
	return s.repo.List(ctx)
}

func (s *UserService) Update(ctx context.Context, id string, in port.UpdateUserInput) (*domain.User, error) {
	var upd domain.UserUpdate
	var errs []*domain.FieldError

	if in.Name != nil {
		name := domain.NormalizeName(*in.Name)
		errs = append(errs, domain.ValidateName(name))
		upd.Name = &name
	}
	if in.Email != nil {
		email := domain.NormalizeEmail(*in.Email)
		errs = append(errs, domain.ValidateEmail(email))
		upd.Email = &email
	}
	if upd.IsEmpty() {
		errs = append(errs, &domain.FieldError{Field: "body", Message: "at least one of name or email is required"})
	}
	if err := domain.NewValidationError(errs...); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, domain.ErrUserNotFound
	}
	return s.repo.Update(ctx, id, upd)
}

func (s *UserService) Delete(ctx context.Context, id string) error {
	if id == "" {
		return domain.ErrUserNotFound
	}
	return s.repo.Delete(ctx, id)
}

func (s *UserService) Count(ctx context.Context) (int64, error) {
	return s.repo.Count(ctx)
}
