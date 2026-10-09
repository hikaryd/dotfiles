.DEFAULT_GOAL := test
.PHONY: test test-go test-node test-runner test-list

test:
	./scripts/test.sh all

test-go:
	./scripts/test.sh go

test-node:
	./scripts/test.sh node

test-runner:
	./scripts/test.sh runner

test-list:
	./scripts/test.sh --list
