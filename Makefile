.PHONY: build install clean test

BINARY   := bonsai
PREFIX   ?= /usr/local

build:
	go build -ldflags="-s -w" -o $(BINARY) .

install: build
	install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)

uninstall:
	rm -f $(PREFIX)/bin/$(BINARY)

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -f $(BINARY)
