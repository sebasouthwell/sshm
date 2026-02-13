.PHONY: build install test clean run

BINARY_NAME=sshm
MAIN_PATH=cmd/sshm/main.go

build:
	go build -o $(BINARY_NAME) $(MAIN_PATH)

install: build
	sudo mv $(BINARY_NAME) /usr/local/bin/

test:
	go test ./...

test-verbose:
	go test -v ./...

clean:
	rm -f $(BINARY_NAME)
	go clean

run:
	go run $(MAIN_PATH)

deps:
	go mod download
	go mod tidy
