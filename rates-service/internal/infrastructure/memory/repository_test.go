package memory

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/evgenypakhomov/bank-system-go/rates-service/internal/domain"
)

func TestRateRepositoryReplaceGetAndList(t *testing.T) {
	repository := NewRateRepository()
	rates := []domain.ExchangeRate{
		{Currency: "USD", RateToRUB: decimal.NewFromInt(75), Timestamp: time.Now().UTC()},
		{Currency: "EUR", RateToRUB: decimal.NewFromInt(85), Timestamp: time.Now().UTC()},
	}

	if err := repository.ReplaceAll(context.Background(), rates); err != nil {
		t.Fatalf("replace rates: %v", err)
	}
	rate, ok, err := repository.Get(context.Background(), " usd ")
	if err != nil {
		t.Fatalf("get rate: %v", err)
	}
	if !ok || !rate.RateToRUB.Equal(decimal.NewFromInt(75)) {
		t.Fatalf("unexpected rate: %+v, found: %t", rate, ok)
	}
	stored, err := repository.List(context.Background())
	if err != nil {
		t.Fatalf("list rates: %v", err)
	}
	if len(stored) != len(rates) {
		t.Fatalf("expected %d rates, got %d", len(rates), len(stored))
	}
}

func TestRateRepositoryReplaceRemovesOldRates(t *testing.T) {
	repository := NewRateRepository()
	if err := repository.ReplaceAll(context.Background(), []domain.ExchangeRate{{Currency: "USD"}}); err != nil {
		t.Fatalf("seed repository: %v", err)
	}
	if err := repository.ReplaceAll(context.Background(), []domain.ExchangeRate{{Currency: "EUR"}}); err != nil {
		t.Fatalf("replace rates: %v", err)
	}
	if _, ok, err := repository.Get(context.Background(), "USD"); err != nil || ok {
		t.Fatalf("old rate was not removed: found=%t, error=%v", ok, err)
	}
}
