BIN := pointcloud
PKG := ./...
CMD := .

.PHONY: all build run test vet lint revive staticcheck fmt tidy clean

all: lint build

build:
	@echo "*** $@"
	@fyne build -o bin/$(BIN) $(CMD)

run:
	@echo "*** $@"
	@go run $(CMD)

test:
	@echo "*** $@"
	@go test $(PKG)

vet:
	@echo "*** $@"
	@go vet $(PKG)

lint: vet revive staticcheck

revive:
	@echo "*** $@"
	@revive ./...

staticcheck:
	@staticcheck ./...

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

clean:
	rm -rf bin
