.PHONY: all install-tools test lint revive security gosec

ifeq ($(OS),Windows_NT)
NULL_DEVICE := NUL
else
NULL_DEVICE := /dev/null
endif

all: install-tools test lint security

install-tools:
	go get -tool github.com/securego/gosec/v2/cmd/gosec
	go get -tool github.com/mgechev/revive
	go tool -n gosec >$(NULL_DEVICE)
	go tool -n revive >$(NULL_DEVICE)

test:
	go test ./...

lint: revive

revive:
	go tool revive -config .revive.toml -formatter friendly ./...

security: gosec

gosec:
	go tool gosec ./...
