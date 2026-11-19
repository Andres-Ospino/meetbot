.PHONY: all build test run-server docker-build docker-up clean

all: build

build:
	go build -o bin/meetbot ./cmd/meetbot
	go build -o bin/meetbot-server ./cmd/server

test:
	go test -v ./...

run-server:
	go run ./cmd/server

docker-build:
	docker build -t meetbot:latest -f docker/Dockerfile .

docker-up:
	docker compose -f docker/docker-compose.yml up -d

clean:
	rm -rf bin/ recordings/
