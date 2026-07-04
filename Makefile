SHELL       := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := check

ILAB_BIN := bin/ilab
MCP_BIN  := bin/infra-lab-mcp
VERSION ?= $(shell cat VERSION 2>/dev/null || echo dev)
GIT_COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
ILAB_LDFLAGS := -X github.com/HeaInSeo/infra-lab/ilab/cmd.infraLabVersion=$(VERSION) -X github.com/HeaInSeo/infra-lab/ilab/cmd.gitCommit=$(GIT_COMMIT) -X github.com/HeaInSeo/infra-lab/ilab/cmd.buildDate=$(BUILD_DATE)

.PHONY: build
build:
	@echo "==> build ilab CLI"
	@mkdir -p bin
	@cd ilab && go build -ldflags "$(ILAB_LDFLAGS)" -o ../$(ILAB_BIN) .
	@echo "[OK] $(ILAB_BIN)"

.PHONY: build-mcp
build-mcp:
	@echo "==> build infra-lab MCP server"
	@mkdir -p bin
	@cd mcp && go build -o ../$(MCP_BIN) ./cmd/infra-lab-mcp
	@echo "[OK] $(MCP_BIN)"

.PHONY: test-mcp
test-mcp:
	@echo "==> test infra-lab MCP server"
	@cd mcp && go test ./...
	@$(MAKE) build-mcp
	@echo "[OK] test-mcp"

.PHONY: mcp-setup
mcp-setup: build build-mcp
	@INFRA_LAB_ROOT="$(CURDIR)" $(MCP_BIN) --setup

.PHONY: install
install:
	@echo "==> install ilab to GOPATH/bin"
	@cd ilab && go install -ldflags "$(ILAB_LDFLAGS)" .
	@echo "[OK] ilab installed"

TF_DIRS := . backends/libvirt lustre-lab

# SC2191: false positive in shellcheck <0.7 for ssh -o Key=Value arrays
# SC2029: informational — intentional client-side expansion in ssh commands
SHELLCHECK_EXCLUDE := SC2191,SC2029

.PHONY: check
check: lint-shell lint-yaml lint-hcl

.PHONY: lint-shell
lint-shell:
	@echo "==> bash -n"
	@find . -name '*.sh' \
	    -not -path './.git/*' \
	    -not -path '*/.terraform/*' \
	    -print0 | xargs -0 bash -n
	@echo "==> shellcheck"
	@find . -name '*.sh' \
	    -not -path './.git/*' \
	    -not -path '*/.terraform/*' \
	    -print0 | xargs -0 shellcheck --exclude=$(SHELLCHECK_EXCLUDE)
	@echo "[OK] lint-shell"

.PHONY: lint-yaml
lint-yaml:
	@echo "==> yaml parse"
	@python3 scripts/lint-yaml.py
	@echo "[OK] lint-yaml"

.PHONY: lint-hcl
lint-hcl:
	@echo "==> tofu fmt"
	@tofu fmt -check -recursive .
	@for d in $(TF_DIRS); do \
	    echo "==> tofu validate $$d"; \
	    tofu -chdir=$$d init -backend=false -input=false >/dev/null; \
	    tofu -chdir=$$d validate -no-color; \
	done
	@echo "[OK] lint-hcl"

# ── Environment targets ──────────────────────────────────────────────────────
# Usage: ENV_PROFILE=envs/libvirt-cilium.env make env-up
#        ENV_PROFILE=envs/multipass-flannel.env make env-up
#
# Copy an .env.example to the same name without .example, fill in your values,
# then pass it via ENV_PROFILE. The profile sets BACKEND, CNI, ADDONS, and
# any TF_VAR_* needed by the chosen backend.
ENV_PROFILE ?=

.PHONY: env-up
env-up:
	HOST_PROFILE="$(ENV_PROFILE)" bash scripts/k8s-tool.sh up

.PHONY: env-down
env-down:
	HOST_PROFILE="$(ENV_PROFILE)" bash scripts/k8s-tool.sh down

.PHONY: env-status
env-status:
	HOST_PROFILE="$(ENV_PROFILE)" bash scripts/k8s-tool.sh status

.PHONY: env-clean
env-clean:
	FORCE=1 HOST_PROFILE="$(ENV_PROFILE)" bash scripts/k8s-tool.sh clean

.PHONY: lint-go
lint-go:
	@echo "==> gofmt"
	@test -z "$$(cd ilab && gofmt -l .)" || \
	    { echo "unformatted Go files:"; cd ilab && gofmt -l .; exit 1; }
	@echo "==> go vet"
	@cd ilab && go vet ./...
	@echo "==> go build"
	@cd ilab && go build ./...
	@echo "[OK] lint-go"

.PHONY: test-go
test-go:
	@echo "==> go test"
	@cd ilab && go test ./...
	@echo "[OK] test-go"

.PHONY: test-contract
test-contract:
	@echo "==> ilab JSON contract tests"
	@cd ilab && go test ./...
	@echo "[OK] test-contract"

.PHONY: help
help:
	@echo "Lint targets (default: check):"
	@echo "  check       Run all lint checks (shell + yaml + hcl)"
	@echo "  lint-shell  bash -n + shellcheck"
	@echo "  lint-yaml   YAML parse check"
	@echo "  lint-hcl    tofu fmt + tofu validate"
	@echo "  lint-go     gofmt + go vet + go build"
	@echo "  test-go     go test ./..."
	@echo "  test-contract  Validate ilab JSON contract tests"
	@echo "  test-mcp    Test and build MCP stdio server"
	@echo "  mcp-setup   Open the MCP setup menu"
	@echo ""
	@echo "Environment targets:"
	@echo "  env-up      Create cluster    (ENV_PROFILE=envs/<name>.env)"
	@echo "  env-down    Destroy cluster   (ENV_PROFILE=envs/<name>.env)"
	@echo "  env-status  Show cluster/VM status"
	@echo "  env-clean   Remove local state files (irreversible)"
	@echo ""
	@echo "CLI targets:"
	@echo "  build       Build ilab CLI binary to bin/ilab"
	@echo "  build-mcp   Build MCP stdio server to bin/infra-lab-mcp"
	@echo "  install     Install ilab CLI to GOPATH/bin"
