dev:
	go run github.com/romshark/templier@latest

test: lint
	go test ./... -v -race

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...
