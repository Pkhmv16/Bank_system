package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"

	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/domain"
)

const (
	BalanceOperationDeposit  = "DEPOSIT"
	BalanceOperationWithdraw = "WITHDRAW"
)

type CommissionConfig struct {
	Own     decimal.Decimal
	Friend  decimal.Decimal
	Default decimal.Decimal
}

type PolicyConfig struct {
	MinimumUserAge int
	MaximumUserAge int
	MoneyScale     int32
}

type Service struct {
	repository       Repository
	rateRepository   RateRepository
	rateProvider     RateProvider
	rateTTL          time.Duration
	commission       CommissionConfig
	passwordHashCost int
	policy           PolicyConfig
}

var _ UseCases = (*Service)(nil)

func NewService(repository Repository, rateRepository RateRepository, rateProvider RateProvider, rateTTL time.Duration, commission CommissionConfig, passwordHashCost int, policy PolicyConfig) *Service {
	return &Service{repository: repository, rateRepository: rateRepository, rateProvider: rateProvider, rateTTL: rateTTL, commission: commission, passwordHashCost: passwordHashCost, policy: policy}
}

type CreateUserRequest struct {
	Login, FirstName, LastName, Password string
	Age                                  int
	Gender                               domain.Gender
	Role                                 domain.Role
}

func (s *Service) Authenticate(ctx context.Context, login, password string) (domain.User, error) {
	user, err := s.repository.UserByLogin(ctx, login)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return domain.User{}, domain.NewError(domain.ErrInvalidCredentials, "invalid login or password")
	}
	return user, nil
}

func (s *Service) CreateUser(ctx context.Context, request CreateUserRequest) (domain.User, error) {
	if err := s.validateUser(request); err != nil {
		return domain.User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), s.passwordHashCost)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	user := domain.User{
		ID: uuid.New(), Login: strings.TrimSpace(request.Login), FirstName: strings.TrimSpace(request.FirstName),
		LastName: strings.TrimSpace(request.LastName), Age: request.Age, Gender: request.Gender,
		PasswordHash: string(hash), Role: request.Role,
	}
	if err := s.repository.CreateUser(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *Service) validateUser(request CreateUserRequest) error {
	if strings.TrimSpace(request.Login) == "" || strings.TrimSpace(request.FirstName) == "" || strings.TrimSpace(request.LastName) == "" || strings.TrimSpace(request.Password) == "" {
		return domain.NewError(domain.ErrValidation, "login, firstName, lastName and password must not be blank")
	}
	if request.Age < s.policy.MinimumUserAge || request.Age > s.policy.MaximumUserAge {
		return domain.NewError(domain.ErrValidation, fmt.Sprintf("age must be between %d and %d", s.policy.MinimumUserAge, s.policy.MaximumUserAge))
	}
	if request.Gender != domain.GenderMale && request.Gender != domain.GenderFemale {
		return domain.NewError(domain.ErrValidation, "gender must be MALE or FEMALE")
	}
	if request.Role != domain.RoleAdmin && request.Role != domain.RoleUser {
		return domain.NewError(domain.ErrValidation, "role must be ADMIN or USER")
	}
	return nil
}

func (s *Service) User(ctx context.Context, id uuid.UUID) (domain.User, error) {
	return s.repository.UserByID(ctx, id)
}
func (s *Service) Users(ctx context.Context, f UserFilter) (domain.Page[domain.User], error) {
	return s.repository.ListUsers(ctx, f)
}
func (s *Service) Friends(ctx context.Context, id uuid.UUID) ([]domain.User, error) {
	return s.repository.Friends(ctx, id)
}
func (s *Service) AddFriends(ctx context.Context, id uuid.UUID, ids []uuid.UUID) error {
	return s.repository.AddFriends(ctx, id, uniqueIDs(ids))
}
func (s *Service) DeleteFriends(ctx context.Context, id uuid.UUID, ids []uuid.UUID) error {
	return s.repository.DeleteFriends(ctx, id, uniqueIDs(ids))
}
func (s *Service) UserAccounts(ctx context.Context, id uuid.UUID) ([]domain.AccountSummary, error) {
	return s.repository.UserAccounts(ctx, id)
}

func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

func (s *Service) CreateAccount(ctx context.Context, ownerID uuid.UUID, balance decimal.Decimal) (domain.AccountDetailed, error) {
	if balance.IsNegative() {
		return domain.AccountDetailed{}, domain.NewError(domain.ErrValidation, "balance must be greater than or equal to 0")
	}
	owner, err := s.repository.UserByID(ctx, ownerID)
	if err != nil {
		return domain.AccountDetailed{}, err
	}
	account := AccountRecord{ID: uuid.New(), Balance: s.money(balance), OwnerID: ownerID}
	if err := s.repository.CreateAccount(ctx, account); err != nil {
		return domain.AccountDetailed{}, err
	}
	return detailed(account, owner), nil
}

func (s *Service) Account(ctx context.Context, id, currentUserID uuid.UUID) (domain.AccountDetailed, error) {
	account, err := s.repository.AccountByID(ctx, id)
	if err != nil {
		return domain.AccountDetailed{}, err
	}
	if account.OwnerID != currentUserID {
		return domain.AccountDetailed{}, domain.NewError(domain.ErrAccessDenied, "account does not belong to current user")
	}
	owner, err := s.repository.UserByID(ctx, account.OwnerID)
	if err != nil {
		return domain.AccountDetailed{}, err
	}
	return detailed(account, owner), nil
}

func (s *Service) Accounts(ctx context.Context, f AccountFilter) (domain.Page[domain.AccountSummary], error) {
	return s.repository.ListAccounts(ctx, f)
}

func (s *Service) Deposit(ctx context.Context, accountID, currentUserID uuid.UUID, amount decimal.Decimal) (domain.BalanceOperationDetailed, error) {
	if err := validatePositive(amount, "amount"); err != nil {
		return domain.BalanceOperationDetailed{}, err
	}
	var result domain.BalanceOperationDetailed
	err := s.repository.InTx(ctx, func(tx Transaction) error {
		account, err := tx.AccountForUpdate(ctx, accountID)
		if err != nil {
			return err
		}
		if account.OwnerID != currentUserID {
			return domain.NewError(domain.ErrAccessDenied, "account does not belong to current user")
		}
		account.Balance = s.money(account.Balance.Add(amount))
		if err := tx.UpdateAccount(ctx, account); err != nil {
			return err
		}
		now := time.Now().UTC()
		op := domain.BalanceOperationSummary{ID: uuid.New(), AccountID: account.ID, Amount: s.money(amount), BalanceAfter: account.Balance, Type: BalanceOperationDeposit, CreatedAt: now}
		if err := tx.SaveBalanceOperation(ctx, op); err != nil {
			return err
		}
		owner, err := tx.UserByID(ctx, account.OwnerID)
		if err != nil {
			return err
		}
		result = domain.BalanceOperationDetailed{ID: op.ID, Account: detailed(account, owner), Amount: op.Amount, BalanceAfter: op.BalanceAfter, Type: op.Type, CreatedAt: now}
		return nil
	})
	return result, err
}

func (s *Service) Withdraw(ctx context.Context, accountID, currentUserID uuid.UUID, amount decimal.Decimal) (domain.BalanceOperationDetailed, error) {
	if err := validatePositive(amount, "amount"); err != nil {
		return domain.BalanceOperationDetailed{}, err
	}
	var result domain.BalanceOperationDetailed
	err := s.repository.InTx(ctx, func(tx Transaction) error {
		account, err := tx.AccountForUpdate(ctx, accountID)
		if err != nil {
			return err
		}
		if account.OwnerID != currentUserID {
			return domain.NewError(domain.ErrAccessDenied, "account does not belong to current user")
		}
		if amount.GreaterThan(account.Balance) {
			return domain.NewError(domain.ErrNotEnoughFunds, "not enough funds")
		}
		account.Balance = s.money(account.Balance.Sub(amount))
		if err := tx.UpdateAccount(ctx, account); err != nil {
			return err
		}
		now := time.Now().UTC()
		op := domain.BalanceOperationSummary{ID: uuid.New(), AccountID: account.ID, Amount: s.money(amount), BalanceAfter: account.Balance, Type: BalanceOperationWithdraw, CreatedAt: now}
		if err := tx.SaveBalanceOperation(ctx, op); err != nil {
			return err
		}
		owner, err := tx.UserByID(ctx, account.OwnerID)
		if err != nil {
			return err
		}
		result = domain.BalanceOperationDetailed{ID: op.ID, Account: detailed(account, owner), Amount: op.Amount, BalanceAfter: op.BalanceAfter, Type: op.Type, CreatedAt: now}
		return nil
	})
	return result, err
}

func (s *Service) Transfer(ctx context.Context, fromID, toID, currentUserID uuid.UUID, amount decimal.Decimal) (domain.TransferOperationDetailed, error) {
	if fromID == toID {
		return domain.TransferOperationDetailed{}, domain.NewError(domain.ErrValidation, "source and target accounts must be different")
	}
	if err := validatePositive(amount, "amount"); err != nil {
		return domain.TransferOperationDetailed{}, err
	}
	var result domain.TransferOperationDetailed
	err := s.repository.InTx(ctx, func(tx Transaction) error {
		ids := []uuid.UUID{fromID, toID}
		sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
		first, err := tx.AccountForUpdate(ctx, ids[0])
		if err != nil {
			return err
		}
		second, err := tx.AccountForUpdate(ctx, ids[1])
		if err != nil {
			return err
		}
		from, to := first, second
		if first.ID != fromID {
			from, to = second, first
		}
		if from.OwnerID != currentUserID {
			return domain.NewError(domain.ErrAccessDenied, "source account does not belong to current user")
		}
		sender, err := tx.UserByID(ctx, from.OwnerID)
		if err != nil {
			return err
		}
		receiver, err := tx.UserByID(ctx, to.OwnerID)
		if err != nil {
			return err
		}
		commissionRate := s.commission.Default
		if sender.ID == receiver.ID {
			commissionRate = s.commission.Own
		} else {
			friend, err := tx.FriendExists(ctx, sender.ID, receiver.ID)
			if err != nil {
				return err
			}
			if friend {
				commissionRate = s.commission.Friend
			}
		}
		commission := s.money(amount.Mul(commissionRate))
		total := s.money(amount.Add(commission))
		if total.GreaterThan(from.Balance) {
			return domain.NewError(domain.ErrNotEnoughFunds, "not enough funds for amount and commission")
		}
		from.Balance = s.money(from.Balance.Sub(total))
		to.Balance = s.money(to.Balance.Add(amount))
		if err := tx.UpdateAccount(ctx, from); err != nil {
			return err
		}
		if err := tx.UpdateAccount(ctx, to); err != nil {
			return err
		}
		now := time.Now().UTC()
		op := domain.TransferOperationSummary{ID: uuid.New(), AccountFromID: from.ID, Amount: s.money(amount), BalanceAfter: from.Balance, CommissionAmount: commission, AccountToID: to.ID, CreatedAt: now}
		if err := tx.SaveTransfer(ctx, op, to.Balance); err != nil {
			return err
		}
		result = domain.TransferOperationDetailed{ID: op.ID, AccountDetailedResponse: detailed(from, sender), Amount: op.Amount, BalanceAfter: from.Balance, CommissionAmount: commission, RelatedAccountDTO: detailed(to, receiver), CreatedAt: now}
		return nil
	})
	return result, err
}

func (s *Service) BalanceOperations(ctx context.Context, f OperationFilter) (domain.Page[domain.BalanceOperationSummary], error) {
	return s.repository.ListBalanceOperations(ctx, f)
}

func (s *Service) Transfers(ctx context.Context, f TransferFilter) (domain.Page[domain.TransferOperationSummary], error) {
	return s.repository.ListTransfers(ctx, f)
}

func (s *Service) BalanceInCurrency(ctx context.Context, accountID, currentUserID uuid.UUID, currency string) (decimal.Decimal, error) {
	account, err := s.repository.AccountByID(ctx, accountID)
	if err != nil {
		return decimal.Zero, err
	}
	if account.OwnerID != currentUserID {
		return decimal.Zero, domain.NewError(domain.ErrAccessDenied, "account does not belong to current user")
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return decimal.Zero, domain.NewError(domain.ErrValidation, "currency must not be blank")
	}
	rate, ok, err := s.cachedRate(ctx, currency)
	if err != nil {
		return decimal.Zero, err
	}
	if !ok {
		response, err := s.rateProvider.GetRate(ctx, currency)
		if err != nil {
			return decimal.Zero, err
		}
		if !response.Success || response.RateToRUB == nil || response.Timestamp == nil {
			message := "currency not found: " + currency
			if response.ErrorMessage != nil {
				message = *response.ErrorMessage
			}
			return decimal.Zero, domain.NewError(domain.ErrNotFound, message)
		}
		rate = domain.ExchangeRate{Currency: response.Currency, RateToRUB: *response.RateToRUB, Timestamp: *response.Timestamp}
		if err := s.UpdateRate(ctx, rate); err != nil {
			return decimal.Zero, err
		}
	}
	if rate.RateToRUB.IsZero() {
		return decimal.Zero, domain.NewError(domain.ErrValidation, "exchange rate must be positive")
	}
	return account.Balance.DivRound(rate.RateToRUB, s.policy.MoneyScale), nil
}

func (s *Service) UpdateRate(ctx context.Context, rate domain.ExchangeRate) error {
	rate.Currency = strings.ToUpper(strings.TrimSpace(rate.Currency))
	if err := s.rateRepository.Save(ctx, rate); err != nil {
		return fmt.Errorf("save %s rate: %w", rate.Currency, err)
	}
	return nil
}

func (s *Service) cachedRate(ctx context.Context, currency string) (domain.ExchangeRate, bool, error) {
	rate, ok, err := s.rateRepository.Get(ctx, currency)
	if err != nil {
		return domain.ExchangeRate{}, false, fmt.Errorf("get %s rate: %w", currency, err)
	}
	if !ok {
		return domain.ExchangeRate{}, false, nil
	}
	return rate, time.Since(rate.Timestamp) <= s.rateTTL, nil
}

func detailed(account AccountRecord, owner domain.User) domain.AccountDetailed {
	return domain.AccountDetailed{ID: account.ID, Balance: account.Balance, Owner: owner}
}

func (s *Service) money(value decimal.Decimal) decimal.Decimal {
	return value.Round(s.policy.MoneyScale)
}

func validatePositive(value decimal.Decimal, field string) error {
	if !value.IsPositive() {
		return domain.NewError(domain.ErrValidation, field+" must be greater than 0")
	}
	return nil
}
