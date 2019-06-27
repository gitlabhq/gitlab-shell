.PHONY: test test_ruby test_ruby_rubocop test_ruby_rspec test_go test_go_format test_go_test

validate: verify test

verify: verify_ruby verify_golang

verify_ruby:
	bundle exec rubocop

verify_golang:
	support/go-format check

test: test_ruby test_golang

test_ruby:
	# bin/gitlab-shell must exist and needs to be the Ruby version for
	# rspec to be able to test.
	cp bin/gitlab-shell-ruby bin/gitlab-shell
	bundle exec rspec --color --tag '~go' --format d spec
	rm -f bin/gitlab-shell

test_golang:
	support/go-test

setup: compile
build: compile
compile:
	bin/install
	bin/compile

check:
	bin/check

clean:
	rm -f bin/gitlab-shell
