.PHONY: build run test test-unit lint up down

build:
	go build ./...

run:
	go run ./cmd/worker/

test:
	go test ./...

test-unit:
	go test -short ./...

lint:
	go vet ./...

up:
	docker-compose up -d

down:
	docker-compose down
