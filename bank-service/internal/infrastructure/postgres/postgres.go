package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/app"
	"github.com/evgenypakhomov/bank-system-go/bank-service/internal/domain"
)

type Store struct {
	pool       *pgxpool.Pool
	moneyScale int32
}

var _ app.Repository = (*Store)(nil)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type transaction struct {
	tx         pgx.Tx
	moneyScale int32
}

func (t *transaction) AccountForUpdate(ctx context.Context, id uuid.UUID) (app.AccountRecord, error) {
	return accountByID(ctx, t.tx, id, true)
}

func (t *transaction) UpdateAccount(ctx context.Context, account app.AccountRecord) error {
	return updateAccount(ctx, t.tx, account, t.moneyScale)
}

func (t *transaction) UserByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	return userByID(ctx, t.tx, id)
}

func (t *transaction) FriendExists(ctx context.Context, userID, friendID uuid.UUID) (bool, error) {
	return friendExists(ctx, t.tx, userID, friendID)
}

func (t *transaction) SaveBalanceOperation(ctx context.Context, operation domain.BalanceOperationSummary) error {
	return saveBalanceOperation(ctx, t.tx, operation, t.moneyScale)
}

func (t *transaction) SaveTransfer(ctx context.Context, operation domain.TransferOperationSummary, receiverBalance decimal.Decimal) error {
	return saveTransfer(ctx, t.tx, operation, receiverBalance, t.moneyScale)
}

func Open(ctx context.Context, databaseURL string, moneyScale int32) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{pool: pool, moneyScale: moneyScale}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) InTx(ctx context.Context, fn func(app.Transaction) error) error {
	return s.withTx(ctx, func(tx pgx.Tx) error { return fn(&transaction{tx: tx, moneyScale: s.moneyScale}) })
}

func (s *Store) withTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (s *Store) CreateUser(ctx context.Context, user domain.User) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, login, first_name, last_name, age, gender, password_hash, role)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		user.ID, user.Login, user.FirstName, user.LastName, user.Age, user.Gender, user.PasswordHash, user.Role)
	return mapWriteError(err, "user with this login already exists")
}

func (s *Store) UserByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	return userByID(ctx, s.pool, id)
}

func (s *Store) UserByLogin(ctx context.Context, login string) (domain.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		SELECT id, login, first_name, last_name, age, gender, password_hash, role
		FROM users WHERE lower(login) = lower($1)`, strings.TrimSpace(login)))
}

func userByID(ctx context.Context, q DBTX, id uuid.UUID) (domain.User, error) {
	return scanUser(q.QueryRow(ctx, `
		SELECT id, login, first_name, last_name, age, gender, password_hash, role
		FROM users WHERE id = $1`, id))
}

func scanUser(row pgx.Row) (domain.User, error) {
	var user domain.User
	if err := row.Scan(&user.ID, &user.Login, &user.FirstName, &user.LastName, &user.Age,
		&user.Gender, &user.PasswordHash, &user.Role); err != nil {
		return domain.User{}, mapReadError(err, "user not found")
	}
	return user, nil
}

func (s *Store) AddFriends(ctx context.Context, userID uuid.UUID, friendIDs []uuid.UUID) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		if _, err := userByID(ctx, tx, userID); err != nil {
			return err
		}
		for _, friendID := range friendIDs {
			if userID == friendID {
				return domain.NewError(domain.ErrValidation, "user cannot be a friend of themselves")
			}
			if _, err := userByID(ctx, tx, friendID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO user_friends (user_id, friend_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, userID, friendID); err != nil {
				return fmt.Errorf("add friend: %w", err)
			}
		}
		return nil
	})
}

func (s *Store) DeleteFriends(ctx context.Context, userID uuid.UUID, friendIDs []uuid.UUID) error {
	if _, err := s.UserByID(ctx, userID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM user_friends WHERE user_id = $1 AND friend_id = ANY($2::uuid[])`, userID, friendIDs)
	if err != nil {
		return fmt.Errorf("delete friends: %w", err)
	}
	return nil
}

func (s *Store) Friends(ctx context.Context, userID uuid.UUID) ([]domain.User, error) {
	if _, err := s.UserByID(ctx, userID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.login, u.first_name, u.last_name, u.age, u.gender, u.password_hash, u.role
		FROM users u JOIN user_friends f ON f.friend_id = u.id
		WHERE f.user_id = $1 ORDER BY u.login`, userID)
	if err != nil {
		return nil, fmt.Errorf("list friends: %w", err)
	}
	defer rows.Close()
	return collectUsers(rows)
}

func (s *Store) ListUsers(ctx context.Context, f app.UserFilter) (domain.Page[domain.User], error) {
	where := []string{"TRUE"}
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if strings.TrimSpace(f.Login) != "" {
		add("lower(login) = lower($%d)", strings.TrimSpace(f.Login))
	}
	if strings.TrimSpace(f.FirstName) != "" {
		add("lower(first_name) = lower($%d)", strings.TrimSpace(f.FirstName))
	}
	if strings.TrimSpace(f.LastName) != "" {
		add("lower(last_name) = lower($%d)", strings.TrimSpace(f.LastName))
	}
	if f.MinAge != nil {
		add("age >= $%d", *f.MinAge)
	}
	if f.MaxAge != nil {
		add("age <= $%d", *f.MaxAge)
	}
	if len(f.Genders) > 0 {
		add("gender = ANY($%d::varchar[])", f.Genders)
	}
	condition := strings.Join(where, " AND ")
	var total int64
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE "+condition, args...).Scan(&total); err != nil {
		return domain.Page[domain.User]{}, fmt.Errorf("count users: %w", err)
	}
	args = append(args, f.PageSize, f.Page*f.PageSize)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, login, first_name, last_name, age, gender, password_hash, role
		FROM users WHERE %s ORDER BY login LIMIT $%d OFFSET $%d`, condition, len(args)-1, len(args)), args...)
	if err != nil {
		return domain.Page[domain.User]{}, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	items, err := collectUsers(rows)
	if err != nil {
		return domain.Page[domain.User]{}, err
	}
	return domain.NewPage(items, f.Page, f.PageSize, total), nil
}

func collectUsers(rows pgx.Rows) ([]domain.User, error) {
	users := make([]domain.User, 0)
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.Login, &user.FirstName, &user.LastName, &user.Age,
			&user.Gender, &user.PasswordHash, &user.Role); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) CreateAccount(ctx context.Context, account app.AccountRecord) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO accounts (id, balance, owner_id) VALUES ($1, $2::numeric, $3)`,
		account.ID, account.Balance.StringFixed(s.moneyScale), account.OwnerID)
	return mapWriteError(err, "account owner not found")
}

func (s *Store) AccountByID(ctx context.Context, id uuid.UUID) (app.AccountRecord, error) {
	return accountByID(ctx, s.pool, id, false)
}

func accountByID(ctx context.Context, q DBTX, id uuid.UUID, forUpdate bool) (app.AccountRecord, error) {
	query := `SELECT id, balance::text, owner_id FROM accounts WHERE id = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var account app.AccountRecord
	var balance string
	if err := q.QueryRow(ctx, query, id).Scan(&account.ID, &balance, &account.OwnerID); err != nil {
		return app.AccountRecord{}, mapReadError(err, "account not found")
	}
	parsed, err := decimal.NewFromString(balance)
	if err != nil {
		return app.AccountRecord{}, fmt.Errorf("parse account balance: %w", err)
	}
	account.Balance = parsed
	return account, nil
}

func updateAccount(ctx context.Context, tx pgx.Tx, account app.AccountRecord, moneyScale int32) error {
	_, err := tx.Exec(ctx, `UPDATE accounts SET balance = $2::numeric WHERE id = $1`, account.ID, account.Balance.StringFixed(moneyScale))
	if err != nil {
		return fmt.Errorf("update account: %w", err)
	}
	return nil
}

func friendExists(ctx context.Context, q DBTX, userID, friendID uuid.UUID) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_friends WHERE user_id = $1 AND friend_id = $2)`, userID, friendID).Scan(&exists)
	return exists, err
}

func (s *Store) UserAccounts(ctx context.Context, userID uuid.UUID) ([]domain.AccountSummary, error) {
	if _, err := s.UserByID(ctx, userID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id, balance::text, owner_id FROM accounts WHERE owner_id = $1 ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user accounts: %w", err)
	}
	defer rows.Close()
	return collectAccounts(rows)
}

func (s *Store) ListAccounts(ctx context.Context, f app.AccountFilter) (domain.Page[domain.AccountSummary], error) {
	where := []string{"TRUE"}
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.MinBalance != nil {
		add("balance >= $%d::numeric", f.MinBalance.String())
	}
	if f.MaxBalance != nil {
		add("balance <= $%d::numeric", f.MaxBalance.String())
	}
	if f.OwnerID != nil {
		add("owner_id = $%d", *f.OwnerID)
	}
	condition := strings.Join(where, " AND ")
	var total int64
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM accounts WHERE "+condition, args...).Scan(&total); err != nil {
		return domain.Page[domain.AccountSummary]{}, fmt.Errorf("count accounts: %w", err)
	}
	args = append(args, f.PageSize, f.Page*f.PageSize)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`SELECT id, balance::text, owner_id FROM accounts WHERE %s ORDER BY id LIMIT $%d OFFSET $%d`, condition, len(args)-1, len(args)), args...)
	if err != nil {
		return domain.Page[domain.AccountSummary]{}, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()
	items, err := collectAccounts(rows)
	if err != nil {
		return domain.Page[domain.AccountSummary]{}, err
	}
	return domain.NewPage(items, f.Page, f.PageSize, total), nil
}

func collectAccounts(rows pgx.Rows) ([]domain.AccountSummary, error) {
	items := make([]domain.AccountSummary, 0)
	for rows.Next() {
		var item domain.AccountSummary
		var balance string
		if err := rows.Scan(&item.ID, &balance, &item.OwnerID); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		parsed, err := decimal.NewFromString(balance)
		if err != nil {
			return nil, err
		}
		item.Balance = parsed
		items = append(items, item)
	}
	return items, rows.Err()
}

func saveBalanceOperation(ctx context.Context, tx pgx.Tx, op domain.BalanceOperationSummary, moneyScale int32) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO balance_operations (id, account_id, amount, balance_after, type, created_at)
		VALUES ($1, $2, $3::numeric, $4::numeric, $5, $6)`, op.ID, op.AccountID,
		op.Amount.StringFixed(moneyScale), op.BalanceAfter.StringFixed(moneyScale), op.Type, op.CreatedAt)
	if err != nil {
		return fmt.Errorf("save balance operation: %w", err)
	}
	return nil
}

func saveTransfer(ctx context.Context, tx pgx.Tx, op domain.TransferOperationSummary, receiverBalance decimal.Decimal, moneyScale int32) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO transfer_operations
		(id, account_id, amount, sender_balance_after, commission_amount, target_account_id, created_at, receiver_balance_after)
		VALUES ($1, $2, $3::numeric, $4::numeric, $5::numeric, $6, $7, $8::numeric)`,
		op.ID, op.AccountFromID, op.Amount.StringFixed(moneyScale), op.BalanceAfter.StringFixed(moneyScale),
		op.CommissionAmount.StringFixed(moneyScale), op.AccountToID, op.CreatedAt, receiverBalance.StringFixed(moneyScale))
	if err != nil {
		return fmt.Errorf("save transfer operation: %w", err)
	}
	return nil
}

func (s *Store) ListBalanceOperations(ctx context.Context, f app.OperationFilter) (domain.Page[domain.BalanceOperationSummary], error) {
	where := []string{"TRUE"}
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.AccountID != nil {
		add("bo.account_id = $%d", *f.AccountID)
	}
	if len(f.Types) > 0 {
		add("bo.type = ANY($%d::varchar[])", f.Types)
	}
	if f.MinAmount != nil {
		add("bo.amount >= $%d::numeric", f.MinAmount.String())
	}
	if f.MaxAmount != nil {
		add("bo.amount <= $%d::numeric", f.MaxAmount.String())
	}
	if f.MinDate != nil {
		add("bo.created_at >= $%d", *f.MinDate)
	}
	if f.MaxDate != nil {
		add("bo.created_at <= $%d", *f.MaxDate)
	}
	if f.OwnerID != nil {
		add("a.owner_id = $%d", *f.OwnerID)
	}
	condition := strings.Join(where, " AND ")
	base := ` FROM balance_operations bo JOIN accounts a ON a.id = bo.account_id WHERE ` + condition
	var total int64
	if err := s.pool.QueryRow(ctx, "SELECT count(*)"+base, args...).Scan(&total); err != nil {
		return domain.Page[domain.BalanceOperationSummary]{}, err
	}
	args = append(args, f.PageSize, f.Page*f.PageSize)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`SELECT bo.id, bo.account_id, bo.amount::text, bo.balance_after::text, bo.type, bo.created_at %s ORDER BY bo.created_at DESC LIMIT $%d OFFSET $%d`, base, len(args)-1, len(args)), args...)
	if err != nil {
		return domain.Page[domain.BalanceOperationSummary]{}, err
	}
	defer rows.Close()
	items := make([]domain.BalanceOperationSummary, 0)
	for rows.Next() {
		var item domain.BalanceOperationSummary
		var amount, balance string
		if err := rows.Scan(&item.ID, &item.AccountID, &amount, &balance, &item.Type, &item.CreatedAt); err != nil {
			return domain.Page[domain.BalanceOperationSummary]{}, err
		}
		item.Amount, _ = decimal.NewFromString(amount)
		item.BalanceAfter, _ = decimal.NewFromString(balance)
		items = append(items, item)
	}
	return domain.NewPage(items, f.Page, f.PageSize, total), rows.Err()
}

func (s *Store) ListTransfers(ctx context.Context, f app.TransferFilter) (domain.Page[domain.TransferOperationSummary], error) {
	where := []string{"TRUE"}
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.SenderID != nil {
		add("t.account_id = $%d", *f.SenderID)
	}
	if f.ReceiverID != nil {
		add("t.target_account_id = $%d", *f.ReceiverID)
	}
	if f.MinAmount != nil {
		add("t.amount > $%d::numeric", f.MinAmount.String())
	}
	if f.MaxAmount != nil {
		add("t.amount < $%d::numeric", f.MaxAmount.String())
	}
	if f.MinDate != nil {
		add("t.created_at > $%d", *f.MinDate)
	}
	if f.MaxDate != nil {
		add("t.created_at < $%d", *f.MaxDate)
	}
	if f.OwnerID != nil {
		args = append(args, *f.OwnerID)
		where = append(where, fmt.Sprintf("(sender.owner_id = $%d OR receiver.owner_id = $%d)", len(args), len(args)))
	}
	condition := strings.Join(where, " AND ")
	base := ` FROM transfer_operations t JOIN accounts sender ON sender.id=t.account_id JOIN accounts receiver ON receiver.id=t.target_account_id WHERE ` + condition
	var total int64
	if err := s.pool.QueryRow(ctx, "SELECT count(*)"+base, args...).Scan(&total); err != nil {
		return domain.Page[domain.TransferOperationSummary]{}, err
	}
	args = append(args, f.PageSize, f.Page*f.PageSize)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`SELECT t.id, t.account_id, t.amount::text, t.sender_balance_after::text, t.commission_amount::text, t.target_account_id, t.created_at %s ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d`, base, len(args)-1, len(args)), args...)
	if err != nil {
		return domain.Page[domain.TransferOperationSummary]{}, err
	}
	defer rows.Close()
	items := make([]domain.TransferOperationSummary, 0)
	for rows.Next() {
		var item domain.TransferOperationSummary
		var amount, balance, commission string
		if err := rows.Scan(&item.ID, &item.AccountFromID, &amount, &balance, &commission, &item.AccountToID, &item.CreatedAt); err != nil {
			return domain.Page[domain.TransferOperationSummary]{}, err
		}
		item.Amount, _ = decimal.NewFromString(amount)
		item.BalanceAfter, _ = decimal.NewFromString(balance)
		item.CommissionAmount, _ = decimal.NewFromString(commission)
		items = append(items, item)
	}
	return domain.NewPage(items, f.Page, f.PageSize, total), rows.Err()
}

func mapReadError(err error, message string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NewError(domain.ErrNotFound, message)
	}
	return fmt.Errorf("postgres read: %w", err)
}

func mapWriteError(err error, conflictMessage string) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return domain.NewError(domain.ErrConflict, conflictMessage)
		case "23503":
			return domain.NewError(domain.ErrNotFound, conflictMessage)
		case "23514":
			return domain.NewError(domain.ErrValidation, pgErr.Message)
		}
	}
	return fmt.Errorf("postgres write: %w", err)
}
