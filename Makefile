BIN := pointcloud
PKG := ./...
CMD := .

.PHONY: all build run test vet fmt tidy clean

all: build

build:
	fyne build -o bin/$(BIN) $(CMD)

run:
	go run $(CMD)

test:
	go test $(PKG)

vet:
	go vet $(PKG)

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

clean:
	rm -rf bin
