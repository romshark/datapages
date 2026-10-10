SERVER_BIN := ./server

.PHONY: dev build bundle gen check-gen lint test npm-install

npm-install:
	cd web && npm install

# Bundle the editor with esbuild.
bundle:
	cd web && node build.mjs

dev: bundle
	datapages watch

build: bundle
	go build -o $(SERVER_BIN) ./cmd/server/

gen: bundle
	go run github.com/a-h/templ/cmd/templ@v0.3.1070 generate
	datapages gen

# Fail if generated files are out of date.
check-gen: gen
	@git diff --exit-code || { \
		echo "Generated files are out of date. Run 'make gen' and commit the changes."; \
		exit 1; \
	}

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...

test:
	go test ./...
	cd web && npm test
