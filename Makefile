.DEFAULT_GOAL := check
.PHONY: check

check:
	cd tools/dots && GOTOOLCHAIN=local go vet ./... && go build ./...
