.PHONY: test test_ruby test_ruby_rubocop test_ruby_rspec test_go test_go_format test_go_test

test: test_ruby test_go

test_ruby: test_ruby_rubocop test_ruby_rspec

test_ruby_rubocop:
	bundle exec rubocop

test_ruby_rspec:
	cp bin/gitlab-shell-ruby bin/gitlab-shell
	bundle exec rspec --color --tag ~go spec

test_go: test_go_format test_go_test

test_go_format:
	support/go-format check

test_go_test:
	support/go-test

build: compile
compile:
	bin/compile
