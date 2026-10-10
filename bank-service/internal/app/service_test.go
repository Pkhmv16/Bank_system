package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"

	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/domain"
)

func TestCreateUserAndAuthenticate(t *testing.T) {
	controller := gomock.NewController(t)
	repository := NewMockRepository(controller)
	rateProvider := NewMockRateProvider(controller)
	service := newTestService(controller, repository, rateProvider)

	var stored domain.User
	repository.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, user domain.User) error {
			stored = user
			return nil
		},
	)

	created, err := service.CreateUser(context.Background(), CreateUserRequest{
		Login: " alice ", FirstName: " Alice ", LastName: " Doe ", Password: "secret",
		Age: 25, Gender: domain.GenderFemale, Role: domain.RoleUser,
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if created.Login != "alice" || created.FirstName != "Alice" || created.LastName != "Doe" {
		t.Fatalf("CreateUser() did not trim fields: %+v", created)
	}
	if created.PasswordHash == "secret" || bcrypt.CompareHashAndPassword([]byte(created.PasswordHash), []byte("secret")) != nil {
		t.Fatal("CreateUser() did not securely hash the password")
	}

	repository.EXPECT().UserByLogin(gomock.Any(), "alice").Return(stored, nil)
	authenticated, err := service.Authenticate(context.Background(), "alice", "secret")
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if authenticated.ID != created.ID {
		t.Fatalf("Authenticate() user ID = %s, want %s", authenticated.ID, created.ID)
	}

	repository.EXPECT().UserByLogin(gomock.Any(), "alice").Return(stored, nil)
	if _, err := service.Authenticate(context.Background(), "alice", "wrong"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("Authenticate() error = %v, want ErrInvalidCredentials", err)
	}
}

func TestCreateUserValidation(t *testing.T) {
	tests := []struct {
		name    string
		request CreateUserRequest
	}{
		{name: "blank login", request: CreateUserRequest{FirstName: "A", LastName: "B", Password: "secret", Age: 20, Gender: domain.GenderMale, Role: domain.RoleUser}},
		{name: "age out of range", request: CreateUserRequest{Login: "a", FirstName: "A", LastName: "B", Password: "secret", Age: 151, Gender: domain.GenderMale, Role: domain.RoleUser}},
		{name: "unknown gender", request: CreateUserRequest{Login: "a", FirstName: "A", LastName: "B", Password: "secret", Age: 20, Gender: "UNKNOWN", Role: domain.RoleUser}},
		{name: "unknown role", request: CreateUserRequest{Login: "a", FirstName: "A", LastName: "B", Password: "secret", Age: 20, Gender: domain.GenderMale, Role: "UNKNOWN"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			controller := gomock.NewController(t)
			service := newTestService(controller, NewMockRepository(controller), NewMockRateProvider(controller))
			if _, err := service.CreateUser(context.Background(), test.request); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("CreateUser() error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestTransferAppliesConfiguredCommission(t *testing.T) {
	tests := []struct {
		name       string
		sameOwner  bool
		friend     bool
		commission string
		sender     string
	}{
		{name: "own account", sameOwner: true, commission: "0", sender: "900"},
		{name: "friend", friend: true, commission: "3", sender: "897"},
		{name: "regular recipient", commission: "10", sender: "890"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			controller := gomock.NewController(t)
			repository := NewMockRepository(controller)
			transaction := NewMockTransaction(controller)
			rateProvider := NewMockRateProvider(controller)

			senderID, receiverID := uuid.New(), uuid.New()
			senderOwnerID, receiverOwnerID := uuid.New(), uuid.New()
			if test.sameOwner {
				receiverOwnerID = senderOwnerID
			}
			sender := domain.User{ID: senderOwnerID, Login: "sender"}
			receiver := domain.User{ID: receiverOwnerID, Login: "receiver"}
			accounts := map[uuid.UUID]AccountRecord{
				senderID:   {ID: senderID, OwnerID: senderOwnerID, Balance: decimal.NewFromInt(1000)},
				receiverID: {ID: receiverID, OwnerID: receiverOwnerID, Balance: decimal.Zero},
			}

			expectTransaction(repository, transaction)
			transaction.EXPECT().AccountForUpdate(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
				func(_ context.Context, id uuid.UUID) (AccountRecord, error) { return accounts[id], nil },
			)
			transaction.EXPECT().UserByID(gomock.Any(), senderOwnerID).Return(sender, nil)
			transaction.EXPECT().UserByID(gomock.Any(), receiverOwnerID).Return(receiver, nil)
			if !test.sameOwner {
				transaction.EXPECT().FriendExists(gomock.Any(), senderOwnerID, receiverOwnerID).Return(test.friend, nil)
			}
			transaction.EXPECT().UpdateAccount(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
				func(_ context.Context, account AccountRecord) error {
					accounts[account.ID] = account
					return nil
				},
			)
			transaction.EXPECT().SaveTransfer(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			result, err := newTestService(controller, repository, rateProvider).Transfer(
				context.Background(), senderID, receiverID, senderOwnerID, decimal.NewFromInt(100),
			)
			if err != nil {
				t.Fatalf("Transfer() error = %v", err)
			}
			assertDecimal(t, "commission", result.CommissionAmount, test.commission)
			assertDecimal(t, "sender balance", accounts[senderID].Balance, test.sender)
			assertDecimal(t, "receiver balance", accounts[receiverID].Balance, "100")
		})
	}
}

func TestTransferRejectsInsufficientFunds(t *testing.T) {
	controller := gomock.NewController(t)
	repository := NewMockRepository(controller)
	transaction := NewMockTransaction(controller)
	rateProvider := NewMockRateProvider(controller)
	senderID, receiverID := uuid.New(), uuid.New()
	senderOwnerID, receiverOwnerID := uuid.New(), uuid.New()
	accounts := map[uuid.UUID]AccountRecord{
		senderID:   {ID: senderID, OwnerID: senderOwnerID, Balance: decimal.NewFromInt(100)},
		receiverID: {ID: receiverID, OwnerID: receiverOwnerID, Balance: decimal.Zero},
	}

	expectTransaction(repository, transaction)
	transaction.EXPECT().AccountForUpdate(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
		func(_ context.Context, id uuid.UUID) (AccountRecord, error) { return accounts[id], nil },
	)
	transaction.EXPECT().UserByID(gomock.Any(), senderOwnerID).Return(domain.User{ID: senderOwnerID}, nil)
	transaction.EXPECT().UserByID(gomock.Any(), receiverOwnerID).Return(domain.User{ID: receiverOwnerID}, nil)
	transaction.EXPECT().FriendExists(gomock.Any(), senderOwnerID, receiverOwnerID).Return(false, nil)

	_, err := newTestService(controller, repository, rateProvider).Transfer(
		context.Background(), senderID, receiverID, senderOwnerID, decimal.NewFromInt(100),
	)
	if !errors.Is(err, domain.ErrNotEnoughFunds) {
		t.Fatalf("Transfer() error = %v, want ErrNotEnoughFunds", err)
	}
}

func TestDepositAndWithdraw(t *testing.T) {
	controller := gomock.NewController(t)
	repository := NewMockRepository(controller)
	transaction := NewMockTransaction(controller)
	rateProvider := NewMockRateProvider(controller)
	ownerID, accountID := uuid.New(), uuid.New()
	owner := domain.User{ID: ownerID, Login: "owner"}
	account := AccountRecord{ID: accountID, OwnerID: ownerID, Balance: decimal.NewFromInt(100)}

	repository.EXPECT().InTx(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
		func(ctx context.Context, callback func(Transaction) error) error { return callback(transaction) },
	)
	transaction.EXPECT().AccountForUpdate(gomock.Any(), accountID).Times(2).DoAndReturn(
		func(context.Context, uuid.UUID) (AccountRecord, error) { return account, nil },
	)
	transaction.EXPECT().UpdateAccount(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
		func(_ context.Context, updated AccountRecord) error {
			account = updated
			return nil
		},
	)
	transaction.EXPECT().SaveBalanceOperation(gomock.Any(), gomock.Any()).Times(2).Return(nil)
	transaction.EXPECT().UserByID(gomock.Any(), ownerID).Times(2).Return(owner, nil)
	service := newTestService(controller, repository, rateProvider)

	deposit, err := service.Deposit(context.Background(), accountID, ownerID, decimal.RequireFromString("50.555"))
	if err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	assertDecimal(t, "deposit balance", deposit.BalanceAfter, "150.56")

	withdrawal, err := service.Withdraw(context.Background(), accountID, ownerID, decimal.RequireFromString("25.555"))
	if err != nil {
		t.Fatalf("Withdraw() error = %v", err)
	}
	assertDecimal(t, "withdrawal balance", withdrawal.BalanceAfter, "125.01")
}

func TestBalanceInCurrencyUsesCachedRate(t *testing.T) {
	controller := gomock.NewController(t)
	repository := NewMockRepository(controller)
	rateRepository := NewMockRateRepository(controller)
	rateProvider := NewMockRateProvider(controller)
	ownerID, accountID := uuid.New(), uuid.New()
	account := AccountRecord{ID: accountID, OwnerID: ownerID, Balance: decimal.NewFromInt(7000)}
	rate := decimal.NewFromInt(70)
	timestamp := time.Now().UTC()

	repository.EXPECT().AccountByID(gomock.Any(), accountID).Times(2).Return(account, nil)
	gomock.InOrder(
		rateRepository.EXPECT().Get(gomock.Any(), "USD").Return(domain.ExchangeRate{}, false, nil),
		rateProvider.EXPECT().GetRate(gomock.Any(), "USD").Return(domain.ExchangeRateResponse{
			Success: true, Currency: "USD", RateToRUB: &rate, Timestamp: &timestamp,
		}, nil),
		rateRepository.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
		rateRepository.EXPECT().Get(gomock.Any(), "USD").Return(domain.ExchangeRate{
			Currency: "USD", RateToRUB: rate, Timestamp: timestamp,
		}, true, nil),
	)
	service := newTestServiceWithRateRepository(repository, rateRepository, rateProvider)

	for call := 0; call < 2; call++ {
		balance, err := service.BalanceInCurrency(context.Background(), accountID, ownerID, " usd ")
		if err != nil {
			t.Fatalf("BalanceInCurrency() error = %v", err)
		}
		assertDecimal(t, "converted balance", balance, "100")
	}
}

func TestBalanceInCurrencyRejectsForeignAccount(t *testing.T) {
	controller := gomock.NewController(t)
	repository := NewMockRepository(controller)
	rateProvider := NewMockRateProvider(controller)
	accountID := uuid.New()
	repository.EXPECT().AccountByID(gomock.Any(), accountID).Return(
		AccountRecord{ID: accountID, OwnerID: uuid.New(), Balance: decimal.NewFromInt(100)}, nil,
	)

	_, err := newTestService(controller, repository, rateProvider).BalanceInCurrency(context.Background(), accountID, uuid.New(), "USD")
	if !errors.Is(err, domain.ErrAccessDenied) {
		t.Fatalf("BalanceInCurrency() error = %v, want ErrAccessDenied", err)
	}
}

func TestAddFriendsRemovesDuplicateIDs(t *testing.T) {
	controller := gomock.NewController(t)
	repository := NewMockRepository(controller)
	rateProvider := NewMockRateProvider(controller)
	userID, friendID := uuid.New(), uuid.New()
	repository.EXPECT().AddFriends(gomock.Any(), userID, gomock.Eq([]uuid.UUID{friendID})).Return(nil)

	if err := newTestService(controller, repository, rateProvider).AddFriends(
		context.Background(), userID, []uuid.UUID{friendID, friendID},
	); err != nil {
		t.Fatalf("AddFriends() error = %v", err)
	}
}

func expectTransaction(repository *MockRepository, transaction *MockTransaction) {
	repository.EXPECT().InTx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, callback func(Transaction) error) error { return callback(transaction) },
	)
}

func newTestService(controller *gomock.Controller, repository Repository, rateProvider RateProvider) *Service {
	return newTestServiceWithRateRepository(repository, NewMockRateRepository(controller), rateProvider)
}

func newTestServiceWithRateRepository(repository Repository, rateRepository RateRepository, rateProvider RateProvider) *Service {
	return NewService(
		repository,
		rateRepository,
		rateProvider,
		time.Minute,
		CommissionConfig{Own: decimal.Zero, Friend: decimal.RequireFromString("0.03"), Default: decimal.RequireFromString("0.10")},
		bcrypt.MinCost,
		PolicyConfig{MinimumUserAge: 1, MaximumUserAge: 150, MoneyScale: 2},
	)
}

func assertDecimal(t *testing.T, name string, actual decimal.Decimal, expected string) {
	t.Helper()
	want := decimal.RequireFromString(expected)
	if !actual.Equal(want) {
		t.Fatalf("%s = %s, want %s", name, actual, want)
	}
}
