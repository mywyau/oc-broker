package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/oc/oc-broker/internal/config"
	"github.com/oc/oc-broker/internal/event"
	"github.com/oc/oc-broker/internal/messaging"
	"github.com/oc/oc-broker/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("open store", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	client, err := messaging.NewSQS(ctx, cfg)
	if err != nil {
		logger.Error("create sqs client", "error", err)
		os.Exit(1)
	}
	ticker := time.NewTicker(cfg.OutboxPollInterval)
	defer ticker.Stop()
	logger.Info("outbox publisher started")
	for {
		publishBatch(ctx, logger, db, client, cfg)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func publishBatch(ctx context.Context, logger *slog.Logger, db *store.Store, client *sqs.Client, cfg config.Config) {
	events, err := db.PendingOutbox(ctx, 50)
	if err != nil {
		logger.Error("read outbox", "error", err)
		return
	}
	for _, e := range events {
		envelope := event.Envelope{EventID: e.EventID, EventType: e.EventType, EventVersion: e.EventVersion, OccurredAt: e.OccurredAt, CorrelationID: e.CorrelationID, Payload: e.Payload}
		body, _ := json.Marshal(envelope)
		queueURL := cfg.CommandsQueueURL
		if e.TargetQueue == "claim-events" {
			queueURL = cfg.EventsQueueURL
		}
		_, err := client.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(queueURL), MessageBody: aws.String(string(body))})
		if err != nil {
			db.MarkPublishFailed(ctx, e.EventID, err)
			logger.Error("publish event", "event_id", e.EventID, "error", err)
			continue
		}
		if err := db.MarkPublished(ctx, e.EventID); err != nil {
			logger.Error("mark event published", "event_id", e.EventID, "error", err)
			continue
		}
		logger.Info("event published", "event_id", e.EventID, "event_type", e.EventType)
	}
}
