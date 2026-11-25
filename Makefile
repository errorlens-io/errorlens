VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w -X github.com/rootcausehq/rootcause/internal/version.Version=$(VERSION) \
           -X github.com/rootcausehq/rootcause/internal/version.Commit=$(COMMIT)
TEST_DB ?= postgres://postgres:postgres@localhost:5434/postgres?sslmode=disable

.PHONY: all web build test test-unit lint dev-db dev-db-stop run docker clean

all: build

web: ## Build the React UI into web/dist (embedded by the Go build)
	cd web && npm ci && npm run build && touch dist/.gitkeep

build: web ## Build the single binary into bin/rootcause
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/rootcause ./cmd/rootcause

test: ## Run all tests (needs the dev database: make dev-db)
	ROOTCAUSE_TEST_DATABASE_URL="$(TEST_DB)" go test -race -count=1 ./...

test-unit: ## Run tests that don't need Postgres
	go test -count=1 ./...

lint:
	go vet ./...
	cd web && npm run typecheck

dev-db: ## Start a throwaway Postgres on localhost:5434
	docker run -d --name rootcause-dev-pg -e POSTGRES_PASSWORD=postgres -p 5434:5432 postgres:17-alpine

dev-db-stop:
	docker rm -f rootcause-dev-pg

run: build ## Run locally against the dev database
	ROOTCAUSE_DATABASE_URL="$(TEST_DB)" ./bin/rootcause serve

docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t rootcausehq/rootcause:$(VERSION) -f deploy/Dockerfile .

clean:
	rm -rf bin web/dist/assets web/dist/index.html web/dist/favicon.svg
