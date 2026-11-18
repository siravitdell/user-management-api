package security

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/siravitdell/user-management-api/internal/core/domain"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func TestNewJWTManager_Validation(t *testing.T) {
	if _, err := NewJWTManager("short", "iss", time.Hour); err == nil {
		t.Fatal("expected error for short secret")
	}
	if _, err := NewJWTManager(testSecret, "iss", 0); err == nil {
		t.Fatal("expected error for zero ttl")
	}
}

func TestJWT_RoundTrip(t *testing.T) {
	m, err := NewJWTManager(testSecret, "user-api", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := m.Generate("user-1", "a@b.co")
	if err != nil {
		t.Fatal(err)
	}
	if tok.TokenType != "Bearer" || strings.Count(tok.AccessToken, ".") != 2 {
		t.Fatalf("unexpected token %+v", tok)
	}

	parsed, _, err := jwt.NewParser().ParseUnverified(tok.AccessToken, &accessClaims{})
	if err != nil || parsed.Method.Alg() != "HS256" {
		t.Fatalf("alg=%v err=%v", parsed.Method.Alg(), err)
	}

	if c := parsed.Claims.(*accessClaims); c.Subject != "user-1" || c.Email != "a@b.co" {
		t.Fatalf("unexpected claims %+v", c)
	}
	if err := m.Verify(tok.AccessToken); err != nil {
		t.Fatal(err)
	}
}

func TestJWT_Rejections(t *testing.T) {
	m, _ := NewJWTManager(testSecret, "user-api", time.Hour)
	good, _ := m.Generate("user-1", "a@b.co")

	other, _ := NewJWTManager("ffffffffffffffffffffffffffffffff", "user-api", time.Hour)
	foreign, _ := other.Generate("user-1", "a@b.co")

	wrongIss, _ := NewJWTManager(testSecret, "someone-else", time.Hour)
	wrongIssTok, _ := wrongIss.Generate("user-1", "a@b.co")

	expiredMgr, _ := NewJWTManager(testSecret, "user-api", time.Minute)
	expiredMgr.now = func() time.Time { return time.Now().Add(-time.Hour) }
	expired, _ := expiredMgr.Generate("user-1", "a@b.co")

	noneTok, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": "user-1", "iss": "user-api", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)

	hs512Tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{
		"sub": "user-1", "iss": "user-api", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(testSecret))

	noExpTok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-1", "iss": "user-api",
	}).SignedString([]byte(testSecret))

	cases := map[string]string{
		"garbage":          "not.a.jwt",
		"wrong secret":     foreign.AccessToken,
		"wrong issuer":     wrongIssTok.AccessToken,
		"expired":          expired.AccessToken,
		"alg none":         noneTok,
		"other HMAC alg":   hs512Tok,
		"missing exp":      noExpTok,
		"tampered payload": good.AccessToken[:len(good.AccessToken)-2] + "xx",
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if err := m.Verify(tok); !errors.Is(err, domain.ErrInvalidToken) {
				t.Fatalf("expected ErrInvalidToken, got %v", err)
			}
		})
	}
}

func TestBcryptHasher(t *testing.T) {
	h, err := NewBcryptHasher(bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := h.Hash("password123")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "password123" || !strings.HasPrefix(hash, "$2a$") {
		t.Fatalf("unexpected hash %q", hash)
	}
	if err := h.Compare(hash, "password123"); err != nil {
		t.Fatalf("expected match: %v", err)
	}
	if err := h.Compare(hash, "wrong"); err == nil {
		t.Fatal("expected mismatch")
	}
	if _, err := NewBcryptHasher(99); err == nil {
		t.Fatal("expected invalid cost error")
	}
}
