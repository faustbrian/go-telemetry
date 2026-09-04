SHELL := /usr/bin/env bash

GOLIB ?= golib

.PHONY: check ci cohesion inventory repository-check specification-check

check:
	$(GOLIB) check --all

ci: repository-check cohesion specification-check check

cohesion:
	$(GOLIB) cohesion check

inventory repository-check:
	$(GOLIB) repository check

specification-check:
	$(GOLIB) specification check --online
