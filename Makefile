DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/monolith?sslmode=disable
TEST_DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable
ADMIN_EMAIL ?= admin@localhost
ADMIN_PASSWORD ?= admin
export DATABASE_URL TEST_DATABASE_URL ADMIN_EMAIL ADMIN_PASSWORD

.PHONY: db run dev test build check

db:
	docker compose up -d --wait db

run: db
	go run ./cmd/server

dev: db
	go run github.com/air-verse/air@v1.67.4 \
		-build.cmd "go build -o tmp/server ./cmd/server" \
		-build.entrypoint tmp/server \
		-build.include_ext go,html,css,sql \
		-build.send_interrupt true

test: db
	go test -race ./...

build:
	go build -o bin/server ./cmd/server

check: db
	test -z "$$(gofmt -l .)"
	golangci-lint run
	go test -race ./...
	go mod tidy -diff
