all: test lint build

build:
	go build

# Parallel execution is only supported with recipe's dependencies:
# https://github.com/casey/just/issues/626
[parallel]
lint: mod-tidy golangci-lint

[private]
mod-tidy:
	go mod tidy -diff

[private]
golangci-lint:
	golangci-lint run

test:
	go test ./...

audit:
	go mod verify
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

format:
	golangci-lint fmt

man-pages: build
	mkdir -p man-pages
	./w2a man man-pages

completions: build
	mkdir -p completions
	./w2a completion bash > completions/w2a.bash
	./w2a completion zsh > completions/w2a.zsh
	./w2a completion fish > completions/w2a.fish