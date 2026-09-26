GO ?= go
NPM ?= npm
CONTAINER_TOOL ?= podman
OAPI_CODEGEN_VERSION := v2.8.0
SQLC_VERSION := v1.31.1
OAPI_CODEGEN := $(GO) run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
SQLC := $(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
VERSION ?= dev
COMMIT ?= unknown
BUILD_DATE ?= unknown
LDFLAGS := -s -w -X portinaia/internal/version.Version=$(VERSION) -X portinaia/internal/version.Commit=$(COMMIT) -X portinaia/internal/version.Date=$(BUILD_DATE)
GO_PACKAGES := ./api/... ./cmd/... ./internal/... ./web

.PHONY: generate generate-api generate-db generate-check format format-check lint test test-web test-e2e verify build build-web build-agent image image-amd64 image-arm64 release

generate: generate-api generate-db build-web

generate-api:
	mkdir -p internal/api/generated web/src/api
	rm -f internal/api/generated/*.go
	$(OAPI_CODEGEN) -generate types,chi-server,strict-server -package generated -o internal/api/generated/openapi.gen.go api/openapi.yaml
	$(NPM) exec --workspace web -- openapi-typescript ../api/openapi.yaml --output src/api/generated.ts
	$(NPM) exec -- prettier --write web/src/api/generated.ts

generate-db:
	mkdir -p internal/database/generated
	rm -f internal/database/generated/*.go
	$(SQLC) generate

generate-check:
	@set -e; tmp="$$(mktemp -d)"; trap 'rm -rf "$$tmp"' EXIT; \
		cp -R internal/api/generated "$$tmp/api-generated"; \
		cp -R internal/database/generated "$$tmp/database-generated"; \
		cp web/src/api/generated.ts "$$tmp/generated.ts"; \
		cp -R web/dist "$$tmp/web-dist"; \
		$(MAKE) generate >/dev/null; \
		diff -ru "$$tmp/api-generated" internal/api/generated; \
		diff -ru "$$tmp/database-generated" internal/database/generated; \
		diff -u "$$tmp/generated.ts" web/src/api/generated.ts; \
		diff -ru "$$tmp/web-dist" web/dist

format:
	$(GO) fmt $(GO_PACKAGES)
	$(NPM) run format

format-check:
	@test -z "$$(gofmt -l $$(go list -f '{{$$dir := .Dir}}{{range .GoFiles}}{{$$dir}}/{{.}} {{end}}{{range .TestGoFiles}}{{$$dir}}/{{.}} {{end}}' $(GO_PACKAGES)))"
	$(NPM) run format:check

lint:
	$(GO) vet $(GO_PACKAGES)
	$(NPM) run typecheck
	$(NPM) run lint

test:
	$(GO) test $(GO_PACKAGES)

test-web:
	$(NPM) test

test-e2e:
	$(NPM) run test:e2e

verify: generate-check format-check lint test test-web build build-agent

build: build-web
	mkdir -p bin
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/portinaia ./cmd/portinaia

build-web:
	$(NPM) run build

build-agent:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/portinaia-agent-linux-amd64 ./cmd/portinaia-agent
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/portinaia-agent-linux-arm64 ./cmd/portinaia-agent

image: image-amd64 image-arm64

image-amd64:
	$(CONTAINER_TOOL) build --platform linux/amd64 --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_DATE=$(BUILD_DATE) --tag portinaia:$(VERSION)-amd64 .

image-arm64:
	$(CONTAINER_TOOL) build --platform linux/arm64 --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_DATE=$(BUILD_DATE) --tag portinaia:$(VERSION)-arm64 .

release: build-web
	rm -rf release
	mkdir -p release
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o release/portinaia-linux-amd64 ./cmd/portinaia
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o release/portinaia-linux-arm64 ./cmd/portinaia
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o release/portinaia-agent-linux-amd64 ./cmd/portinaia-agent
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o release/portinaia-agent-linux-arm64 ./cmd/portinaia-agent
	cd release && shasum -a 256 portinaia-* > checksums.txt
