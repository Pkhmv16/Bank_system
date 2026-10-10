package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/domain"
)

type Config struct {
	Brokers                []string
	RateUpdatesTopic       string
	UpdatesConsumerGroup   string
	DialTimeout            time.Duration
	RequestTimeoutOverhead time.Duration
}

type RateUpdater interface {
	UpdateRate(context.Context, domain.ExchangeRate) error
}

func ConsumeRateUpdates(ctx context.Context, config Config, updater RateUpdater) error {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(config.Brokers...),
		kgo.WithLogger(kgo.BasicLogger(os.Stderr, kgo.LogLevelWarn, nil)),
		kgo.DialTimeout(config.DialTimeout),
		kgo.RequestTimeoutOverhead(config.RequestTimeoutOverhead),
		kgo.ConsumerGroup(config.UpdatesConsumerGroup),
		kgo.ConsumeTopics(config.RateUpdatesTopic),
	)
	if err != nil {
		return fmt.Errorf("create rate update consumer: %w", err)
	}
	defer client.Close()
	for {
		fetches := client.PollFetches(ctx)
		if ctx.Err() != nil {
			return nil
		}
		fetches.EachError(func(topic string, partition int32, err error) {
			slog.Error("Kafka update consumer", "topic", topic, "partition", partition, "error", err)
		})
		var updateErr error
		fetches.EachRecord(func(record *kgo.Record) {
			if updateErr != nil {
				return
			}
			var rate domain.ExchangeRate
			if err := json.Unmarshal(record.Value, &rate); err != nil {
				slog.Error("decode rate update", "error", err)
				return
			}
			if err := updater.UpdateRate(ctx, rate); err != nil {
				updateErr = fmt.Errorf("update %s rate: %w", rate.Currency, err)
			}
		})
		if updateErr != nil {
			return updateErr
		}
	}
}
