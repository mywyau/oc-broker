package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr           string
	DatabaseURL        string
	AWSRegion          string
	AWSEndpointURL     string
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	CommandsQueueURL   string
	EventsQueueURL     string
	WorkerConcurrency  int
	OutboxPollInterval time.Duration
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr:           env("HTTP_ADDR", ":8080"),
		DatabaseURL:        env("DATABASE_URL", "postgres://oc:oc@localhost:5432/oc_broker?sslmode=disable"),
		AWSRegion:          env("AWS_REGION", "eu-west-2"),
		AWSEndpointURL:     optionalEnv("AWS_ENDPOINT_URL", "http://localhost:4566"),
		AWSAccessKeyID:     env("AWS_ACCESS_KEY_ID", "test"),
		AWSSecretAccessKey: env("AWS_SECRET_ACCESS_KEY", "test"),
		CommandsQueueURL:   env("CLAIM_COMMANDS_QUEUE_URL", "http://localhost:4566/queue/eu-west-2/000000000000/oc-claim-commands"),
		EventsQueueURL:     env("CLAIM_EVENTS_QUEUE_URL", "http://localhost:4566/queue/eu-west-2/000000000000/oc-claim-events"),
	}

	var err error
	c.WorkerConcurrency, err = strconv.Atoi(env("WORKER_CONCURRENCY", "4"))
	if err != nil || c.WorkerConcurrency < 1 || c.WorkerConcurrency > 100 {
		return Config{}, fmt.Errorf("WORKER_CONCURRENCY must be between 1 and 100")
	}
	c.OutboxPollInterval, err = time.ParseDuration(env("OUTBOX_POLL_INTERVAL", "1s"))
	if err != nil || c.OutboxPollInterval <= 0 {
		return Config{}, fmt.Errorf("OUTBOX_POLL_INTERVAL must be a positive duration")
	}
	return c, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func optionalEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
