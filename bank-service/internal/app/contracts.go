package app

//go:generate go run go.uber.org/mock/mockgen -destination mocks_test.go -package app github.com/evgenypakhomov/bank-system-go/bank-service/internal/app Repository,Transaction,RateRepository,RateProvider

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/domain"
)

type AccountRecord struct {
	ID      uuid.UUID
	Balance decimal.Decimal
	OwnerID uuid.UUID
}

type UserFilter struct {
	Login, FirstName, LastName string
	MinAge, MaxAge             *int
	Genders                    []string
	Page, PageSize             int
}

type AccountFilter struct {
	MinBalance, MaxBalance *decimal.Decimal
	OwnerID                *uuid.UUID
	Page, PageSize         int
}

type OperationFilter struct {
	AccountID            *uuid.UUID
	Types                []string
	MinAmount, MaxAmount *decimal.Decimal
	MinDate, MaxDate     *time.Time
	OwnerID              *uuid.UUID
	Page, PageSize       int
}

type TransferFilter struct {
	SenderID, ReceiverID *uuid.UUID
	MinAmount, MaxAmount *decimal.Decimal
	MinDate, MaxDate     *time.Time
	OwnerID              *uuid.UUID
	Page, PageSize       int
}

type Repository interface {
	InTx(context.Context, func(Transaction) error) error
	CreateUser(context.Context, domain.User) error
	UserByID(context.Context, uuid.UUID) (domain.User, error)
	UserByLogin(context.Context, string) (domain.User, error)
	AddFriends(context.Context, uuid.UUID, []uuid.UUID) error
	DeleteFriends(context.Context, uuid.UUID, []uuid.UUID) error
	Friends(context.Context, uuid.UUID) ([]domain.User, error)
	ListUsers(context.Context, UserFilter) (domain.Page[domain.User], error)
	CreateAccount(context.Context, AccountRecord) error
	AccountByID(context.Context, uuid.UUID) (AccountRecord, error)
	UserAccounts(context.Context, uuid.UUID) ([]domain.AccountSummary, error)
	ListAccounts(context.Context, AccountFilter) (domain.Page[domain.AccountSummary], error)
	ListBalanceOperations(context.Context, OperationFilter) (domain.Page[domain.BalanceOperationSummary], error)
	ListTransfers(context.Context, TransferFilter) (domain.Page[domain.TransferOperationSummary], error)
}

type Transaction interface {
	AccountForUpdate(context.Context, uuid.UUID) (AccountRecord, error)
	UpdateAccount(context.Context, AccountRecord) error
	UserByID(context.Context, uuid.UUID) (domain.User, error)
	FriendExists(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	SaveBalanceOperation(context.Context, domain.BalanceOperationSummary) error
	SaveTransfer(context.Context, domain.TransferOperationSummary, decimal.Decimal) error
}

type RateRepository interface {
	Get(context.Context, string) (domain.ExchangeRate, bool, error)
	Save(context.Context, domain.ExchangeRate) error
}

type RateProvider interface {
	GetRate(context.Context, string) (domain.ExchangeRateResponse, error)
}

type UseCases interface {
	Authenticate(context.Context, string, string) (domain.User, error)
	CreateUser(context.Context, CreateUserRequest) (domain.User, error)
	User(context.Context, uuid.UUID) (domain.User, error)
	Users(context.Context, UserFilter) (domain.Page[domain.User], error)
	Friends(context.Context, uuid.UUID) ([]domain.User, error)
	AddFriends(context.Context, uuid.UUID, []uuid.UUID) error
	DeleteFriends(context.Context, uuid.UUID, []uuid.UUID) error
	UserAccounts(context.Context, uuid.UUID) ([]domain.AccountSummary, error)
	CreateAccount(context.Context, uuid.UUID, decimal.Decimal) (domain.AccountDetailed, error)
	Account(context.Context, uuid.UUID, uuid.UUID) (domain.AccountDetailed, error)
	Accounts(context.Context, AccountFilter) (domain.Page[domain.AccountSummary], error)
	Deposit(context.Context, uuid.UUID, uuid.UUID, decimal.Decimal) (domain.BalanceOperationDetailed, error)
	Withdraw(context.Context, uuid.UUID, uuid.UUID, decimal.Decimal) (domain.BalanceOperationDetailed, error)
	Transfer(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, decimal.Decimal) (domain.TransferOperationDetailed, error)
	BalanceOperations(context.Context, OperationFilter) (domain.Page[domain.BalanceOperationSummary], error)
	Transfers(context.Context, TransferFilter) (domain.Page[domain.TransferOperationSummary], error)
	BalanceInCurrency(context.Context, uuid.UUID, uuid.UUID, string) (decimal.Decimal, error)
}
