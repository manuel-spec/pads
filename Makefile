.PHONY: build test test-race fmt lint clean

BINARY := pads

build:
	go build -o $(BINARY) .

test:
	go test ./...

test-race:
	go test -race ./...

fmt:
	gofmt -w .
	goimports -w .

clean:
	rm -f $(BINARY) $(BINARY).exe
