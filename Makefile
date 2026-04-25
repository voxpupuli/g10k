DEPS = $(wildcard */*.go)
BUILDVERSION = $(shell git describe --tags)
BUILDTIME = $(shell date -u '+%Y-%m-%d_%H:%M:%S')
UNAME := $(shell uname)

GO           ?= go
FIRST_GOPATH := $(firstword $(subst :, ,$(shell $(GO) env GOPATH)))
SKIP_GOLANGCI_LINT :=
GOLANGCI_LINT :=
GOLANGCI_LINT_OPTS ?=
GOLANGCI_LINT_VERSION ?= v2.11.4
GOLANGCI_FMT_OPTS ?=
# golangci-lint only supports linux, darwin and windows platforms on i386/amd64/arm64.
# windows isn't included here because of the path separator being different.
ifeq ($(GOHOSTOS),$(filter $(GOHOSTOS),linux darwin))
	ifeq ($(GOHOSTARCH),$(filter $(GOHOSTARCH),amd64 i386 arm64))
		# If we're in CI and there is an Actions file, that means the linter
		# is being run in Actions, so we don't need to run it here.
		ifneq (,$(SKIP_GOLANGCI_LINT))
			GOLANGCI_LINT :=
		else ifeq (,$(CIRCLE_JOB))
			GOLANGCI_LINT := $(FIRST_GOPATH)/bin/golangci-lint
		else ifeq (,$(wildcard .github/workflows/golangci-lint.yml))
			GOLANGCI_LINT := $(FIRST_GOPATH)/bin/golangci-lint
		endif
	endif
endif

all: test g10k

g10k: g10k.go $(DEPS)
# -race flag is currently removed because of issues in OS X Monterey. Should be solved above go version 1.17.6
ifeq ($(UNAME), Darwin)
	CGO_ENABLED=1 GOOS=darwin go build \
		-ldflags "-s -w -X main.buildversion=${BUILDVERSION} -X main.buildtime=${BUILDTIME}" \
	-o $@
	strip -X $@
endif
ifeq ($(UNAME), Linux)
	CGO_ENABLED=1 GOOS=linux go build \
		-race -ldflags "-s -w -X main.buildversion=${BUILDVERSION} -X main.buildtime=${BUILDTIME}" \
	-o $@
	strip $@
endif

lint: $(GOLANGCI_LINT)
ifdef GOLANGCI_LINT
	@echo ">> running golangci-lint"
	$(GOLANGCI_LINT) run $(GOLANGCI_LINT_OPTS) $(pkgs)
endif

lint-fix: $(GOLANGCI_LINT)
ifdef GOLANGCI_LINT
	@echo ">> running golangci-lint fix"
	$(GOLANGCI_LINT) run --fix $(GOLANGCI_LINT_OPTS) $(pkgs)
endif

ifdef GOLANGCI_LINT
$(GOLANGCI_LINT):
	mkdir -p $(FIRST_GOPATH)/bin
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh \
		| sed -e '/install -d/d' \
		| sh -s -- -b $(FIRST_GOPATH)/bin $(GOLANGCI_LINT_VERSION)
endif

vet: g10k.go
	go vet

imports: g10k.go
	go install golang.org/x/tools/cmd/goimports@latest && \
	goimports -d *.go tests/

test: lint vet imports
# This is a workaround for Bug https://github.com/golang/go/issues/49138
ifeq ($(UNAME), Darwin)
	MallocNanoZone=0 go test -race -coverprofile=coverage.txt -covermode=atomic -v
endif
ifeq ($(UNAME), Linux)
	go test -race -coverprofile=coverage.txt -covermode=atomic -v ./...
endif

clean:
	rm -rf g10k coverage.txt cache example

build-image:
	docker build -t g10k:${BUILDVERSION} .

update-deps:
	go get -u
	go mod vendor

.PHONY: all lint vet imports test clean
