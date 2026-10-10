package memory

import (
	"context"
	"strings"
	"sync"

	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/domain"
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

func (r *RateRepository) Save(ctx context.Context, rate domain.ExchangeRate) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rate.Currency = strings.ToUpper(strings.TrimSpace(rate.Currency))
	r.mu.Lock()
	r.rates[rate.Currency] = rate
	r.mu.Unlock()
	return nil
}
