-- +goose Up
CREATE TYPE user_role AS ENUM ('ADMIN', 'USER');

CREATE TABLE users (
    id UUID PRIMARY KEY,
    login VARCHAR(255) NOT NULL UNIQUE,
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255) NOT NULL,
    age INTEGER NOT NULL CHECK (age >= 0),
    gender VARCHAR(50) NOT NULL CHECK (gender IN ('MALE', 'FEMALE')),
    password_hash VARCHAR(255) NOT NULL,
    role user_role NOT NULL
);

CREATE TABLE user_friends (
    user_id UUID NOT NULL REFERENCES users(id),
    friend_id UUID NOT NULL REFERENCES users(id),
    PRIMARY KEY (user_id, friend_id),
    CHECK (user_id <> friend_id)
);
CREATE INDEX idx_user_friends_friend_id ON user_friends(friend_id);

CREATE TABLE accounts (
    id UUID PRIMARY KEY,
    balance NUMERIC NOT NULL DEFAULT 0 CHECK (balance >= 0),
    owner_id UUID NOT NULL REFERENCES users(id)
);
CREATE INDEX idx_accounts_owner_id ON accounts(owner_id);

CREATE TABLE balance_operations (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    amount NUMERIC NOT NULL CHECK (amount > 0),
    balance_after NUMERIC NOT NULL,
    type VARCHAR(50) NOT NULL CHECK (type IN ('DEPOSIT', 'WITHDRAW')),
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_balance_operations_account_id ON balance_operations(account_id);
CREATE INDEX idx_balance_operations_created_at ON balance_operations(created_at);
CREATE INDEX idx_balance_operations_type ON balance_operations(type);

CREATE TABLE transfer_operations (
    id UUID PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id),
    amount NUMERIC NOT NULL CHECK (amount > 0),
    sender_balance_after NUMERIC NOT NULL,
    commission_amount NUMERIC NOT NULL CHECK (commission_amount >= 0),
    target_account_id UUID NOT NULL REFERENCES accounts(id),
    created_at TIMESTAMPTZ NOT NULL,
    receiver_balance_after NUMERIC NOT NULL,
    CHECK (account_id <> target_account_id)
);
CREATE INDEX idx_transfer_operations_account_id ON transfer_operations(account_id);
CREATE INDEX idx_transfer_operations_target_account_id ON transfer_operations(target_account_id);
CREATE INDEX idx_transfer_operations_created_at ON transfer_operations(created_at);

-- +goose Down
DROP TABLE IF EXISTS transfer_operations;
DROP TABLE IF EXISTS balance_operations;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS user_friends;
DROP TABLE IF EXISTS users;
DROP TYPE IF EXISTS user_role;
