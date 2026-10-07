# Justfile for terraprovider Terraform/OpenTofu providers (exo, scc, teams, ...).
#
# Generic across the providers: each emits internal/provider from a cmdlet catalog
# via `cmd/gen-tf`, generates docs from ./tools (tfplugindocs), and is released
# with goreleaser on a signed `v*` tag through an approval-gated environment.
#
# Day-to-day:
#   just preflight                       # full release gate — run before tagging
#   just bump <module>@<version>         # bump a dep in both modules, tidy both
#   just gen                             # regenerate resources + docs

# List available recipes.
default:
    @just --list

# ------------------------------------------------------------- build & verify --

# Compile every package.
build:
    go build ./...

# Unit tests (acceptance tests are gated behind TF_ACC — see `acc`).
test:
    go test ./...

# Report suspicious constructs (go vet).
vet:
    go vet ./...

# Lint (golangci-lint v2).
lint:
    golangci-lint run ./...

# Format all Go sources in place.
fmt:
    gofmt -w .

# Fail if any Go source is not gofmt-clean.
fmt-check:
    @test -z "$(gofmt -l .)" || { echo 'not gofmt-clean (run: just fmt):'; gofmt -l .; exit 1; }

# Non-mutating quality gate: build, fmt-check, vet, lint, unit tests.
check: build fmt-check vet lint test

# Acceptance tests against a live tenant (TF_ACC + ARM_* creds; disposable tenant only).
acc:
    TF_ACC=1 go test -run '^TestAcc' -timeout 45m -v ./internal/provider/

# --------------------------------------------------------- code & docs (gen) --

# Regenerate provider resources/data sources from the cmdlet catalog.
generate:
    go run ./cmd/gen-tf

# Regenerate docs (tfplugindocs) from the schema + examples; needs terraform on PATH (or use docs-tofu).
docs:
    cd tools && go generate ./...

# Regenerate docs with OpenTofu instead of terraform. tfplugindocs would run
# `init` for hashicorp/exo, which tofu resolves against its own registry, so
# export the schema via a dev override and hand it to tfplugindocs instead.
docs-tofu:
    #!/usr/bin/env bash
    set -euo pipefail
    tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
    mkdir -p "$tmp/bin" "$tmp/cfg"
    go build -o "$tmp/bin/terraform-provider-exo" .
    printf 'provider_installation {\n  dev_overrides { "registry.opentofu.org/terraprovider/exo" = "%s" }\n  direct {}\n}\n' "$tmp/bin" > "$tmp/tofurc"
    printf 'terraform {\n  required_providers {\n    exo = { source = "terraprovider/exo" }\n  }\n}\n' > "$tmp/cfg/main.tf"
    (cd "$tmp/cfg" && TF_CLI_CONFIG_FILE="$tmp/tofurc" tofu providers schema -json) \
        | sed 's#registry.opentofu.org/terraprovider/exo#registry.terraform.io/hashicorp/exo#' > "$tmp/schema.json"
    tofu fmt -recursive examples/
    cd tools && go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate \
        --provider-dir .. --provider-name exo --providers-schema "$tmp/schema.json"

# Regenerate everything (code, then docs).
gen: generate docs

# ---------------------------------------------------------------- go modules --

# Tidy both the provider module and the tools module.
tidy:
    go mod tidy
    cd tools && go mod tidy

# Bump a dep across both modules + tidy both (e.g. bump github.com/terraprovider/tf-msadmin@v0.5.0); then `just gen`.
bump module:
    go get {{ module }}
    cd tools && go get {{ module }}
    go mod tidy
    cd tools && go mod tidy

# Move every terraprovider/* dependency to its latest tag, tidy both, regenerate.
update-internal:
    #!/usr/bin/env bash
    set -euo pipefail
    mods=$(go list -m all | awk 'NF==2 && $1 ~ /^github\.com\/terraprovider\// {print $1"@latest"}')
    if [ -n "$mods" ]; then echo "bumping: $mods"; go get $mods; fi
    just tidy
    just gen

# ------------------------------------------------------------------ release --

# Install the aqua-pinned tools (goreleaser, cosign, tfplugindocs, terraform).
install-tools:
    aqua install

# Validate the goreleaser config.
release-check:
    goreleaser check

# Dry-run the release build for every target (no publish).
snapshot:
    goreleaser build --snapshot --clean

# Fail if the working tree has uncommitted changes (stale generated/tidied files).
verify-clean:
    @test -z "$(git status --porcelain)" || { echo 'working tree dirty — commit regenerated/tidied files before tagging:'; git status --short; exit 1; }

# Release preflight: tidy, regenerate code+docs, full quality gate + goreleaser check, verify clean.
preflight: tidy gen check release-check verify-clean
    @echo '✓ preflight passed — safe to tag the release'
