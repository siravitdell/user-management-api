package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/siravitdell/user-management-api/internal/core/domain"
	"github.com/siravitdell/user-management-api/internal/core/port"
)

type AuthService struct {
	repo   port.UserRepository
	hasher port.PasswordHasher
	tokens port.TokenManager

	dummyHash string
}

var _ port.AuthService = (*AuthService)(nil)

func NewAuthService(repo port.UserRepository, hasher port.PasswordHasher, tokens port.TokenManager) (*AuthService, error) {
	dummy, err := hasher.Hash("timing-attack-mitigation-dummy")
	if err != nil {
		return nil, fmt.Errorf("prepare dummy hash: %w", err)
	}
	return &AuthService{repo: repo, hasher: hasher, tokens: tokens, dummyHash: dummy}, nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*port.Token, error) {
	email = domain.NormalizeEmail(email)
	if email == "" || password == "" {
		return nil, domain.ErrInvalidCredentials
	}

	user, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			_ = s.hasher.Compare(s.dummyHash, password)
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}
	if err := s.hasher.Compare(user.PasswordHash, password); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	token, err := s.tokens.Generate(user.ID, user.Email)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	return token, nil
}

func (s *AuthService) VerifyToken(_ context.Context, token string) error {
	if token == "" {
		return domain.ErrInvalidToken
	}
	return s.tokens.Verify(token)
}
