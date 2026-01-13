NATS_CONTAINER_NAME := nats-datapages-demo
NATS_PORT := 4222
NATS_HTTP_PORT := 8222
NATS_IMAGE := nats:latest

dev: nats-up
	go run github.com/romshark/templier@latest

test: lint
	go test ./... -v -race

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...

nats-up:
	@docker inspect -f '{{.State.Running}}' $(NATS_CONTAINER_NAME) 2>/dev/null | grep -q true || \
	docker run -d --rm \
		--name $(NATS_CONTAINER_NAME) \
		-p $(NATS_PORT):4222 \
		-p $(NATS_HTTP_PORT):8222 \
		$(NATS_IMAGE) \
		-js

nats-stop:
	@docker stop $(NATS_CONTAINER_NAME) 2>/dev/null || true

nats-reset: nats-stop nats-up