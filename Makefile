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