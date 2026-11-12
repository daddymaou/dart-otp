.PHONY: run build test migrate-up migrate-down db fmt vet

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./...

fmt:
	gofmt -l .

vet:
	go vet ./...

migrate-up:
	docker compose exec db psql -U dart -d dartotp -c "select 1"

migrate-down:

db:
	docker compose up -d
