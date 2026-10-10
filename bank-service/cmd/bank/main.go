package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/shopspring/decimal"

	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/app"
	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/config"
	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/domain"
	ratesgrpc "github.com/evgenypakhomov/bank-system-go/bank-service/internal/infrastructure/grpc/rates"
	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/infrastructure/memory"
	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/infrastructure/postgres"
	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/security"
	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/transport/httpapi"
	kafkatransport "github.com/evgenypakhomov/bank-system-go/bank-service/internal/transport/kafka"
)

func main() {
	decimal.MarshalJSONWithoutQuotes = true
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	migrationCtx, cancelMigration := context.WithTimeout(ctx, cfg.Database.MigrationTimeout)
	err = migrate(migrationCtx, cfg.Database.URL, cfg.Database.MigrationsPath)
	cancelMigration()
	if err != nil {
		slog.Error("run migrations", "error", err)
		os.Exit(1)
	}

	databaseCtx, cancelDatabase := context.WithTimeout(ctx, cfg.Database.ConnectTimeout)
	repository, err := postgres.Open(databaseCtx, cfg.Database.URL, cfg.Policy.MoneyScale)
	cancelDatabase()
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer repository.Close()

	rateClient, err := ratesgrpc.NewClient(ratesgrpc.Config{
		Target: cfg.GRPC.RatesTarget, RequestTimeout: cfg.GRPC.RequestTimeout,
	})
	if err != nil {
		slog.Error("create rates gRPC client", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := rateClient.Close(); err != nil {
			slog.Error("close rates gRPC client", "error", err)
		}
	}()

	rateRepository := memory.NewRateRepository()
	service := app.NewService(
		repository,
		rateRepository,
		rateClient,
		cfg.RateTTL,
		app.CommissionConfig{Own: cfg.Commission.Own, Friend: cfg.Commission.Friend, Default: cfg.Commission.Default},
		cfg.PasswordHashCost,
		app.PolicyConfig{MinimumUserAge: cfg.Policy.MinimumUserAge, MaximumUserAge: cfg.Policy.MaximumUserAge, MoneyScale: cfg.Policy.MoneyScale},
	)
	if err := bootstrapAdmin(ctx, cfg.BootstrapAdmin, repository, service); err != nil {
		slog.Error("bootstrap admin", "error", err)
		os.Exit(1)
	}

	kafkaConfig := kafkatransport.Config{
		Brokers: cfg.Kafka.Brokers, RateUpdatesTopic: cfg.Kafka.RateUpdatesTopic,
		UpdatesConsumerGroup: cfg.Kafka.UpdatesConsumerGroup, DialTimeout: cfg.Kafka.DialTimeout,
		RequestTimeoutOverhead: cfg.Kafka.RequestTimeoutOverhead,
	}
	go func() {
		if err := kafkatransport.ConsumeRateUpdates(ctx, kafkaConfig, service); err != nil {
			slog.Error("consume rate updates", "error", err)
		}
	}()

	server := &http.Server{
		Addr: cfg.HTTP.Addr,
		Handler: httpapi.New(service, security.NewJWT(cfg.JWTSecret, cfg.JWTExpiration), httpapi.Config{
			HandlerTimeout: cfg.HTTP.HandlerTimeout, RequestBodyLimit: cfg.HTTP.RequestBodyLimit,
			DefaultPageSize: cfg.Pagination.DefaultPageSize, MaxPageSize: cfg.Pagination.MaxPageSize,
		}),
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}
	go func() {
		slog.Info("bank service started", "address", cfg.HTTP.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown HTTP server", "error", err)
	}
}

func migrate(ctx context.Context, databaseURL, path string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, path)
}

func bootstrapAdmin(ctx context.Context, cfg config.BootstrapAdminConfig, repository app.Repository, service *app.Service) error {
	if !cfg.Enabled {
		return nil
	}
	if _, err := repository.UserByLogin(ctx, cfg.Login); err == nil {
		return nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("find bootstrap admin: %w", err)
	}
	_, err := service.CreateUser(ctx, app.CreateUserRequest{
		Login: cfg.Login, Password: cfg.Password, FirstName: cfg.FirstName, LastName: cfg.LastName,
		Age: cfg.Age, Gender: domain.Gender(cfg.Gender), Role: domain.RoleAdmin,
	})
	return err
}
