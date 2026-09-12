package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL string
	Origin      string
	Address     string
	UploadDir   string
	WebDir      string
	Production  bool
}

func Load() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), Origin: os.Getenv("APP_ORIGIN"), Address: env("HTTP_ADDR", "127.0.0.1:8080"), UploadDir: env("UPLOAD_DIR", "../.local/uploads"), WebDir: env("WEB_DIR", "../web/dist")}
	mode := os.Getenv("APP_ENV")
	if mode != "development" && mode != "production" {
		return c, fmt.Errorf("APP_ENV must be development or production")
	}
	c.Production = mode == "production"
	u, err := url.Parse(c.Origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return c, fmt.Errorf("APP_ORIGIN must be an origin without a path")
	}
	if c.Production && u.Scheme != "https" {
		return c, fmt.Errorf("production requires HTTPS APP_ORIGIN")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	return c, nil
}
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
