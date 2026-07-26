default: test

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

# Directory the dev_overrides binary is installed to (see docs/local-testing.md).
DEV_PLUGIN_DIR ?= $(HOME)/.terraform.d/plugins-dev-getstream

.PHONY: build
build:
	go build ./...

# Build the provider binary into the dev_overrides directory for local `terraform plan`.
.PHONY: install-dev
install-dev:
	mkdir -p "$(DEV_PLUGIN_DIR)"
	go build -o "$(DEV_PLUGIN_DIR)/terraform-provider-getstream" .

# ---------------------------------------------------------------------------
# Quality gates
# ---------------------------------------------------------------------------

.PHONY: vet
vet:
	go vet ./...

.PHONY: fmt
fmt:
	gofmt -w -l internal/ main.go

.PHONY: fmtcheck
fmtcheck:
	@test -z "$$(gofmt -l internal/ main.go)" || (echo "gofmt needed:"; gofmt -l internal/ main.go; exit 1)

# Regenerate registry docs from schema + examples (requires terraform on PATH).
.PHONY: docs
docs:
	go generate ./...

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

# Unit tests only — no credentials, no live API. TF_ACC is forced empty so a
# local .env (which may set TF_ACC=1) can't turn this into a live run; godotenv's
# Load never overrides an already-set variable.
.PHONY: test
test:
	TF_ACC= go test ./... $(TESTARGS)

# Non-destructive credential check: configures the provider and reads the app
# identity back, asserting it matches STREAM_APP_NAME. Green => the key/secret
# are valid AND point at the expected app. Source .env first.
.PHONY: verify-creds
verify-creds:
	TF_ACC=1 go test ./internal/provider/ -v -run TestAccAppDataSource

# Acceptance tests — real CRUD against the app in your environment. Source a
# .env (see .env.template) first, or export STREAM_API_KEY/SECRET/APP_NAME/APP_ID.
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

# Full pre-PR gate.
.PHONY: check
check: fmtcheck vet build test
