package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultJWTTTLHours = 24

	// 7 days: long enough that clients rarely need to re-authenticate, short
	// enough that a stolen token expires on its own.
	defaultRefreshTokenTTLHours = 168
)

type Config struct {
	Port      string
	DBUrl     string
	JWTSecret string
	JWTTTL    time.Duration
	// RefreshTokenTTL is how long a refresh token stays valid. Rotation gives
	// each individual token this lifetime from the moment it is issued.
	RefreshTokenTTL time.Duration
	BCryptCost      int
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using system env variables")
	}

	jwtSecret := getEnv("JWT_SECRET", "")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is not set: refusing to start without a token signing secret")
	}

	return &Config{
		Port:            getEnv("PORT", "8080"),
		DBUrl:           getEnv("DB_URL", ""),
		JWTSecret:       jwtSecret,
		JWTTTL:          jwtTTLFromEnv(),
		RefreshTokenTTL: refreshTokenTTLFromEnv(),
		BCryptCost:      bcryptCostFromEnv(),
	}
}

// jwtTTLFromEnv reads JWT_EXPIRY_HOURS. An unparseable or non-positive value is
// treated as a configuration error instead of silently falling back to a default.
func jwtTTLFromEnv() time.Duration {
	raw := getEnv("JWT_EXPIRY_HOURS", strconv.Itoa(defaultJWTTTLHours))
	hours, err := strconv.Atoi(raw)
	if err != nil || hours <= 0 {
		log.Fatalf("Invalid JWT_EXPIRY_HOURS %q: must be a positive integer", raw)
	}
	return time.Duration(hours) * time.Hour
}

// refreshTokenTTLFromEnv reads REFRESH_TOKEN_TTL_HOURS with the same fail-fast
// rule as the access-token TTL: a broken value must not silently become a
// default that nobody notices.
func refreshTokenTTLFromEnv() time.Duration {
	raw := getEnv("REFRESH_TOKEN_TTL_HOURS", strconv.Itoa(defaultRefreshTokenTTLHours))
	hours, err := strconv.Atoi(raw)
	if err != nil || hours <= 0 {
		log.Fatalf("Invalid REFRESH_TOKEN_TTL_HOURS %q: must be a positive integer", raw)
	}
	return time.Duration(hours) * time.Hour
}

// bcryptCostFromEnv reads BCRYPT_COST and bounds it to the range bcrypt accepts.
func bcryptCostFromEnv() int {
	raw := getEnv("BCRYPT_COST", strconv.Itoa(bcrypt.DefaultCost))
	cost, err := strconv.Atoi(raw)
	if err != nil || cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		log.Fatalf("Invalid BCRYPT_COST %q: must be an integer between %d and %d", raw, bcrypt.MinCost, bcrypt.MaxCost)
	}
	return cost
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
