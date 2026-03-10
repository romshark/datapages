all: test vulncheck fmt mod-tidy gen-templ gen-docs

test: lint
	go test ./... -cover

fmt:
	go run mvdan.cc/gofumpt@latest -w .
	go run github.com/daixiang0/gci@latest write \
		--skip-generated \
		-s standard \
		-s default \
		-s "prefix(github.com/romshark/datapages)" .

# Verify all go.mod/go.sum files in the repo are tidy.
# For each module: back up, tidy, diff, and restore on mismatch.
check-mod:
	@fail=0; \
	find . -name go.mod -not -path './vendor/*' | \
	while read mod; do \
		dir=$$(dirname "$$mod"); \
		cp "$$dir/go.mod" "$$dir/go.mod.tmp"; \
		cp "$$dir/go.sum" "$$dir/go.sum.tmp" 2>/dev/null; \
		( cd "$$dir" && go mod tidy ); \
		if ! diff -q "$$dir/go.mod" "$$dir/go.mod.tmp" \
			> /dev/null 2>&1 || \
			! diff -q "$$dir/go.sum" "$$dir/go.sum.tmp" \
			> /dev/null 2>&1; then \
			echo "go.mod not tidy in $$dir"; \
			mv "$$dir/go.mod.tmp" "$$dir/go.mod"; \
			mv "$$dir/go.sum.tmp" "$$dir/go.sum" \
				2>/dev/null; \
			fail=1; \
		else \
			rm -f "$$dir/go.mod.tmp" "$$dir/go.sum.tmp"; \
		fi; \
	done; \
	test "$$fail" = 0

check-fmt:
	@bad=$$(go run mvdan.cc/gofumpt@latest -l .); \
	bad="$$bad$$(go run github.com/daixiang0/gci@latest list \
		--skip-generated \
		-s standard \
		-s default \
		-s "prefix(github.com/romshark/datapages)" .)"; \
	test -z "$$bad" || { \
		echo "files not formatted (run make fmt):"; \
		echo "$$bad"; \
		exit 1; \
	}

lint: check-fmt check-mod lint-datapages
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...
	@find example -name go.mod -not -path '*/vendor/*' | \
	while read mod; do \
		dir=$$(dirname "$$mod"); \
		echo "==> golangci-lint in $$dir"; \
		( cd "$$dir" && go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./... ) || exit 1; \
	done

lint-datapages:
	@bin=$$(mktemp -d)/datapages; \
	go build -o "$$bin" . && \
	trap 'rm -f "$$bin"' EXIT; \
	find example -name go.mod -not -path '*/vendor/*' | \
	while read mod; do \
		dir=$$(dirname "$$mod"); \
		echo "==> datapages lint in $$dir"; \
		( cd "$$dir" && "$$bin" lint ) || exit 1; \
	done

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	@find example -name go.mod -not -path '*/vendor/*' | \
	while read mod; do \
		dir=$$(dirname "$$mod"); \
		echo "==> govulncheck in $$dir"; \
		( cd "$$dir" && go run golang.org/x/vuln/cmd/govulncheck@latest ./... ) || exit 1; \
	done

mod-update: mod-update-examples mod-update-parser-tests
	go get -u -t ./...

mod-update-examples:
	@find example -name go.mod -not -path '*/vendor/*' | \
	while read mod; do \
		dir=$$(dirname "$$mod"); \
		echo "==> go get -u -t ./... in $$dir"; \
		( cd "$$dir" && go get -u -t ./... ); \
	done

mod-update-parser-tests:
	@find parser/testdata -name go.mod -not -path '*/vendor/*' | \
	while read mod; do \
		dir=$$(dirname "$$mod"); \
		echo "==> go get -u -t ./... in $$dir"; \
		( cd "$$dir" && go get -u -t ./... ); \
	done

mod-tidy: mod-tidy-examples mod-tidy-parser-tests
	go mod tidy

mod-tidy-examples:
	@find example -name go.mod -not -path '*/vendor/*' | \
	while read mod; do \
		dir=$$(dirname "$$mod"); \
		echo "==> go mod tidy in $$dir"; \
		( cd "$$dir" && go mod tidy ); \
	done

mod-tidy-parser-tests:
	@find parser/testdata -name go.mod -not -path '*/vendor/*' | \
	while read mod; do \
		dir=$$(dirname "$$mod"); \
		echo "==> go mod tidy in $$dir"; \
		( cd "$$dir" && go mod tidy ); \
	done

gen-templ: gen-templ-examples gen-templ-parser-tests

gen-templ-examples:
	@find example -name go.mod -not -path '*/vendor/*' | \
	while read mod; do \
		dir=$$(dirname "$$mod"); \
		echo "==> templ generate in $$dir"; \
		( cd "$$dir" && go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate ); \
	done

gen-templ-parser-tests:
	@find parser/testdata -name go.mod -not -path '*/vendor/*' | \
	while read mod; do \
		dir=$$(dirname "$$mod"); \
		echo "==> templ generate in $$dir"; \
		( cd "$$dir" && go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate ); \
	done

gen-docs:
	@version="$$(git describe --tags --abbrev=0 2>/dev/null || echo latest)"; \
	go run github.com/a-h/templ/cmd/templ@v0.3.1001 generate -path ./docs-src && \
	go run ./scripts/render-pages -version "$$version"
