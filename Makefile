.PHONY: build run test lint clean

APP_NAME := ServerPshhh
BUILD_DIR := bin

build:
	go build -o $(BUILD_DIR)/$(APP_NAME).exe .

run:
	go run .

test:
	go test -v -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

lint:
	golangci-lint run

clean:
	del /Q $(BUILD_DIR)\* 2>nul
	del coverage.out 2>nul

tidy:
	go mod tidy
	go fmt ./...
