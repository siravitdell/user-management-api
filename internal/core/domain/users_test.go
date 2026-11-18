package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		email string
		ok    bool
	}{
		{"alice@example.com", true},
		{"a.b+tag@sub.example.co.th", true},
		{"", false},
		{"not-an-email", false},
		{"alice@localhost", false},
		{"Alice <alice@example.com>", false},
		{"alice@.com", false},
		{"alice@example.", false},
		{strings.Repeat("a", 250) + "@x.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			if got := ValidateEmail(tt.email) == nil; got != tt.ok {
				t.Fatalf("ValidateEmail(%q) ok=%v, want %v", tt.email, got, tt.ok)
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	if ValidateName("Alice") != nil {
		t.Fatal("expected valid name")
	}
	if ValidateName("") == nil {
		t.Fatal("expected empty name to fail")
	}
	if ValidateName(strings.Repeat("ก", 101)) == nil {
		t.Fatal("expected 101-rune name to fail")
	}
	if ValidateName(strings.Repeat("ก", 100)) != nil {
		t.Fatal("expected 100-rune (multi-byte) name to pass")
	}
}

func TestValidatePassword(t *testing.T) {
	cases := map[string]bool{
		"":                      false,
		"short":                 false,
		"longenough":            true,
		strings.Repeat("x", 72): true,
		strings.Repeat("x", 73): false,
	}
	for pw, ok := range cases {
		if got := ValidatePassword(pw) == nil; got != ok {
			t.Errorf("ValidatePassword(len=%d) ok=%v, want %v", len(pw), got, ok)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  Alice@Example.COM "); got != "alice@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestNewValidationError(t *testing.T) {
	if err := NewValidationError(nil, nil); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	err := NewValidationError(&FieldError{Field: "name", Message: "is required"}, nil)
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Fields) != 1 {
		t.Fatalf("expected one field error, got %v", err)
	}
	if ve.Error() != "validation failed: name is required" {
		t.Fatalf("unexpected message %q", ve.Error())
	}
}
