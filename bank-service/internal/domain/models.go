package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Role string

const (
	RoleAdmin Role = "ADMIN"
	RoleUser  Role = "USER"
)

type Gender string

const (
	GenderMale   Gender = "MALE"
	GenderFemale Gender = "FEMALE"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Login        string    `json:"login"`
	FirstName    string    `json:"firstName"`
	LastName     string    `json:"lastName"`
	Age          int       `json:"age"`
	Gender       Gender    `json:"gender"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"-"`
}

type AccountSummary struct {
	ID      uuid.UUID       `json:"id"`
	Balance decimal.Decimal `json:"balance"`
	OwnerID uuid.UUID       `json:"ownerId"`
}

type AccountDetailed struct {
	ID      uuid.UUID       `json:"id"`
	Balance decimal.Decimal `json:"balance"`
	Owner   User            `json:"owner"`
}

type BalanceOperationSummary struct {
	ID           uuid.UUID       `json:"id"`
	AccountID    uuid.UUID       `json:"accountId"`
	Amount       decimal.Decimal `json:"amount"`
	BalanceAfter decimal.Decimal `json:"balanceAfter"`
	Type         string          `json:"type"`
	CreatedAt    time.Time       `json:"createdAt"`
}

type BalanceOperationDetailed struct {
	ID           uuid.UUID       `json:"id"`
	Account      AccountDetailed `json:"account"`
	Amount       decimal.Decimal `json:"amount"`
	BalanceAfter decimal.Decimal `json:"balanceAfter"`
	Type         string          `json:"type"`
	CreatedAt    time.Time       `json:"createdAt"`
}

type TransferOperationSummary struct {
	ID               uuid.UUID       `json:"id"`
	AccountFromID    uuid.UUID       `json:"accountFromId"`
	Amount           decimal.Decimal `json:"amount"`
	BalanceAfter     decimal.Decimal `json:"balanceAfter"`
	CommissionAmount decimal.Decimal `json:"commissionAmount"`
	AccountToID      uuid.UUID       `json:"accountToId"`
	CreatedAt        time.Time       `json:"createdAt"`
}

type TransferOperationDetailed struct {
	ID                      uuid.UUID       `json:"id"`
	AccountDetailedResponse AccountDetailed `json:"accountDetailedWebResponse"`
	Amount                  decimal.Decimal `json:"amount"`
	BalanceAfter            decimal.Decimal `json:"balanceAfter"`
	CommissionAmount        decimal.Decimal `json:"commissionAmount"`
	RelatedAccountDTO       AccountDetailed `json:"relatedAccountDto"`
	CreatedAt               time.Time       `json:"createdAt"`
}

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

type Page[T any] struct {
	Items         []T   `json:"items"`
	Page          int   `json:"page"`
	PageSize      int   `json:"pageSize"`
	TotalElements int64 `json:"totalElements"`
	TotalPages    int   `json:"totalPages"`
}

func NewPage[T any](items []T, page, pageSize int, total int64) Page[T] {
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Page: page, PageSize: pageSize, TotalElements: total, TotalPages: totalPages}
}
