package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

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

	jobs := make(chan types.Message, cfg.WorkerConcurrency*2)
	var wg sync.WaitGroup
	for i := 0; i < cfg.WorkerConcurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for message := range jobs {
				handleMessage(ctx, logger.With("worker_id", workerID), db, client, cfg.CommandsQueueURL, message)
			}
		}(i + 1)
	}
	logger.Info("claim worker started", "concurrency", cfg.WorkerConcurrency)
	poll(ctx, logger, client, cfg.CommandsQueueURL, jobs)
	close(jobs)
	wg.Wait()
}

func poll(ctx context.Context, logger *slog.Logger, client *sqs.Client, queueURL string, jobs chan<- types.Message) {
	for ctx.Err() == nil {
		result, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{QueueUrl: aws.String(queueURL), MaxNumberOfMessages: 10, WaitTimeSeconds: 20, VisibilityTimeout: 30})
		if err != nil {
			if ctx.Err() == nil {
				logger.Error("receive messages", "error", err)
			}
			continue
		}
		for _, message := range result.Messages {
			select {
			case jobs <- message:
			case <-ctx.Done():
				return
			}
		}
	}
}

func handleMessage(ctx context.Context, logger *slog.Logger, db *store.Store, client *sqs.Client, queueURL string, message types.Message) {
	if message.Body == nil || message.ReceiptHandle == nil {
		logger.Error("invalid SQS message")
		return
	}
	var envelope event.Envelope
	if err := json.Unmarshal([]byte(*message.Body), &envelope); err != nil {
		logger.Error("decode envelope; message will be retried", "message_id", aws.ToString(message.MessageId), "error", err)
		return
	}
	if envelope.EventID.String() == "00000000-0000-0000-0000-000000000000" || envelope.EventType != event.ClaimSubmitted {
		logger.Error("unsupported envelope; message will be retried", "message_id", aws.ToString(message.MessageId), "event_type", envelope.EventType)
		return
	}
	if err := db.ProcessSubmittedClaim(ctx, envelope); err != nil {
		logger.Error("process claim; message will be retried", "event_id", envelope.EventID, "error", fmt.Errorf("process: %w", err))
		return
	}
	_, err := client.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: aws.String(queueURL), ReceiptHandle: message.ReceiptHandle})
	if err != nil {
		logger.Error("delete processed message", "event_id", envelope.EventID, "error", err)
		return
	}
	logger.Info("claim processed", "event_id", envelope.EventID)
}
