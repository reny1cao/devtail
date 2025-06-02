.PHONY: build run test clean install-deps proto

build: proto
	go build -o bin/gateway cmd/gateway/main.go
	go build -o bin/test-client cmd/test-client/main.go
	go build -o bin/test-terminal cmd/test-terminal/main.go

run: build
	./bin/gateway --log-level debug

test-client: build
	./bin/test-client

test-terminal: build
	./bin/test-terminal

install-deps:
	go mod download
	go mod tidy

clean:
	rm -rf bin/

test:
	go test -v ./...

docker-build:
	docker build -t devtail-gateway .

.DEFAULT_GOAL := build

# Include proto makefile
include Makefile.proto