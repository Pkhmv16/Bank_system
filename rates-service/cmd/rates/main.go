package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc"

	"github.com/evgenypakhomov/bank-system-go/rates-service/internal/app"
	"github.com/evgenypakhomov/bank-system-go/rates-service/internal/config"
	kafkainfra "github.com/evgenypakhomov/bank-system-go/rates-service/internal/infrastructure/kafka"
	"github.com/evgenypakhomov/bank-system-go/rates-service/internal/infrastructure/memory"
	grpctransport "github.com/evgenypakhomov/bank-system-go/rates-service/internal/transport/grpcapi"
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

	publisher, err := kafkainfra.NewPublisher(kafkainfra.Config{
		Brokers: cfg.Kafka.Brokers, RateUpdatesTopic: cfg.Kafka.RateUpdatesTopic,
		DialTimeout: cfg.Kafka.DialTimeout, RequestTimeoutOverhead: cfg.Kafka.RequestTimeoutOverhead,
	})
	if err != nil {
		slog.Error("create Kafka publisher", "error", err)
		os.Exit(1)
	}
	defer publisher.Close()

	repository := memory.NewRateRepository()
	service, err := app.NewService(ctx, repository, publisher, cfg.RateUpdate, cfg.RatePublish, cfg.BaseRates, cfg.JitterMin, cfg.JitterMax, cfg.RateScale)
	if err != nil {
		slog.Error("initialize rates service", "error", err)
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", cfg.GRPC.Addr)
	if err != nil {
		slog.Error("listen for gRPC", "error", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer()
	grpctransport.Register(grpcServer, service)

	go func() {
		if err := service.Run(ctx); err != nil {
			slog.Error("run rates service", "error", err)
			stop()
		}
	}()
	go func() {
		slog.Info("rates service started", "grpc_address", cfg.GRPC.Addr, "kafka_brokers", cfg.Kafka.Brokers)
		if err := grpcServer.Serve(listener); err != nil {
			slog.Error("serve gRPC", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	gracefulStop(grpcServer, cfg.GRPC.ShutdownTimeout)
}

func gracefulStop(server *grpc.Server, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		server.Stop()
	}
}
