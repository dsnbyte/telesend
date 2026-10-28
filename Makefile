.PHONY: all build test check clean

all: build

build:
	mkdir -p dist
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/telesend ./cmd/telesend

test:
	go test -v ./...

check:
	go test ./...
	go vet ./...

clean:
	rm -f dist/telesend
