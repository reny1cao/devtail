.PHONY: build run test clean migrate docker-build

build:
	go build -o bin/control-plane cmd/control-plane/main.go

run: build
	./bin/control-plane --log-level debug

test:
	go test -v ./...

clean:
	rm -rf bin/

migrate:
	psql -U devtail -d devtail -f migrations/001_create_vms.sql

docker-build:
	docker build -t devtail-control-plane .

install-deps:
	go mod download
	go mod tidy

.DEFAULT_GOAL := build