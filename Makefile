.DEFAULT_GOAL := help

GO ?= go
DOCKER_COMPOSE ?= $(shell if docker compose version >/dev/null 2>&1; then printf '%s' 'docker compose'; elif docker-compose version >/dev/null 2>&1; then printf '%s' 'docker-compose'; fi)

ROOT_COMPOSE_FILE ?= compose.yaml
LOCAL_COMPOSE_FILE ?= deployments/compose/local.yaml
MODEL_COMPOSE_FILE ?= deployments/compose/models.yaml

SERVICE_DIRS := ingestd normalized indexerd apid agentd backfill modeld
BIN_DIR ?= bin

define require_docker_compose
	@if [ -z "$(DOCKER_COMPOSE)" ]; then \
		echo "Docker Compose is not available. Install Compose support or set DOCKER_COMPOSE explicitly."; \
		exit 1; \
	fi
endef

.PHONY: help tidy fmt test build compile check build-bins clean \
	dev-up dev-down dev-logs dev-ps \
	dev-full-up dev-app-up dev-full-down \
	models-up models-down run-%

help:
	@printf "Available targets:\n"
	@printf "  %-18s %s\n" "build" "Compile all Go packages"
	@printf "  %-18s %s\n" "build-bins" "Build command binaries into $(BIN_DIR)/"
	@printf "  %-18s %s\n" "test" "Run the Go test suite"
	@printf "  %-18s %s\n" "check" "Run tests and build"
	@printf "  %-18s %s\n" "fmt" "Format Go packages"
	@printf "  %-18s %s\n" "tidy" "Sync Go module dependencies"
	@printf "  %-18s %s\n" "run-<service>" "Run a cmd/<service> entrypoint, e.g. make run-apid"
	@printf "  %-18s %s\n" "dev-up" "Start the minimal root Docker infrastructure stack"
	@printf "  %-18s %s\n" "dev-down" "Stop the minimal root Docker infrastructure stack"
	@printf "  %-18s %s\n" "dev-full-up" "Start the richer local Docker stack infrastructure"
	@printf "  %-18s %s\n" "dev-app-up" "Build and start the richer local Docker app profile"
	@printf "  %-18s %s\n" "dev-full-down" "Stop the richer local Docker stack"
	@printf "  %-18s %s\n" "models-up" "Start the model-serving Docker profile"
	@printf "  %-18s %s\n" "models-down" "Stop the model-serving Docker profile"

tidy:
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

build:
	$(GO) build ./...

compile: build

check: test build

build-bins:
	@mkdir -p $(BIN_DIR)
	@for service in $(SERVICE_DIRS); do \
		printf "building %s\n" "$$service"; \
		$(GO) build -o $(BIN_DIR)/$$service ./cmd/$$service || exit $$?; \
	done

clean:
	rm -rf $(BIN_DIR)

run-%:
	$(GO) run ./cmd/$*

dev-up:
	$(call require_docker_compose)
	$(DOCKER_COMPOSE) -f $(ROOT_COMPOSE_FILE) up -d

dev-down:
	$(call require_docker_compose)
	$(DOCKER_COMPOSE) -f $(ROOT_COMPOSE_FILE) down

dev-logs:
	$(call require_docker_compose)
	$(DOCKER_COMPOSE) -f $(ROOT_COMPOSE_FILE) logs -f

dev-ps:
	$(call require_docker_compose)
	$(DOCKER_COMPOSE) -f $(ROOT_COMPOSE_FILE) ps

dev-full-up:
	$(call require_docker_compose)
	$(DOCKER_COMPOSE) -f $(LOCAL_COMPOSE_FILE) up -d

dev-app-up:
	$(call require_docker_compose)
	$(DOCKER_COMPOSE) -f $(LOCAL_COMPOSE_FILE) --profile app up -d --build

dev-full-down:
	$(call require_docker_compose)
	$(DOCKER_COMPOSE) -f $(LOCAL_COMPOSE_FILE) down

models-up:
	$(call require_docker_compose)
	$(DOCKER_COMPOSE) -f $(MODEL_COMPOSE_FILE) --profile models up -d

models-down:
	$(call require_docker_compose)
	$(DOCKER_COMPOSE) -f $(MODEL_COMPOSE_FILE) down
