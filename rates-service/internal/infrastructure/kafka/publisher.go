package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/evgenypakhomov/bank-system-go/rates-service/internal/domain"
)

type Config struct {
	Brokers                []string
	RateUpdatesTopic       string
	DialTimeout            time.Duration
	RequestTimeoutOverhead time.Duration
}

type Publisher struct {
	client *kgo.Client
	topic  string
}

func NewPublisher(config Config) (*Publisher, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(config.Brokers...),
		kgo.WithLogger(kgo.BasicLogger(os.Stderr, kgo.LogLevelWarn, nil)),
		kgo.DialTimeout(config.DialTimeout),
		kgo.RequestTimeoutOverhead(config.RequestTimeoutOverhead),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka rate publisher: %w", err)
	}
	return &Publisher{client: client, topic: config.RateUpdatesTopic}, nil
}

func (p *Publisher) Close() { p.client.Close() }

func (p *Publisher) Publish(ctx context.Context, rate domain.ExchangeRate) error {
	payload, err := json.Marshal(rate)
	if err != nil {
		return fmt.Errorf("encode rate update: %w", err)
	}
	record := &kgo.Record{Topic: p.topic, Key: []byte(rate.Currency), Value: payload}
	if err := p.client.ProduceSync(ctx, record).FirstErr(); err != nil {
		return fmt.Errorf("publish rate update: %w", err)
	}
	return nil
}
