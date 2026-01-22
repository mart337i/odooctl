.PHONY: build build-all clean test install

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/egeskov/odooctl/cmd.version=$(VERSION)"

# Build for current platform
build:
	go build $(LDFLAGS) -o bin/odooctl .

# Build for all platforms
build-all: clean
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/odooctl-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/odooctl-linux-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/odooctl-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/odooctl-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/odooctl-windows-amd64.exe .

# Clean build artifacts
clean:
	rm -rf bin/

# Run tests
test:
	go test -v ./...

# Install to GOPATH/bin
install:
	go install $(LDFLAGS) .

# Development: build and run
run: build
	./bin/odooctl $(ARGS)
