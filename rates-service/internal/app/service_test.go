package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/mock/gomock"

	"github.com/evgenypakhomov/bank-system-go/rates-service/internal/domain"
)

func TestNewServiceSeedsSupportedRates(t *testing.T) {
	controller := gomock.NewController(t)
	repository := NewMockRateRepository(controller)
	publisher := NewMockPublisher(controller)

	repository.EXPECT().ReplaceAll(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, rates []domain.ExchangeRate) error {
			if len(rates) != 2 {
				t.Fatalf("expected two rates, got %d", len(rates))
			}
			found := make(map[string]bool, len(rates))
			for _, rate := range rates {
				found[rate.Currency] = rate.RateToRUB.IsPositive()
			}
			if !found["USD"] || !found["EUR"] {
				t.Fatalf("invalid generated rates: %+v", rates)
			}
			return nil
		},
	)

	if _, err := newTestService(context.Background(), repository, publisher); err != nil {
		t.Fatalf("new service: %v", err)
	}
}

func TestGetRate(t *testing.T) {
	controller := gomock.NewController(t)
	repository := NewMockRateRepository(controller)
	publisher := NewMockPublisher(controller)
	now := time.Now().UTC()
	rate := domain.ExchangeRate{Currency: "USD", RateToRUB: decimal.NewFromInt(75), Timestamp: now}

	repository.EXPECT().ReplaceAll(gomock.Any(), gomock.Any()).Return(nil)
	service, err := newTestService(context.Background(), repository, publisher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	repository.EXPECT().Get(gomock.Any(), "USD").Return(rate, true, nil)

	response, err := service.Get(context.Background(), " usd ")
	if err != nil {
		t.Fatalf("get rate: %v", err)
	}
	if !response.Success || response.RateToRUB == nil || !response.RateToRUB.Equal(rate.RateToRUB) {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestUnknownRate(t *testing.T) {
	controller := gomock.NewController(t)
	repository := NewMockRateRepository(controller)
	publisher := NewMockPublisher(controller)

	repository.EXPECT().ReplaceAll(gomock.Any(), gomock.Any()).Return(nil)
	service, err := newTestService(context.Background(), repository, publisher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	repository.EXPECT().Get(gomock.Any(), "GBP").Return(domain.ExchangeRate{}, false, nil)

	response, err := service.Get(context.Background(), "GBP")
	if err != nil {
		t.Fatalf("get rate: %v", err)
	}
	if response.Success || response.ErrorMessage == nil {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestGetRateReturnsRepositoryError(t *testing.T) {
	controller := gomock.NewController(t)
	repository := NewMockRateRepository(controller)
	publisher := NewMockPublisher(controller)

	repository.EXPECT().ReplaceAll(gomock.Any(), gomock.Any()).Return(nil)
	service, err := newTestService(context.Background(), repository, publisher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	repositoryError := errors.New("repository unavailable")
	repository.EXPECT().Get(gomock.Any(), "USD").Return(domain.ExchangeRate{}, false, repositoryError)

	if _, err := service.Get(context.Background(), "USD"); !errors.Is(err, repositoryError) {
		t.Fatalf("expected repository error, got %v", err)
	}
}

func TestPublishRates(t *testing.T) {
	controller := gomock.NewController(t)
	repository := NewMockRateRepository(controller)
	publisher := NewMockPublisher(controller)
	rates := []domain.ExchangeRate{
		{Currency: "USD", RateToRUB: decimal.NewFromInt(75)},
		{Currency: "EUR", RateToRUB: decimal.NewFromInt(85)},
	}

	repository.EXPECT().ReplaceAll(gomock.Any(), gomock.Any()).Return(nil)
	service, err := newTestService(context.Background(), repository, publisher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	repository.EXPECT().List(gomock.Any()).Return(rates, nil)
	for _, rate := range rates {
		publisher.EXPECT().Publish(gomock.Any(), rate).Return(nil)
	}

	if err := service.publishRates(context.Background()); err != nil {
		t.Fatalf("publish rates: %v", err)
	}
}

func newTestService(ctx context.Context, repository RateRepository, publisher Publisher) (*Service, error) {
	return NewService(
		ctx,
		repository,
		publisher,
		time.Second,
		time.Minute,
		map[string]decimal.Decimal{"USD": decimal.NewFromInt(70), "EUR": decimal.NewFromInt(80)},
		decimal.RequireFromString("0.95"),
		decimal.RequireFromString("1.05"),
		2,
	)
}
