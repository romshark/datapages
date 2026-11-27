SERVER_BIN := ./server

.PHONY: dev build gen check-gen lint

dev:
	datapages watch

build:
	go build -o $(SERVER_BIN) ./cmd/server/

gen:
	go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate
	datapages gen

check-gen: gen
	@git diff --exit-code || { \
		echo "Generated files are out of date. Run 'make gen' and commit the changes."; \
		exit 1; \
	}

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...
