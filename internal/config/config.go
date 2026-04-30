package config

import (
	"fmt"
	"os"
)

type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	HASdkModelDir string // unused until cgoClient is built with -tags hasdk

	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	MinioUseSSL    bool
}

func Load() (*Config, error) {
	c := &Config{
		HTTPAddr:      getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		HASdkModelDir: os.Getenv("HASDK_MODEL_DIR"),

		MinioEndpoint:  getenv("MINIO_ENDPOINT", "localhost:9000"),
		MinioAccessKey: getenv("MINIO_ACCESS_KEY", "minioadmin"),
		MinioSecretKey: getenv("MINIO_SECRET_KEY", "minioadmin"),
		MinioBucket:    getenv("MINIO_BUCKET", "face-photos"),
		MinioUseSSL:    os.Getenv("MINIO_USE_SSL") == "true",
	}
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	return c, nil
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
