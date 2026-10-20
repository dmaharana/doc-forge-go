.PHONY: all build build-backend build-frontend test run-backend run-frontend clean help

GOOS ?= $(shell go env GOOS)
BINARY_EXT := $(if $(filter windows,$(GOOS)),.exe,)
BINARY := docforge-$(GOOS)$(BINARY_EXT)

# Default target
all: build

help:
	@echo "DocForge Makefile commands:"
	@echo "  make build          - Build both backend (CGO_ENABLED=0) and frontend"
	@echo "  make build-backend  - Build static backend binary ($(BINARY))"
	@echo "  make build-frontend - Build frontend production bundle"
	@echo "  make test           - Run backend automated test suite"
	@echo "  make run-backend    - Run backend development server ($(BINARY))"
	@echo "  make run-frontend   - Run frontend Vite development server"
	@echo "  make clean          - Clean build artifacts and binaries"

build: build-frontend build-backend

build-frontend:
	pnpm -C frontend run build

build-backend:
	cd backend && CGO_ENABLED=0 go build -o $(BINARY) ./cmd/server

test:
	cd backend && go test -v ./...

run-backend:
	cd backend && ./$(BINARY)

run-frontend:
	pnpm -C frontend run dev

clean:
	rm -f backend/$(BINARY) backend/docforge-* backend/server backend/docforge.db
	rm -rf backend/internal/web/dist

