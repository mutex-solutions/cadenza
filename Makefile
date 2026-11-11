.PHONY: build install clean test test-postgres lint fmt gen check bench web

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X main.Version=$(VERSION)"
SQLC    ?= sqlc
# nodynamic: never load a system libwebp; the binary stays self-contained.
GOTAGS  := -tags nodynamic

build:
	CGO_ENABLED=0 go build $(GOTAGS) $(LDFLAGS) -o bin/cadenza ./cmd/cadenza/
	CGO_ENABLED=0 go build $(GOTAGS) $(LDFLAGS) -o bin/cadenza-server ./cmd/cadenza-server/

install: build
	mkdir -p ~/.local/bin
	cp bin/cadenza ~/.local/bin/cadenza
	@echo "Installed cadenza $(VERSION) to ~/.local/bin/cadenza"

clean:
	rm -rf bin/ dist/

test:
	go test $(GOTAGS) -race ./...

# Runs the store tests against a real Postgres too, e.g.
#   make test-postgres CADENZA_TEST_POSTGRES='postgres://postgres:cadenza@localhost:5432/cadenza?sslmode=disable'
test-postgres:
	CADENZA_TEST_POSTGRES='$(CADENZA_TEST_POSTGRES)' go test -race ./internal/adapter/sqlstore/...

lint:
	go vet $(GOTAGS) ./...

fmt:
	gofmt -w .

# Regenerate everything generated: sqlc output, the Postgres repository, the
# TypeScript API types and the agent guide.
gen:
	cd internal/adapter/sqlstore && $(SQLC) generate
	sh scripts/gen-postgres-repo.sh
	go run ./cmd/cadenza-gen

check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)
	go vet $(GOTAGS) ./...
	go test $(GOTAGS) -race ./...

bench:
	go test $(GOTAGS) -run '^$$' -bench . -benchmem ./...

# Build the web app and copy it into the server binary's embedded files.
web:
	cd web && pnpm install --frozen-lockfile && pnpm --filter @cadenza/web build
	rm -rf internal/server/webapp/dist && mkdir -p internal/server/webapp/dist
	cp -r web/apps/web/dist/. internal/server/webapp/dist/
	touch internal/server/webapp/dist/.gitkeep
