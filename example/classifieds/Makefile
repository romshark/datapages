NATS_CONTAINER_NAME := nats-datapages-demo
NATS_PORT := 4222
NATS_HTTP_PORT := 8222
NATS_IMAGE := nats:2.12.3

PROM_CONTAINER_NAME := prometheus-datapages-demo
PROM_PORT := 9091
PROM_IMAGE := prom/prometheus:v3.5.1
PROM_CONFIG := ./prometheus.yml

GRAFANA_CONTAINER_NAME := grafana-datapages-demo
GRAFANA_PORT := 3000
GRAFANA_IMAGE := grafana/grafana:12.3.0

GRAFANA_DATA := ./grafana/data
GRAFANA_PROVISIONING := ./grafana/provisioning

STAGE_SERVER_BIN := ./server
STAGE_ENV := ./.env.stage

# mkcert stage certificate
STAGE_CERT_DIR := ./certs
STAGE_CERT_FILE := $(STAGE_CERT_DIR)/stage.crt
STAGE_KEY_FILE  := $(STAGE_CERT_DIR)/stage.key

# Read HOST from .env.stage (fallback if missing)
STAGE_HOST ?= $(shell awk -F= '$$1=="HOST"{print $$2}' $(STAGE_ENV) 2>/dev/null)

# Hosts to include in the cert: stage host + local loopbacks for convenience
MKCERT_HOSTS := $(STAGE_HOST) localhost 127.0.0.1 ::1

K6 := k6
LOAD_SCRIPT := ./k6/load.js
LOAD_ENV ?= ./.env.dev

dev: nats-stop nats-up prom-stop prom-up grafana-stop grafana-up
	@set -a; [ -f .env.dev ] && . .env.dev; set +a; \
	go run github.com/romshark/templier@latest

# make stage runs the server in a production-grade mode for QA testing.
stage: check-hosts nats-stop nats-up prom-stop prom-up grafana-stop grafana-up certstage build
	@set -a; [ -f $(STAGE_ENV) ] && . $(STAGE_ENV); set +a; \
	sudo -E $(STAGE_SERVER_BIN); status=$$?; rm -f $(STAGE_SERVER_BIN); exit $$status

build:
	go build -o $(STAGE_SERVER_BIN) ./cmd/server/

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...

check-hosts:
	@HOST=$$(awk -F= '$$1=="HOST"{print $$2}' $(STAGE_ENV) 2>/dev/null); \
	test -n "$$HOST" || { echo "HOST missing in $(STAGE_ENV)"; exit 1; }; \
	if ! grep -q "127.0.0.1[[:space:]].*$$HOST" /etc/hosts && \
	   ! grep -q "::1[[:space:]].*$$HOST" /etc/hosts; then \
		echo "Error: the staging host '$$HOST' is not in /etc/hosts"; \
		echo ""; \
		echo "Add the following line to /etc/hosts:"; \
		echo "  127.0.0.1  $$HOST"; \
		echo ""; \
		echo "Run this command:"; \
		echo "  echo '127.0.0.1  $$HOST' | sudo tee -a /etc/hosts"; \
		exit 1; \
	fi

nats-stop:
	@docker stop $(NATS_CONTAINER_NAME) 2>/dev/null || true

nats-up:
	@docker ps --format '{{.Names}}' | grep -qx '$(NATS_CONTAINER_NAME)' || \
	docker run -d --rm \
		--name $(NATS_CONTAINER_NAME) \
		-p $(NATS_PORT):4222 \
		-p $(NATS_HTTP_PORT):8222 \
		$(NATS_IMAGE) \
		-js

prom-stop:
	@docker stop $(PROM_CONTAINER_NAME) 2>/dev/null || true

prom-up: $(PROM_CONFIG)
	@docker ps --format '{{.Names}}' | grep -qx '$(PROM_CONTAINER_NAME)' || \
	docker run -d --rm \
		--name $(PROM_CONTAINER_NAME) \
		-p $(PROM_PORT):9090 \
		-v $(abspath $(PROM_CONFIG)):/etc/prometheus/prometheus.yml:ro \
		$(PROM_IMAGE)

grafana-stop:
	@docker stop $(GRAFANA_CONTAINER_NAME) 2>/dev/null || true

grafana-up: $(GRAFANA_DATA)
	@docker ps --format '{{.Names}}' | grep -qx '$(GRAFANA_CONTAINER_NAME)' || \
	docker run -d --rm \
		--name $(GRAFANA_CONTAINER_NAME) \
		-p $(GRAFANA_PORT):3000 \
		-v $(abspath $(GRAFANA_DATA)):/var/lib/grafana \
		-v $(abspath $(GRAFANA_PROVISIONING)):/etc/grafana/provisioning:ro \
		-v $(abspath ./grafana/dashboards):/etc/grafana/dashboards:ro \
		-e GF_SECURITY_ADMIN_USER=admin \
		-e GF_SECURITY_ADMIN_PASSWORD=admin \
		$(GRAFANA_IMAGE)

$(GRAFANA_DATA):
	mkdir -p $(GRAFANA_DATA)

certstage: $(STAGE_CERT_FILE) $(STAGE_KEY_FILE)

$(STAGE_CERT_DIR):
	mkdir -p $(STAGE_CERT_DIR)

$(STAGE_CERT_FILE) $(STAGE_KEY_FILE): | $(STAGE_CERT_DIR)
	@command -v mkcert >/dev/null 2>&1 || \
		{ echo "mkcert not found. Install mkcert first."; exit 1; }
	@set -a && . $(STAGE_ENV) && set +a && \
	test -n "$$HOST" || { echo "HOST missing in $(STAGE_ENV)"; exit 1; }; \
	test -f $(STAGE_CERT_FILE) -a -f $(STAGE_KEY_FILE) || \
	mkcert -cert-file $(STAGE_CERT_FILE) -key-file $(STAGE_KEY_FILE) \
		$$HOST localhost 127.0.0.1 ::1

load:
	@command -v $(K6) >/dev/null 2>&1 || { \
		echo "k6 not found. Install from https://k6.io"; exit 1; }
	@echo "Running load test using $(LOAD_ENV)"
	@set -a; [ -f $(LOAD_ENV) ] && . $(LOAD_ENV); set +a; \
	$(K6) run \
		-e HOST=$$HOST \
		-e PORT=$$PORT \
		-e CSRF_DEV_BYPASS=$$CSRF_DEV_BYPASS \
		$(LOAD_SCRIPT)
