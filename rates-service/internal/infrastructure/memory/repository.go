package memory

import (
	"context"
	"strings"
	"sync"

	"github.com/evgenypakhomov/bank-system-go/rates-service/internal/domain"
)

type RateRepository struct {
	mu    sync.RWMutex
	rates map[string]domain.ExchangeRate
}

func NewRateRepository() *RateRepository {
	return &RateRepository{rates: make(map[string]domain.ExchangeRate)}
}

func (r *RateRepository) Get(ctx context.Context, currency string) (domain.ExchangeRate, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.ExchangeRate{}, false, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	rate, ok := r.rates[strings.ToUpper(strings.TrimSpace(currency))]
	return rate, ok, nil
}

func (r *RateRepository) ReplaceAll(ctx context.Context, rates []domain.ExchangeRate) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	next := make(map[string]domain.ExchangeRate, len(rates))
	for _, rate := range rates {
		next[strings.ToUpper(strings.TrimSpace(rate.Currency))] = rate
	}
	r.mu.Lock()
	r.rates = next
	r.mu.Unlock()
	return nil
}

func (r *RateRepository) List(ctx context.Context) ([]domain.ExchangeRate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	rates := make([]domain.ExchangeRate, 0, len(r.rates))
	for _, rate := range r.rates {
		rates = append(rates, rate)
	}
	return rates, nil
}
