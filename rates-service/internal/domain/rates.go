package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

type ExchangeRate struct {
	Currency  string          `json:"currency"`
	RateToRUB decimal.Decimal `json:"rateToRub"`
	Timestamp time.Time       `json:"timestamp"`
}

type ExchangeRateResponse struct {
	Success      bool             `json:"success"`
	ErrorMessage *string          `json:"errorMessage"`
	Currency     string           `json:"currency"`
	RateToRUB    *decimal.Decimal `json:"rateToRub"`
	Timestamp    *time.Time       `json:"timestamp"`
}
