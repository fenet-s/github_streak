.PHONY: run build test migrate docker-up docker-down tidy lint

# ── Dev ────────────────────────────────────────────────────────────
run:
	go run ./cmd/server

build:
	go build -o streak-guardian ./cmd/server

# ── Test ───────────────────────────────────────────────────────────
test:
	go test -v -race ./...

test-short:
	go test -short ./...

# ── Database ───────────────────────────────────────────────────────
migrate:
	@echo "Running migrations..."
	@go run ./cmd/server --migrate-only

# ── Docker ─────────────────────────────────────────────────────────
docker-up:
	docker-compose up -d
	@echo "Waiting for PostgreSQL to be ready..."
	@timeout /t 5 >nul 2>&1 || sleep 5

docker-down:
	docker-compose down

docker-clean:
	docker-compose down -v

# ── Dependencies ───────────────────────────────────────────────────
tidy:
	go mod tidy

lint:
	golangci-lint run ./...
