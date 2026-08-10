.PHONY: test build web sdk

test:
	go test ./...
	cd sdk/python && python -m pytest

build:
	go build ./cmd/jobdock-server ./cmd/jobdock-agent

web:
	cd web && npm ci && npm run build

sdk:
	cd sdk/python && python -m build

