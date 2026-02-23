set shell := ["bash", "-cu"]
set dotenv-load := true

# Build kasmos binary
build:
    go build -o kasmos .

# Install to GOPATH/bin (with kas, ks, km aliases)
install:
    go install .
    ln -sf "$(go env GOPATH)/bin/kasmos" "$(go env GOPATH)/bin/kas"
    ln -sf "$(go env GOPATH)/bin/kasmos" "$(go env GOPATH)/bin/ks"
    ln -sf "$(go env GOPATH)/bin/kasmos" "$(go env GOPATH)/bin/km"

# Build + install
bi: build install

# run with no args
bin:
    kas

init:
    kas init --force

# Build + install + run
kas: build install bin

# Run tests
test:
    go test ./...

# Run linter
lint:
    go vet ./...

# Run kasmos (pass-through args)
run *ARGS:
    go run . {{ARGS}}

# Dry-run release (no publish)
release-dry v:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "==> Dry run for kasmos v{{v}}"
    goreleaser release --snapshot --clean
    echo "==> Artifacts in dist/"

# Full release: just release 0.2.1
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
    sed -i "s/var version = \".*\"/var version = \"${VERSION}\"/" main.go
    if [[ -n "$(git status --porcelain)" ]]; then
        git add main.go
        git commit -m "release: v${VERSION}"
        echo "    committed version bump"
    fi

    # 3. Tag
    git tag -a "${TAG}" -m "kasmos ${TAG}"
    echo "    tagged ${TAG}"

    # 4. Push commit + tag
    git push origin "${BRANCH}"
    git push origin "${TAG}"
    echo "    pushed to origin"

    # 5. Goreleaser builds, creates GH release, pushes homebrew formula
    GITHUB_TOKEN="${GH_PAT}" goreleaser release --clean
    echo "==> Done: https://github.com/kastheco/kasmos/releases/tag/${TAG}"

# Clean build artifacts
clean:
    rm -f kasmos
    rm -rf dist/
