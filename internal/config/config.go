package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr          string
	GRPCAddr          string
	MongoURI          string
	MongoDatabase     string
	JWTSecret         string
	JWTIssuer         string
	JWTTTL            time.Duration
	UserCountInterval time.Duration
}

func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	var errs []error
	duration := func(key, def string) time.Duration {
		d, err := time.ParseDuration(get(key, def))
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("%s must be a positive duration (e.g. 10s, 1h)", key))
		}
		return d
	}

	cfg := Config{
		HTTPAddr:          get("HTTP_ADDR", ":8080"),
		GRPCAddr:          get("GRPC_ADDR", ":50051"),
		MongoURI:          get("MONGO_URI", "mongodb://localhost:27017"),
		MongoDatabase:     get("MONGO_DATABASE", "user_management"),
		JWTSecret:         getenv("JWT_SECRET"),
		JWTIssuer:         get("JWT_ISSUER", "user-management-api"),
		JWTTTL:            duration("JWT_TTL", "1h"),
		UserCountInterval: duration("USER_COUNT_INTERVAL", "10s"),
	}

	if len(cfg.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET is required and must be at least 32 bytes (try: openssl rand -base64 48)"))
	}
	return cfg, errors.Join(errs...)
}
