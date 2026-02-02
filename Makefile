test: lint
	go test ./... -cover

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...

mod-tidy-parser-tests:
	@find parser/testdata -name go.mod -not -path './vendor/*' | \
	while read mod; do \
		dir=$$(dirname "$$mod"); \
		echo "==> go mod tidy in $$dir"; \
		( cd "$$dir" && go mod tidy ); \
	done
