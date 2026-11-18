package domain

import (
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxNameLength     = 100
	MaxEmailLength    = 254
	MinPasswordLength = 8
	MaxPasswordBytes  = 72
)

type User struct {
	ID           string
	Name         string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

type UserUpdate struct {
	Name  *string
	Email *string
}

func (u UserUpdate) IsEmpty() bool {
	return u.Name == nil && u.Email == nil
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func NormalizeName(name string) string {
	return strings.TrimSpace(name)
}

func ValidateName(name string) *FieldError {
	switch {
	case name == "":
		return &FieldError{Field: "name", Message: "is required"}
	case utf8.RuneCountInString(name) > MaxNameLength:
		return &FieldError{Field: "name", Message: "must be at most 100 characters"}
	}
	return nil
}

func ValidateEmail(email string) *FieldError {
	if email == "" {
		return &FieldError{Field: "email", Message: "is required"}
	}
	if len(email) > MaxEmailLength {
		return &FieldError{Field: "email", Message: "must be at most 254 characters"}
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || addr.Name != "" {
		return &FieldError{Field: "email", Message: "must be a valid email address"}
	}
	at := strings.LastIndexByte(email, '@')
	if domainPart := email[at+1:]; !strings.Contains(domainPart, ".") ||
		strings.HasPrefix(domainPart, ".") || strings.HasSuffix(domainPart, ".") {
		return &FieldError{Field: "email", Message: "must be a valid email address"}
	}
	return nil
}

func ValidatePassword(password string) *FieldError {
	switch {
	case password == "":
		return &FieldError{Field: "password", Message: "is required"}
	case utf8.RuneCountInString(password) < MinPasswordLength:
		return &FieldError{Field: "password", Message: "must be at least 8 characters"}
	case len(password) > MaxPasswordBytes:
		return &FieldError{Field: "password", Message: "must be at most 72 bytes"}
	}
	return nil
}
