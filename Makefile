BINARY_NAME=videoflow-server

.PHONY: all build run test clean tidy build-all

all: build

build:
	go build -o bin/$(BINARY_NAME) ./cmd/server

run:
	go run ./cmd/server

test:
	go test -v ./...

tidy:
	go mod tidy

clean:
	rm -rf bin/

# Cross-compilation for all desktop OS targets
build-all:
	GOOS=linux GOARCH=amd64 go build -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/server
	GOOS=linux GOARCH=arm64 go build -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/server
	GOOS=darwin GOARCH=arm64 go build -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/server
	GOOS=darwin GOARCH=amd64 go build -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/server
	GOOS=windows GOARCH=amd64 go build -o bin/$(BINARY_NAME)-windows-amd64.exe ./cmd/server

.PHONY: cloud cloud-worker cloud-admin cloud-test cloud-contract cloud-build cloud-dev
cloud:
	go run ./cmd/cloud

cloud-worker:
	go run ./cmd/cloud-worker

cloud-admin:
	go run ./cmd/cloud-admin $(ARGS)

cloud-test:
	go test -race ./internal/cloud ./cmd/cloud ./cmd/cloud-worker ./cmd/cloud-admin

cloud-contract:
	python3 scripts/generate-cloud-contract.py

cloud-build:
	go build -o bin/videoflow-cloud ./cmd/cloud
	go build -o bin/videoflow-cloud-worker ./cmd/cloud-worker
	go build -o bin/videoflow-cloud-admin ./cmd/cloud-admin

cloud-dev:
	docker compose --env-file .env.cloud -f compose.cloud.yml --profile app up --build
