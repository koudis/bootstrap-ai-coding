BINARY   := bac
MODULE   := github.com/koudis/bootstrap-ai-coding
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -ldflags "-s -w -X '$(MODULE)/internal/constants.Version=$(VERSION)' -extldflags '-static'"

.PHONY: release test test-integration vet clean

## release: build static binaries for linux/amd64 and linux/arm64
release:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)-linux-amd64 .
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-linux-arm64 .

## test: run unit and property-based tests
test:
	go test ./...

## test-integration: run integration tests (requires a running Docker daemon)
test-integration:
	BAC_INTEGRATION_CONSENT=yes go test -tags integration -timeout 30m -p 1 ./...

## vet: run go vet
vet:
	go vet ./...

## clean: remove compiled binaries
clean:
	rm -f $(BINARY)-linux-amd64 $(BINARY)-linux-arm64
