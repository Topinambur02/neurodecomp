GOLANGCI_LINT_VERSION ?= 2.14.0
GO_BIN := $(shell go env GOBIN)
ifeq ($(GO_BIN),)
GO_BIN := $(firstword $(subst :, ,$(shell go env GOPATH)))/bin
endif
LINTER := $(GO_BIN)/golangci-lint

.PHONY: install lint fmt

install:
	@$(LINTER) version 2>/dev/null | grep -q "version $(GOLANGCI_LINT_VERSION) " || \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION)

lint: install
	$(LINTER) run ./...

fmt: install
	$(LINTER) fmt ./...
