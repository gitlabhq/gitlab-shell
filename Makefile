.PHONY: validate verify verify_ruby verify_golang test test_ruby test_golang test_fancy test_golang_fancy coverage coverage_golang setup _script_install make_necessary_dirs build compile check clean install lint

FIPS_MODE ?= 0
OS := $(shell uname | tr A-Z a-z)
GO_SOURCES := $(shell git ls-files \*.go)
VERSION_STRING := $(shell git describe --match v* 2>/dev/null || awk '$$0="v"$$0' VERSION 2>/dev/null || echo unknown)
BUILD_TIME := $(shell date -u +%Y%m%d.%H%M%S)
GO_TAGS := tracer_static tracer_static_jaeger continuous_profiler_stackdriver

ARCH ?= $(shell uname -m | sed -e 's/x86_64/amd64/' | sed -e 's/aarch64/arm64/')

GOTESTSUM_VERSION := 1.11.0
GOTESTSUM_FILE := support/bin/gotestsum-${GOTESTSUM_VERSION}

GOLANGCI_LINT_VERSION := 1.60.1
GOLANGCI_LINT_FILE := support/bin/golangci-lint-${GOLANGCI_LINT_VERSION}

export GOFLAGS := -mod=readonly

ifeq (${FIPS_MODE}, 1)
    GO_TAGS += fips

    # If the golang-fips compiler is built with CGO_ENABLED=0, this needs to be
    # explicitly switched on.
    export CGO_ENABLED=1

    # Go 1.19 now requires GOEXPERIMENT=boringcrypto for FIPS compilation.
    # See https://github.com/golang/go/issues/51940 for more details.
    BORINGCRYPTO_SUPPORT := $(shell GOEXPERIMENT=boringcrypto go version > /dev/null 2>&1; echo $$?)
    ifeq ($(BORINGCRYPTO_SUPPORT), 0)
        export GOEXPERIMENT=boringcrypto
    endif
endif

ifneq (${CGO_ENABLED}, 0)
	GO_TAGS += gssapi
endif

ifeq (${OS}, darwin) # Mac OS
    BREW_PREFIX := $(shell brew --prefix 2>/dev/null || echo "/opt/homebrew")

    # To be able to compile gssapi library
    export CGO_CFLAGS="-I$(BREW_PREFIX)/opt/heimdal/include"
endif

GOBUILD_FLAGS := -ldflags "-X main.Version=$(VERSION_STRING) -X main.BuildTime=$(BUILD_TIME)" -tags "$(GO_TAGS)" -mod=mod

PREFIX ?= /usr/local

build: compile

validate: verify test

verify: verify_golang

verify_golang:
	gofmt -s -l $(GO_SOURCES) | awk '{ print } END { if (NR > 0) { print "Please run make fmt"; exit 1 } }'

fmt:
	gofmt -w -s $(GO_SOURCES)

test: test_ruby test_golang

test_fancy: test_ruby test_golang_fancy

# The Ruby tests are now all integration specs that test the Go implementation.
test_ruby:
	bundle exec rspec --color --format d spec

test_golang:
	go test -cover -coverprofile=cover.out -count 1 -tags "$(GO_TAGS)" ./...

test_golang_fancy: ${GOTESTSUM_FILE}
	@${GOTESTSUM_FILE} --version
	@${GOTESTSUM_FILE} --junitfile ./cover.xml --format pkgname -- -coverprofile=./cover.out -covermode=atomic -count 1 -tags "$(GO_TAGS)" ./...

${GOTESTSUM_FILE}:
	mkdir -p $(shell dirname ${GOTESTSUM_FILE})
	curl -L https://github.com/gotestyourself/gotestsum/releases/download/v${GOTESTSUM_VERSION}/gotestsum_${GOTESTSUM_VERSION}_${OS}_${ARCH}.tar.gz | tar -zOxf - gotestsum > ${GOTESTSUM_FILE} && chmod +x ${GOTESTSUM_FILE}

test_golang_race:
	go test -race -count 1 ./...

coverage: coverage_golang

coverage_golang:
	[ -f cover.out ] && go tool cover -func cover.out

lint: ${GOLANGCI_LINT_FILE}
	${GOLANGCI_LINT_FILE} --version
	${GOLANGCI_LINT_FILE} run --issues-exit-code 0 --print-issued-lines=false ${GOLANGCI_LINT_ARGS}

${GOLANGCI_LINT_FILE}:
	mkdir -p $(shell dirname ${GOLANGCI_LINT_FILE})
	curl -L https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_LINT_VERSION}/golangci-lint-${GOLANGCI_LINT_VERSION}-${OS}-${ARCH}.tar.gz | tar --strip-components 1 -zOxf - golangci-lint-${GOLANGCI_LINT_VERSION}-${OS}-${ARCH}/golangci-lint > ${GOLANGCI_LINT_FILE} && chmod +x ${GOLANGCI_LINT_FILE}

setup: make_necessary_dirs bin/gitlab-shell

make_necessary_dirs: _script_install
_script_install:
	bin/install

compile: bin/gitlab-shell bin/gitlab-sshd

bin/gitlab-shell: $(GO_SOURCES)
	go build $(GOBUILD_FLAGS) -o $(CURDIR)/bin ./cmd/...

bin/gitlab-sshd: $(GO_SOURCES)
	go build $(GOBUILD_FLAGS) -o $(CURDIR)/bin/gitlab-sshd ./cmd/gitlab-sshd

check:
	bin/check

clean:
	rm -f bin/check bin/gitlab-shell bin/gitlab-shell-authorized-keys-check bin/gitlab-shell-authorized-principals-check bin/gitlab-sshd

install: compile
	mkdir -p $(DESTDIR)$(PREFIX)/bin/
	install -m755 bin/check $(DESTDIR)$(PREFIX)/bin/check
	install -m755 bin/gitlab-shell $(DESTDIR)$(PREFIX)/bin/gitlab-shell
	install -m755 bin/gitlab-shell-authorized-keys-check $(DESTDIR)$(PREFIX)/bin/gitlab-shell-authorized-keys-check
	install -m755 bin/gitlab-shell-authorized-principals-check $(DESTDIR)$(PREFIX)/bin/gitlab-shell-authorized-principals-check
	install -m755 bin/gitlab-sshd $(DESTDIR)$(PREFIX)/bin/gitlab-sshd
