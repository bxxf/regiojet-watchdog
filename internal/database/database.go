package database

import (
	"github.com/bxxf/regiojet-watchdog/internal/config"
	"github.com/go-redis/redis/v8"
)

type DatabaseClient struct {
	RedisClient *redis.Client
}

func NewDatabaseClient(config config.Config) *DatabaseClient {
	opt, err := redis.ParseURL(config.RedisURL)
	if err != nil {
		panic(err)
	}

	client := redis.NewClient(opt)
	_, err = client.Ping(client.Context()).Result()
	if err != nil {
		panic(err)
	}
	return &DatabaseClient{
		RedisClient: client,
	}
}
