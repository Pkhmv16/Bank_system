package app

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/evgenypakhomov/bank-system-go/rates-service/internal/domain"
)

type Service struct {
	repository RateRepository
	base       map[string]decimal.Decimal
	update     time.Duration
	publish    time.Duration
	publisher  Publisher
	jitterMin  decimal.Decimal
	jitterMax  decimal.Decimal
	rateScale  int32
}

func NewService(ctx context.Context, repository RateRepository, publisher Publisher, update, publish time.Duration, baseRates map[string]decimal.Decimal, jitterMin, jitterMax decimal.Decimal, rateScale int32) (*Service, error) {
	service := &Service{repository: repository, base: baseRates, update: update, publish: publish, publisher: publisher, jitterMin: jitterMin, jitterMax: jitterMax, rateScale: rateScale}
	if err := service.refresh(ctx); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) Run(ctx context.Context) error {
	updateTicker := time.NewTicker(s.update)
	publishTicker := time.NewTicker(s.publish)
	defer updateTicker.Stop()
	defer publishTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-updateTicker.C:
			if err := s.refresh(ctx); err != nil {
				return err
			}
		case <-publishTicker.C:
			if err := s.publishRates(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *Service) Get(ctx context.Context, currency string) (domain.ExchangeRateResponse, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	rate, ok, err := s.repository.Get(ctx, currency)
	if err != nil {
		return domain.ExchangeRateResponse{}, fmt.Errorf("get %s rate: %w", currency, err)
	}
	if !ok {
		message := "Currency not found: " + currency
		return domain.ExchangeRateResponse{Success: false, ErrorMessage: &message, Currency: currency}, nil
	}
	return domain.ExchangeRateResponse{Success: true, Currency: currency, RateToRUB: &rate.RateToRUB, Timestamp: &rate.Timestamp}, nil
}

func (s *Service) refresh(ctx context.Context) error {
	now := time.Now().UTC()
	rates := make([]domain.ExchangeRate, 0, len(s.base))
	for currency, base := range s.base {
		spread := s.jitterMax.Sub(s.jitterMin)
		coefficient := s.jitterMin.Add(spread.Mul(decimal.NewFromFloat(rand.Float64())))
		rates = append(rates, domain.ExchangeRate{Currency: currency, RateToRUB: base.Mul(coefficient).Round(s.rateScale), Timestamp: now})
	}
	if err := s.repository.ReplaceAll(ctx, rates); err != nil {
		return fmt.Errorf("replace exchange rates: %w", err)
	}
	return nil
}

func (s *Service) publishRates(ctx context.Context) error {
	rates, err := s.repository.List(ctx)
	if err != nil {
		return fmt.Errorf("list exchange rates: %w", err)
	}
	for _, rate := range rates {
		if err := s.publisher.Publish(ctx, rate); err != nil {
			return fmt.Errorf("publish %s rate: %w", rate.Currency, err)
		}
	}
	return nil
}
