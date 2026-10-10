package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type KafkaConfig struct {
	Brokers                []string
	RateUpdatesTopic       string
	DialTimeout            time.Duration
	RequestTimeoutOverhead time.Duration
}

type GRPCConfig struct {
	Addr            string
	ShutdownTimeout time.Duration
}

type Config struct {
	Kafka       KafkaConfig
	GRPC        GRPCConfig
	RateUpdate  time.Duration
	RatePublish time.Duration
	BaseRates   map[string]decimal.Decimal
	JitterMin   decimal.Decimal
	JitterMax   decimal.Decimal
	RateScale   int32
}

func Load() (Config, error) {
	var cfg Config
	var err error
	brokers, err := required("KAFKA_BROKERS")
	if err != nil {
		return Config{}, err
	}
	cfg.Kafka.Brokers = split(brokers)
	if cfg.Kafka.RateUpdatesTopic, err = required("KAFKA_RATE_UPDATES_TOPIC"); err != nil {
		return Config{}, err
	}
	if cfg.Kafka.DialTimeout, err = requiredDuration("KAFKA_DIAL_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.Kafka.RequestTimeoutOverhead, err = requiredDuration("KAFKA_REQUEST_TIMEOUT_OVERHEAD"); err != nil {
		return Config{}, err
	}
	if cfg.GRPC.Addr, err = required("GRPC_ADDR"); err != nil {
		return Config{}, err
	}
	if cfg.GRPC.ShutdownTimeout, err = requiredDuration("GRPC_SHUTDOWN_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.RateUpdate, err = requiredDuration("RATE_UPDATE_INTERVAL"); err != nil {
		return Config{}, err
	}
	if cfg.RatePublish, err = requiredDuration("RATE_PUBLISH_INTERVAL"); err != nil {
		return Config{}, err
	}
	if cfg.BaseRates, err = requiredRates("BASE_RATES"); err != nil {
		return Config{}, err
	}
	if cfg.JitterMin, err = requiredDecimal("RATE_JITTER_MIN"); err != nil {
		return Config{}, err
	}
	if cfg.JitterMax, err = requiredDecimal("RATE_JITTER_MAX"); err != nil {
		return Config{}, err
	}
	if !cfg.JitterMax.GreaterThan(cfg.JitterMin) {
		return Config{}, fmt.Errorf("RATE_JITTER_MAX must be greater than RATE_JITTER_MIN")
	}
	rateScale, err := requiredInt("RATE_SCALE")
	if err != nil {
		return Config{}, err
	}
	cfg.RateScale = int32(rateScale)
	return cfg, nil
}

func required(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}
func requiredDuration(key string) (time.Duration, error) {
	value, err := required(key)
	if err != nil {
		return 0, err
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return parsed, nil
}
func requiredInt(key string) (int, error) {
	value, err := required(key)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", key)
	}
	return parsed, nil
}
func requiredDecimal(key string) (decimal.Decimal, error) {
	value, err := required(key)
	if err != nil {
		return decimal.Zero, err
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero, fmt.Errorf("%s must be a decimal", key)
	}
	return parsed, nil
}
func requiredRates(key string) (map[string]decimal.Decimal, error) {
	value, err := required(key)
	if err != nil {
		return nil, err
	}
	result := make(map[string]decimal.Decimal)
	for _, pair := range split(value) {
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%s entries must use CURRENCY:RATE", key)
		}
		currency := strings.ToUpper(strings.TrimSpace(parts[0]))
		rate, err := decimal.NewFromString(strings.TrimSpace(parts[1]))
		if currency == "" || err != nil || !rate.IsPositive() {
			return nil, fmt.Errorf("invalid %s entry %q", key, pair)
		}
		result[currency] = rate
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%s must contain at least one rate", key)
	}
	return result, nil
}
func split(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
