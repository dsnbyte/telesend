.PHONY: all build test check clean release

all: build

build:
	mkdir -p dist
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/telesend ./cmd/telesend

test:
	go test -v ./...

check:
	go test ./...
	go vet ./...

release:
	@./scripts/release.sh $(VERSION)

clean:
	rm -f dist/telesend
