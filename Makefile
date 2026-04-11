BINARIES := $(notdir $(shell find cmd -mindepth 1 -maxdepth 1 -type d))

.PHONY: $(BINARIES)
.PHONY: all 
.PHONY: build 
.PHONY: run 
.PHONY: test 
.PHONY: bench 
.PHONY: benchstat 
.PHONY: vet 
.PHONY: lint 
.PHONY: revive 
.PHONY: staticcheck 
.PHONY: fmt 
.PHONY: tidy 
.PHONY: clean

all: lint vet staticcheck test build

build: $(BINARIES)

$(BINARIES):
	@echo "*** $@"
	@cd cmd/$@ && go build $(LDFLAGS) -trimpath -o ../../bin/$@

run:
	@echo "*** $@"
	@go run cmd/

test:
	@echo "*** $@"
	@go test ./...

bench:
	@echo "*** $@"
	@go test -bench=. -benchmem -count=6 ./... | tee bench/new.txt

benchstat:
	@echo "*** $@"
	@benchstat bench/old.txt bench/new.txt

vet:
	@echo "*** $@"
	@go vet ./...

lint:
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
