BINARY  := agent-tonbrowser
CMD     := ./cmd/agent-tonbrowser
BINDIR  := bin
VERSION := dev

.PHONY: build clean install test vet fmt lint

build:
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINDIR)/$(BINARY) $(CMD)

clean:
	rm -rf $(BINDIR)

install:
	go install -trimpath -ldflags "-s -w -X main.version=$(VERSION)" $(CMD)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

lint:
	golangci-lint run ./...
