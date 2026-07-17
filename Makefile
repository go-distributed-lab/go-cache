.PHONY: all build test race bench lint vet tidy clean

all: build

build:
	go build ./...

test:
	go test ./...

race:
	go test -race ./...

bench:
	cd benchmarks && go test -bench=. -benchmem -benchtime=3s .

lint: vet
	staticcheck ./...

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	go clean ./...

docker-build:
	docker build -t go-cache:latest .

docker-lru:
	docker compose --profile lru up --build

docker-lfu:
	docker compose --profile lfu up --build

docker-fifo:
	docker compose --profile fifo up --build

docker-ttl:
	docker compose --profile ttl up --build

docker-sharded:
	docker compose --profile sharded up --build

docker-down:
	docker compose down