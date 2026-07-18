.PHONY: build run debug test test-one lint fmt tidy clean release-check release-dry-run

build:
	go build -o ./build/ktails ./cmd/page-client

run:
	go run ./cmd/page-client

debug:
	KTAILS_DEBUG=1 go run ./cmd/page-client

test:
	go test ./...

test-one:
	@if [ -z "$(pkg)" ]; then \
		echo "Usage: make test-one pkg=./path [name=TestName]"; \
		exit 1; \
	fi
	@if [ -z "$(name)" ]; then \
		go test $(pkg); \
	else \
		go test $(pkg) -run $(name); \
	fi

lint:
	go vet ./...
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; fi

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -f ktails
	rm -rf build dist

# Validates .goreleaser.yaml without building anything.
release-check:
	goreleaser check

# Builds all release targets locally (snapshot version, no publishing) into ./dist.
release-dry-run:
	goreleaser release --snapshot --clean --skip=publish
