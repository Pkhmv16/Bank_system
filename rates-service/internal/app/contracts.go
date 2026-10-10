package app

//go:generate go run go.uber.org/mock/mockgen -destination mocks_test.go -package app github.com/evgenypakhomov/bank-system-go/rates-service/internal/app RateRepository,Publisher

import (
	"context"

	"github.com/evgenypakhomov/bank-system-go/rates-service/internal/domain"
)

type RateRepository interface {
	Get(context.Context, string) (domain.ExchangeRate, bool, error)
	ReplaceAll(context.Context, []domain.ExchangeRate) error
	List(context.Context) ([]domain.ExchangeRate, error)
}

type Publisher interface {
	Publish(context.Context, domain.ExchangeRate) error
}
