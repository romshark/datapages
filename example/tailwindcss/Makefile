CSS_INPUT  := ./input.tw.css
CSS_OUTPUT := ./app/static/output.css
SERVER_BIN := ./server

.PHONY: dev build css gen check-gen lint npm-install

dev: css
	datapages watch

build: css
	go build -o $(SERVER_BIN) ./cmd/server/

node_modules:
	npm install

# Rebuild output.css from input.css using TailwindCSS v4.
css: node_modules
	npx tailwindcss -i $(CSS_INPUT) -o $(CSS_OUTPUT)

gen: css
	go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate
	datapages gen

# Fail if running all generators produces a diff (generated files out of date).
# In GitHub Actions, run "make gen" in one step and "git diff --exit-code"
# in the next to catch stale generated files in CI.
check-gen: gen
	@git diff --exit-code || { \
		echo "Generated files are out of date. Run 'make gen' and commit the changes."; \
		exit 1; \
	}

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...
