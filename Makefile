.DEFAULT_GOAL := help

GO ?= go
VP ?= vp
ARGS ?=
JOBS ?= 4

ifeq ($(OS),Windows_NT)
BINARY := bin/agentdesk.exe
else
BINARY := bin/agentdesk
endif

.PHONY: help install build build-all check test test-e2e fmt serve run start status install-skill

help:
	@echo "Targets: install build build-all check test test-e2e fmt serve start status install-skill"
	@echo "Pass server options with ARGS, e.g. make start ARGS=--telegram"

install:
	$(VP) -C agentdesk-ui install
	$(GO) mod download

build:
	$(VP) node scripts/build.mjs

build-all:
	$(VP) node scripts/build.mjs --all --jobs $(JOBS)

$(BINARY):
	$(VP) node scripts/build.mjs

check:
	$(VP) -C agentdesk-ui check
	$(GO) vet ./...

test:
	$(GO) test ./...
	$(VP) -C agentdesk-ui test

test-e2e: $(BINARY)
	$(VP) -C agentdesk-ui run test:e2e

fmt:
	gofmt -w cmd internal agentdesk-ui/embed.go
	$(VP) -C agentdesk-ui fmt src e2e vite.config.ts playwright.config.ts

serve run: $(BINARY)
	$(BINARY) serve $(ARGS)

start: $(BINARY)
	$(BINARY) start $(ARGS)

status: $(BINARY)
	$(BINARY) status --json $(ARGS)

install-skill: $(BINARY)
	$(VP) node scripts/install-skill.mjs $(ARGS)
