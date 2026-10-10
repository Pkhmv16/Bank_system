package rates

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/domain"
	ratesv1 "github.com/evgenypakhomov/bank-system-go/bank-service/internal/gen/rates/v1"
)

type Config struct {
	Target         string
	RequestTimeout time.Duration
}

type Client struct {
	connection     *grpc.ClientConn
	rates          ratesv1.RateServiceClient
	requestTimeout time.Duration
}

func NewClient(config Config) (*Client, error) {
	connection, err := grpc.NewClient(config.Target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("create rates gRPC client: %w", err)
	}
	return &Client{
		connection:     connection,
		rates:          ratesv1.NewRateServiceClient(connection),
		requestTimeout: config.RequestTimeout,
	}, nil
}

func (c *Client) Close() error { return c.connection.Close() }

func (c *Client) GetRate(ctx context.Context, currency string) (domain.ExchangeRateResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	response, err := c.rates.GetRate(requestCtx, &ratesv1.GetRateRequest{Currency: currency})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded {
			return domain.ExchangeRateResponse{}, domain.NewError(domain.ErrExternalTimeout, "rate service did not respond in time")
		}
		return domain.ExchangeRateResponse{}, fmt.Errorf("request exchange rate over gRPC: %w", err)
	}
	if !response.GetSuccess() {
		message := response.GetErrorMessage()
		return domain.ExchangeRateResponse{Success: false, Currency: response.GetCurrency(), ErrorMessage: &message}, nil
	}
	rate, err := decimal.NewFromString(response.GetRateToRub())
	if err != nil {
		return domain.ExchangeRateResponse{}, fmt.Errorf("parse rate from gRPC response: %w", err)
	}
	if response.GetTimestamp() == nil || !response.GetTimestamp().IsValid() {
		return domain.ExchangeRateResponse{}, errors.New("gRPC rate response contains invalid timestamp")
	}
	return domain.ExchangeRateResponse{
		Success: true, Currency: response.GetCurrency(), RateToRUB: &rate, Timestamp: new(response.GetTimestamp().AsTime()),
	}, nil
}
