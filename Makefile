.PHONY: validate verify verify_ruby verify_golang test test_ruby test_golang coverage coverage_golang setup _install build compile check clean

GO_SOURCES := $(shell find . -name '*.go')
VERSION_STRING := $(shell git describe --match v* 2>/dev/null || cat VERSION 2>/dev/null || echo unknown)
BUILD_TIME := $(shell date -u +%Y%m%d.%H%M%S)
GOBUILD_FLAGS := -ldflags "-X main.Version=$(VERSION_STRING) -X main.BuildTime=$(BUILD_TIME)"

validate: verify test

verify: verify_ruby verify_golang

verify_ruby:
	bundle exec rubocop

verify_golang:
	gofmt -s -l $(GO_SOURCES)

test: test_ruby test_golang

# The Ruby tests are now all integration specs that test the Go implementation.
test_ruby:
	bundle exec rspec --color --format d spec

test_golang:
	go test -cover -coverprofile=cover.out ./...

test_golang_race:
	go test -race ./...

coverage: coverage_golang

coverage_golang:
	[ -f cover.out ] && go tool cover -func cover.out

setup: _install bin/gitlab-shell

_install:
	bin/install

build: bin/gitlab-shell
compile: bin/gitlab-shell
bin/gitlab-shell: $(GO_SOURCES)
	GOBIN="$(CURDIR)/bin" go install $(GOBUILD_FLAGS) ./cmd/...

check:
	bin/check

clean:
	rm -f bin/check bin/gitlab-shell bin/gitlab-shell-authorized-keys-check bin/gitlab-shell-authorized-principals-check
