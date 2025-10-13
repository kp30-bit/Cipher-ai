package config

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	Port        string
	MongoURI    string
	MongoDBName string
	Env         string // local or prod
	Host        string
}

// LoadConfig loads environment-specific config safely
func LoadConfig() (*Config, error) {
	env := os.Getenv("CONFIG_ENV")
	if env == "" {
		env = "local" // default to local
	}

	// Load .env file for local development only
	if env == "local" {
		if err := godotenv.Load(".env.local"); err != nil {
			log.Println("⚠️ No .env.local file found, using system environment variables")
		}
	}

	viper.AutomaticEnv() // read environment variables for prod in containerized application

	cfg := &Config{
		Env:         strings.ToLower(env),
		Port:        viper.GetString("PORT"),
		MongoURI:    viper.GetString("MONGO_URI"),
		MongoDBName: viper.GetString("MONGO_DB"),
	}

	// Set hostname dynamically based on environment
	switch cfg.Env {
	case "local":
		if cfg.Port == "" {
			cfg.Port = "8080" // default port
		}
		cfg.Host = "localhost:" + cfg.Port
	case "prod":
		cfg.Host = viper.GetString("HOST") // e.g., shorty.yourdomain.com
		if cfg.Host == "" {
			return nil, fmt.Errorf("HOST environment variable must be set for production")
		}
	default:
		return nil, fmt.Errorf("unknown CONFIG_ENV: %s", cfg.Env)
	}

	// Validate critical values
	if cfg.Port == "" {
		cfg.Port = "8080" // default port
	}
	if cfg.MongoURI == "" || cfg.MongoDBName == "" {
		return nil, fmt.Errorf("missing required configuration for %s environment", cfg.Env)
	}

	// Log safe info only
	log.Printf("📦 Loaded Config: Env=%s, Port=%s, DB=%s", cfg.Env, cfg.Port, cfg.MongoDBName)

	return cfg, nil
}
