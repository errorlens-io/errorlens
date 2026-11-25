VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w -X github.com/errorlens-io/errorlens/internal/version.Version=$(VERSION) \
           -X github.com/errorlens-io/errorlens/internal/version.Commit=$(COMMIT)
TEST_DB ?= postgres://postgres:postgres@localhost:5434/postgres?sslmode=disable

.PHONY: all web build test test-unit lint dev-db dev-db-stop run docker clean

all: build

web: ## Build the React UI into web/dist (embedded by the Go build)
	cd web && npm ci && npm run build && touch dist/.gitkeep

build: web ## Build the single binary into bin/errorlens
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/errorlens ./cmd/errorlens

test: ## Run all tests (needs the dev database: make dev-db)
	ERRORLENS_TEST_DATABASE_URL="$(TEST_DB)" go test -race -count=1 ./...

test-unit: ## Run tests that don't need Postgres
	go test -count=1 ./...

lint:
	go vet ./...
	cd web && npm run typecheck

dev-db: ## Start a throwaway Postgres on localhost:5434
	docker run -d --name errorlens-dev-pg -e POSTGRES_PASSWORD=postgres -p 5434:5432 postgres:17-alpine

dev-db-stop:
	docker rm -f errorlens-dev-pg

run: build ## Run locally against the dev database
	ERRORLENS_DATABASE_URL="$(TEST_DB)" ./bin/errorlens serve

docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t errorlensio/errorlens:$(VERSION) -f deploy/Dockerfile .

clean:
	rm -rf bin web/dist/assets web/dist/index.html web/dist/favicon.svg
