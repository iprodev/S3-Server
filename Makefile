.PHONY: all build test lint clean release

BINARY=s3_server
MAIN=./cmd/s3_server

all: build

build:
	go build -o $(BINARY) $(MAIN)

test:
	go test -race ./...

lint:
	go vet ./...
	go fmt ./...

clean:
	rm -f $(BINARY)
	rm -rf dist/

release:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/$(BINARY)-linux-amd64 $(MAIN)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/$(BINARY)-linux-arm64 $(MAIN)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/$(BINARY)-darwin-amd64 $(MAIN)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/$(BINARY)-darwin-arm64 $(MAIN)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/$(BINARY)-windows-amd64.exe $(MAIN)
