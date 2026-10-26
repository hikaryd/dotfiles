.DEFAULT_GOAL := test
.PHONY: test test-go test-integration test-node test-runner test-list test-voice

test:
	./scripts/test.sh all
	$(MAKE) test-voice

test-voice:
	PYTHONDONTWRITEBYTECODE=1 "$${PYTHON_BIN:-python3}" -m unittest discover -s config/jcode -p test_agent_voice.py -v

test-go:
	./scripts/test.sh go

test-integration:
	GOTOOLCHAIN=local GORACE="$${GORACE:-atexit_sleep_ms=0}" "$${GO_BIN:-go}" -C tools/dots test -race -vet=all -count=1 ./tests/integration

test-node:
	./scripts/test.sh node

test-runner:
	./scripts/test.sh runner

test-list:
	./scripts/test.sh --list
