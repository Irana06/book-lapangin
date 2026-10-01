package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Env         string
	Addr        string
	DatabaseURL string
}

// Load membaca konfigurasi dari environment variable.
// File .env (kalau ada) dimuat dulu supaya enak untuk development lokal.
func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		Env:         getenv("APP_ENV", "development"),
		Addr:        getenv("APP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL belum di-set (lihat .env.example)")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
