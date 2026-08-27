.PHONY: build run test clean docker-build docker-run

build:
	go build -ldflags="-s -w" -o bin/vilicus ./cmd/bot

run:
	go run ./cmd/bot

test:
	go test -v ./...

clean:
	rm -rf bin/ data/

docker-build:
	docker build -t vilicus .

docker-run:
	docker run -d --name vilicus -p 8080:8080 -v ./data:/app/data --env-file .env vilicus
