package memory

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/domain"
)

func TestRateRepositorySaveAndGet(t *testing.T) {
	repository := NewRateRepository()
	want := domain.ExchangeRate{Currency: "usd", RateToRUB: decimal.NewFromInt(75), Timestamp: time.Now().UTC()}

	if err := repository.Save(context.Background(), want); err != nil {
		t.Fatalf("save rate: %v", err)
	}
	got, ok, err := repository.Get(context.Background(), " USD ")
	if err != nil {
		t.Fatalf("get rate: %v", err)
	}
	if !ok || got.Currency != "USD" || !got.RateToRUB.Equal(want.RateToRUB) {
		t.Fatalf("unexpected rate: %+v, found: %t", got, ok)
	}
}

func TestRateRepositoryHonorsCanceledContext(t *testing.T) {
	repository := NewRateRepository()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repository.Save(ctx, domain.ExchangeRate{Currency: "USD"}); err == nil {
		t.Fatal("expected canceled context error")
	}
	if _, _, err := repository.Get(ctx, "USD"); err == nil {
		t.Fatal("expected canceled context error")
	}
}
