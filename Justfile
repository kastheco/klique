set shell := ["bash", "-cu"]
set dotenv-load := true

# Build kasmos binary
build:
    go build -o kasmos .

# Install to GOPATH/bin (with kas, kms aliases)
install:
    go install .
    ln -sf "$(go env GOPATH)/bin/kasmos" "$(go env GOPATH)/bin/kas"
    ln -sf "$(go env GOPATH)/bin/kasmos" "$(go env GOPATH)/bin/kms"

# Build + install
bi: build install

# Install the kasmos user service unit
kasmosd-install: install
    mkdir -p ~/.config/systemd/user
    cp contrib/kasmos.service ~/.config/systemd/user/
    systemctl --user daemon-reload

# Enable and start the kasmos user service
kasmosd-enable: kasmosd-install
    systemctl --user enable --now kasmos

# Install the plan store user service unit
db-service-install: install
    mkdir -p ~/.config/systemd/user
    cp contrib/kasmosdb.service ~/.config/systemd/user/
    systemctl --user daemon-reload

# Enable and start the plan store user service
db-service-enable: db-service-install
    systemctl --user enable --now kasmosdb

# Install and start both user services
services-enable: kasmosd-enable db-service-enable

# run with no args
bin:
    kas

# Run the orchestration daemon directly
kasmosd-start: install
    kas daemon start

kasmosd-stop:
    kas daemon stop

kasmosd-status:
    kas daemon status

# Diagnose kasmos daemon installation and runtime status
doctord:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "== kasmos daemon doctor =="
    echo
    echo "binary:"
    if command -v kas >/dev/null 2>&1; then
        command -v kas
    else
        echo "missing: kas not found in PATH"
    fi
    echo
    echo "user units:"
    for unit in kasmos.service kasmosdb.service; do
        if systemctl --user cat "$unit" >/dev/null 2>&1; then
            state=$(systemctl --user is-active "$unit" 2>/dev/null || true)
            enabled=$(systemctl --user is-enabled "$unit" 2>/dev/null || true)
            echo "- $unit: installed, enabled=$enabled, active=$state"
        else
            echo "- $unit: not installed"
        fi
    done
    echo
    echo "daemon api:"
    if [ -n "${XDG_RUNTIME_DIR:-}" ]; then
        socket_path="$XDG_RUNTIME_DIR/kasmos/kas.sock"
    else
        socket_path="/tmp/kasmos-$(id -u)/kas.sock"
    fi
    if [ -S "$socket_path" ]; then
        echo "daemon socket present: $socket_path"
    else
        echo "daemon not responding"
    fi
    echo
    echo "next steps:"
    echo "- just kasmosd-enable      # install + enable orchestration daemon"
    echo "- just db-service-enable   # install + enable plan store service"
    echo "- just services-enable     # enable both services"
    echo "- just kasmosd-start       # direct foreground/background daemon start"

# Backward-compatible aliases
daemon-service-install: kasmosd-install

daemon-service-enable: kasmosd-enable

daemon-start: kasmosd-start

daemon-stop: kasmosd-stop

daemon-status: kasmosd-status

setup:
    kas setup --force

# Build + install + run
kas: build install bin

# Alias for kas
kms: kas

# Run tests
test:
    go test ./...

# Run linter
lint:
    go vet ./...

# Run kasmos (pass-through args)
run *ARGS:
    go run . {{ARGS}}

# Tag and push a release (CI runs goreleaser): just release 1.0.0
release v:
    #!/usr/bin/env bash
    set -euo pipefail

    VERSION="{{v}}"
    TAG="v${VERSION}"

    echo "==> Releasing kasmos ${TAG}"

    # 1. Ensure clean working tree
    if [[ -n "$(git status --porcelain)" ]]; then
        echo "ERROR: working tree is dirty, commit or stash first"
        exit 1
    fi

    BRANCH=$(git branch --show-current)
    echo "    branch: ${BRANCH}"

    # 2. Update version in source
    sd 'version\s*=\s*"[^"]*"' "version     = \"${VERSION}\"" main.go
    if [[ -n "$(git status --porcelain)" ]]; then
        git add main.go
        git commit -m "release: v${VERSION}"
        echo "    committed version bump"
    fi

    # 3. Tag
    git tag -a "${TAG}" -m "kasmos ${TAG}"
    echo "    tagged ${TAG}"

    # 4. Push commit + tag — CI takes it from here
    git push origin "${BRANCH}"
    git push origin "${TAG}"
    echo "==> Pushed ${TAG}. CI will build and publish the release."
    echo "    https://github.com/kastheco/kasmos/releases/tag/${TAG}"

# Build the admin SPA (outputs to web/admin/dist/)
admin-build:
    cd web/admin && npm ci && npm run build

# Start admin SPA dev server with proxy to kas serve
admin-dev:
    cd web/admin && npm run dev

# Install docs site dependencies
docs-install:
    cd web/docs && npm ci

# Start docs site dev server
docs-dev:
    cd web/docs && npm run dev

# Build docs site for production
docs-build:
    cd web/docs && npm ci && npm run build

# Clean build artifacts
clean:
    rm -f kasmos
    rm -rf dist/
