package store

import (
	"context"
	"os"
	"strconv"
	"strings"
)

func NewConfiguredStore(ctx context.Context) (Store, error) {
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	seed := parseBoolEnv("SEED_DEMO_DATA", true)

	if dsn == "" {
		if seed {
			return NewSeededMemoryStore(), nil
		}
		return NewMemoryStore(), nil
	}

	return NewPostgresStore(ctx, dsn, seed)
}

func parseBoolEnv(name string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}
