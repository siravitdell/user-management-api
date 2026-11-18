package port

import (
	"context"
	"time"

	"github.com/siravitdell/user-management-api/internal/core/domain"
)

type CreateUserInput struct {
	Name     string
	Email    string
	Password string
}

type UpdateUserInput struct {
	Name  *string
	Email *string
}

type UserService interface {
	Create(ctx context.Context, in CreateUserInput) (*domain.User, error)
	Get(ctx context.Context, id string) (*domain.User, error)
	List(ctx context.Context) ([]domain.User, error)
	Update(ctx context.Context, id string, in UpdateUserInput) (*domain.User, error)
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int64, error)
}

type Token struct {
	AccessToken string
	TokenType   string
	ExpiresAt   time.Time
}

type AuthService interface {
	Login(ctx context.Context, email, password string) (*Token, error)
	VerifyToken(ctx context.Context, token string) error
}

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	List(ctx context.Context) ([]domain.User, error)
	Update(ctx context.Context, id string, update domain.UserUpdate) (*domain.User, error)
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int64, error)
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

type TokenManager interface {
	Generate(userID, email string) (*Token, error)
	Verify(token string) error
}
