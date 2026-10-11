SHELL := /usr/bin/env bash

GOLIB ?= golib
GO ?= go

.PHONY: benchmark check check-full ci cohesion fuzz inventory race repository-check specification-check specification-check-online

check:
	$(GOLIB) check --local --all

check-full: specification-check-online
	$(GOLIB) check --all

ci: repository-check cohesion specification-check check

cohesion:
	$(GOLIB) cohesion check

inventory repository-check:
	$(GOLIB) repository check

specification-check:
	$(GOLIB) specification check

specification-check-online:
	$(GOLIB) specification check --online

race:
	$(GO) test -race ./...

fuzz:
	$(GO) test -run '^$$' -fuzz '^FuzzResourceAttributes$$' -fuzztime=10000x .
	$(GO) test -run '^$$' -fuzz '^FuzzConfiguration$$' -fuzztime=10000x .
	$(GO) test -run '^$$' -fuzz '^FuzzPropagationHeaders$$' -fuzztime=10000x ./propagation
	$(GO) test -run '^$$' -fuzz '^FuzzUntrustedMetadata$$' -fuzztime=10000x ./propagation

benchmark:
	$(GO) test -run '^$$' -bench . -benchtime=100ms ./...
