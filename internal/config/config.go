package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	RedisURL             string
	Port                 string
	CheckIntervalMinutes int
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "7900"
	}

	interval := 1
	if val := os.Getenv("CHECK_INTERVAL_MINUTES"); val != "" {
		parsed, err := strconv.Atoi(val)
		if err != nil {
			log.Fatal("invalid CHECK_INTERVAL_MINUTES: must be a number")
		}
		if parsed < 0 {
			log.Fatal("invalid CHECK_INTERVAL_MINUTES: must greater or equal zero")
		}
		interval = parsed
	}

	return Config{
		RedisURL:             redisURL,
		Port:                 port,
		CheckIntervalMinutes: interval,
	}
}
