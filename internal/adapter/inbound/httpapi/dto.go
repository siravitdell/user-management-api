package httpapi

import (
	"time"

	"github.com/siravitdell/user-management-api/internal/core/domain"
	"github.com/siravitdell/user-management-api/internal/core/port"
)

type createUserRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type updateUserRequest struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
}

type userResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserResponse(u *domain.User) userResponse {
	return userResponse{ID: u.ID, Name: u.Name, Email: u.Email, CreatedAt: u.CreatedAt}
}

func toUserResponses(users []domain.User) []userResponse {
	out := make([]userResponse, 0, len(users))
	for i := range users {
		out = append(out, toUserResponse(&users[i]))
	}
	return out
}

type tokenResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   int64     `json:"expires_in"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func toTokenResponse(t *port.Token, now time.Time) tokenResponse {
	return tokenResponse{
		AccessToken: t.AccessToken,
		TokenType:   t.TokenType,
		ExpiresIn:   int64(t.ExpiresAt.Sub(now).Seconds()),
		ExpiresAt:   t.ExpiresAt.UTC(),
	}
}
