BIN := cutejaguar

GOPATH := $(shell go env GOPATH)

ifeq ($(OS),Windows_NT)
    GO_FILES := $(shell dir /S /B *.go)
    GO_DEPS := $(shell dir /S /B go.mod go.sum)
    CLEAN := del
else
    GO_FILES := $(shell find . -name '*.go')
    GO_DEPS := $(shell find . -name go.mod -o -name go.sum)
    CLEAN := rm -f
endif

$(BIN): $(GO_FILES) $(GO_DEPS)
	go mod tidy
	gofumpt -w $(GO_FILES)
	golangci-lint run
	go build -o $(BIN) cmd/main.go

.PHONY: test
test: $(BIN)
	./$(BIN) -v -v

.PHONY: install
install: $(BIN)
	mv $(BIN) $(GOPATH)/bin/$(BIN)

.PHONY: clean
clean:
	$(CLEAN) $(BIN)
