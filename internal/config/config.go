package config

import (
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort          string
	AppHost          string
	AppEnv           string
	MySQLDSN         string
	MySQLMaxOpenConns int
	MySQLMaxIdleConns int
	RedisAddr        string
	RedisPassword    string
	RedisDB          int
	CacheEnabled     bool
	RateLimitPerMin  int
	TTLProductDetail time.Duration
	TTLProductList   time.Duration
	TTLTopProducts   time.Duration
	TTLDashboard     time.Duration
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		slog.Info("No .env file found or error loading, reading directly from environment variables")
	}

	return &Config{
		AppPort:          getEnv("APP_PORT", "8080"),
		AppHost:          getEnv("APP_HOST", "0.0.0.0"),
		AppEnv:           getEnv("APP_ENV", "development"),
		MySQLDSN:         getEnv("MYSQL_DSN", "root:@tcp(192.168.54.202:3306)/redis_case?parseTime=true&charset=utf8mb4"),
		MySQLMaxOpenConns: getEnvInt("MYSQL_MAX_OPEN_CONNS", 20),
		MySQLMaxIdleConns: getEnvInt("MYSQL_MAX_IDLE_CONNS", 10),
		RedisAddr:        getEnv("REDIS_ADDR", "192.168.54.202:6379"),
		RedisPassword:    getEnv("REDIS_PASSWORD", ""),
		RedisDB:          getEnvInt("REDIS_DB", 0),
		CacheEnabled:     getEnvBool("CACHE_ENABLED", true),
		RateLimitPerMin:  getEnvInt("RATE_LIMIT_PER_MIN", 100),
		TTLProductDetail: time.Duration(getEnvInt("TTL_PRODUCT_DETAIL", 300)) * time.Second,
		TTLProductList:   time.Duration(getEnvInt("TTL_PRODUCT_LIST", 120)) * time.Second,
		TTLTopProducts:   time.Duration(getEnvInt("TTL_TOP_PRODUCTS", 600)) * time.Second,
		TTLDashboard:     time.Duration(getEnvInt("TTL_DASHBOARD", 60)) * time.Second,
	}
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseBool(val); err == nil {
			return parsed
		}
	}
	return defaultVal
}
