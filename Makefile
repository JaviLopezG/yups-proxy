.PHONY: all build run test test-raw check-proxies docker-build docker-run clean help

all: build test

help:
	@echo "Available make targets:"
	@echo "  build         Compile the yups binary"
	@echo "  run           Run yups locally using go run"
	@echo "  test          Run tests with formatted, readable output"
	@echo "  test-raw      Run standard go test -v ./..."
	@echo "  check-proxies Verify proxy availability and update proxies.csv"
	@echo "  docker-build  Build the Docker container image"
	@echo "  docker-run    Run the Docker container on port 8080"
	@echo "  clean         Remove compiled binaries"

build:
	go build -trimpath -ldflags="-s -w" -o yups ./cmd/yups

run:
	go run ./cmd/yups

test:
	./scripts/test-runner.sh

test-raw:
	go test -v ./...

check-proxies:
	./scripts/check-proxies.py

docker-build:
	docker build -t yups:latest .

docker-run:
	docker run --rm -it -p 8080:8080 -e YUPS_ACCESS_LOG=true yups:latest

clean:
	rm -f yups
