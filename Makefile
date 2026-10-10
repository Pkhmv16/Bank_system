.PHONY: run-bank run-rates generate test lint compose-up compose-down

run-bank:
	cd bank-service && go run ./cmd/bank

run-rates:
	cd rates-service && go run ./cmd/rates

generate:
	buf generate --template buf.gen.bank.yaml
	buf generate --template buf.gen.rates.yaml
	cd bank-service && go generate ./internal/app
	cd rates-service && go generate ./internal/app

test:
	cd bank-service && go test -race ./...
	cd rates-service && go test -race ./...

lint:
	cd bank-service && go vet ./...
	cd rates-service && go vet ./...

compose-up:
	docker compose up --build

compose-down:
	docker compose down
