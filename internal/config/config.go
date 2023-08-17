package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	RedisURL string
}

func LoadConfig() Config {
	env := os.Getenv("ENV")
	if env == "" {
		env = "development"
	}
	if env == "development" {
		dotenv := godotenv.Load()
		if dotenv != nil {
			log.Fatal("Error loading .env file")
		}
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Fatal("REDIS_URL must be set")
	}

	return Config{
		RedisURL: redisURL,
	}
}
