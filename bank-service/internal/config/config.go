package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type HTTPConfig struct {
	Addr              string
	HandlerTimeout    time.Duration
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	RequestBodyLimit  int64
}

type DatabaseConfig struct {
	URL              string
	ConnectTimeout   time.Duration
	MigrationTimeout time.Duration
	MigrationsPath   string
}

type KafkaConfig struct {
	Brokers                []string
	RateUpdatesTopic       string
	UpdatesConsumerGroup   string
	DialTimeout            time.Duration
	RequestTimeoutOverhead time.Duration
}

type GRPCConfig struct {
	RatesTarget    string
	RequestTimeout time.Duration
}

type PaginationConfig struct {
	DefaultPageSize int
	MaxPageSize     int
}

type CommissionConfig struct {
	Own     decimal.Decimal
	Friend  decimal.Decimal
	Default decimal.Decimal
}

type PolicyConfig struct {
	MinimumUserAge int
	MaximumUserAge int
	MoneyScale     int32
}

type BootstrapAdminConfig struct {
	Enabled   bool
	Login     string
	Password  string
	FirstName string
	LastName  string
	Age       int
	Gender    string
}

type Config struct {
	HTTP             HTTPConfig
	Database         DatabaseConfig
	Kafka            KafkaConfig
	GRPC             GRPCConfig
	JWTSecret        string
	JWTExpiration    time.Duration
	RateTTL          time.Duration
	Pagination       PaginationConfig
	Commission       CommissionConfig
	Policy           PolicyConfig
	PasswordHashCost int
	BootstrapAdmin   BootstrapAdminConfig
}

func Load() (Config, error) {
	var cfg Config
	var err error

	if cfg.HTTP.Addr, err = required("HTTP_ADDR"); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.HandlerTimeout, err = requiredDuration("HTTP_HANDLER_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.ReadHeaderTimeout, err = requiredDuration("HTTP_READ_HEADER_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.ReadTimeout, err = requiredDuration("HTTP_READ_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.WriteTimeout, err = requiredDuration("HTTP_WRITE_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.IdleTimeout, err = requiredDuration("HTTP_IDLE_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.ShutdownTimeout, err = requiredDuration("HTTP_SHUTDOWN_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.RequestBodyLimit, err = requiredInt64("HTTP_REQUEST_BODY_LIMIT"); err != nil {
		return Config{}, err
	}

	if cfg.Database.URL, err = required("DATABASE_URL"); err != nil {
		return Config{}, err
	}
	if cfg.Database.ConnectTimeout, err = requiredDuration("DATABASE_CONNECT_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.Database.MigrationTimeout, err = requiredDuration("DATABASE_MIGRATION_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.Database.MigrationsPath, err = required("MIGRATIONS_PATH"); err != nil {
		return Config{}, err
	}

	brokers, err := required("KAFKA_BROKERS")
	if err != nil {
		return Config{}, err
	}
	cfg.Kafka.Brokers = split(brokers)
	if len(cfg.Kafka.Brokers) == 0 {
		return Config{}, fmt.Errorf("KAFKA_BROKERS must not be empty")
	}
	if cfg.Kafka.RateUpdatesTopic, err = required("KAFKA_RATE_UPDATES_TOPIC"); err != nil {
		return Config{}, err
	}
	if cfg.Kafka.UpdatesConsumerGroup, err = required("KAFKA_UPDATES_CONSUMER_GROUP"); err != nil {
		return Config{}, err
	}
	if cfg.Kafka.DialTimeout, err = requiredDuration("KAFKA_DIAL_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if cfg.Kafka.RequestTimeoutOverhead, err = requiredDuration("KAFKA_REQUEST_TIMEOUT_OVERHEAD"); err != nil {
		return Config{}, err
	}
	if cfg.GRPC.RatesTarget, err = required("GRPC_RATES_TARGET"); err != nil {
		return Config{}, err
	}
	if cfg.GRPC.RequestTimeout, err = requiredDuration("GRPC_REQUEST_TIMEOUT"); err != nil {
		return Config{}, err
	}

	if cfg.JWTSecret, err = required("JWT_SECRET"); err != nil {
		return Config{}, err
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must contain at least 32 characters")
	}
	if cfg.JWTExpiration, err = requiredDuration("JWT_EXPIRATION"); err != nil {
		return Config{}, err
	}
	if cfg.RateTTL, err = requiredDuration("RATE_TTL"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.DefaultPageSize, err = requiredInt("PAGINATION_DEFAULT_PAGE_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.MaxPageSize, err = requiredInt("PAGINATION_MAX_PAGE_SIZE"); err != nil {
		return Config{}, err
	}
	if cfg.Pagination.DefaultPageSize > cfg.Pagination.MaxPageSize {
		return Config{}, fmt.Errorf("default page size must not exceed maximum page size")
	}
	if cfg.PasswordHashCost, err = requiredInt("PASSWORD_HASH_COST"); err != nil {
		return Config{}, err
	}
	if cfg.Policy.MinimumUserAge, err = requiredNonNegativeInt("USER_MIN_AGE"); err != nil {
		return Config{}, err
	}
	if cfg.Policy.MaximumUserAge, err = requiredInt("USER_MAX_AGE"); err != nil {
		return Config{}, err
	}
	if cfg.Policy.MaximumUserAge < cfg.Policy.MinimumUserAge {
		return Config{}, fmt.Errorf("USER_MAX_AGE must not be less than USER_MIN_AGE")
	}
	moneyScale, err := requiredNonNegativeInt("MONEY_SCALE")
	if err != nil {
		return Config{}, err
	}
	cfg.Policy.MoneyScale = int32(moneyScale)

	if cfg.Commission.Own, err = requiredRate("TRANSFER_COMMISSION_OWN"); err != nil {
		return Config{}, err
	}
	if cfg.Commission.Friend, err = requiredRate("TRANSFER_COMMISSION_FRIEND"); err != nil {
		return Config{}, err
	}
	if cfg.Commission.Default, err = requiredRate("TRANSFER_COMMISSION_DEFAULT"); err != nil {
		return Config{}, err
	}

	if cfg.BootstrapAdmin.Enabled, err = requiredBool("BOOTSTRAP_ADMIN_ENABLED"); err != nil {
		return Config{}, err
	}
	if cfg.BootstrapAdmin.Enabled {
		if cfg.BootstrapAdmin.Login, err = required("BOOTSTRAP_ADMIN_LOGIN"); err != nil {
			return Config{}, err
		}
		if cfg.BootstrapAdmin.Password, err = required("BOOTSTRAP_ADMIN_PASSWORD"); err != nil {
			return Config{}, err
		}
		if cfg.BootstrapAdmin.FirstName, err = required("BOOTSTRAP_ADMIN_FIRST_NAME"); err != nil {
			return Config{}, err
		}
		if cfg.BootstrapAdmin.LastName, err = required("BOOTSTRAP_ADMIN_LAST_NAME"); err != nil {
			return Config{}, err
		}
		if cfg.BootstrapAdmin.Age, err = requiredInt("BOOTSTRAP_ADMIN_AGE"); err != nil {
			return Config{}, err
		}
		if cfg.BootstrapAdmin.Gender, err = required("BOOTSTRAP_ADMIN_GENDER"); err != nil {
			return Config{}, err
		}
	}
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
	if err != nil {
		return 0, fmt.Errorf("parse %s=%q: %w", key, value, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return parsed, nil
}

func requiredInt(key string) (int, error) {
	value, err := required(key)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return parsed, nil
}

func requiredNonNegativeInt(key string) (int, error) {
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

func requiredInt64(key string) (int64, error) {
	value, err := required(key)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return parsed, nil
}

func requiredBool(key string) (bool, error) {
	value, err := required(key)
	if err != nil {
		return false, err
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return parsed, nil
}

func requiredRate(key string) (decimal.Decimal, error) {
	value, err := required(key)
	if err != nil {
		return decimal.Zero, err
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil || parsed.IsNegative() || parsed.GreaterThan(decimal.NewFromInt(1)) {
		return decimal.Zero, fmt.Errorf("%s must be a decimal between 0 and 1", key)
	}
	return parsed, nil
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
