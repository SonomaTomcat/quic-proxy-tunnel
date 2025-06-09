
BINARY=qp-tunnel
GO=go

.PHONY: all build clean test

all: build

build:
	$(GO) build -o $(BINARY) main.go

clean:
	rm -f $(BINARY)

run-client:
	$(GO) run main.go client --config test/client.json

run-server:
	$(GO) run main.go server --config test/server.json

test:
	$(GO) test ./...

