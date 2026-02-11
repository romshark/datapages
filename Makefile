test: lint
	go test ./... -cover

fmt:
	go run mvdan.cc/gofumpt@latest -w .

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
	@test -z "$$(go run mvdan.cc/gofumpt@latest -l .)" || { \
		echo "files not formatted with gofumpt:"; \
		go run mvdan.cc/gofumpt@latest -l .; \
		exit 1; \
	}

lint: check-fmt check-mod
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...
	(cd example/classifieds/; go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...)

mod-tidy: mod-tidy-parser-tests
	go mod tidy

mod-tidy-parser-tests:
	@find parser/testdata -name go.mod -not -path './vendor/*' | \
	while read mod; do \
		dir=$$(dirname "$$mod"); \
		echo "==> go mod tidy in $$dir"; \
		( cd "$$dir" && go mod tidy ); \
	done
