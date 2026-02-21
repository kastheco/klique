set shell := ["bash", "-cu"]
set dotenv-load := true

# Build klique binary
build:
    go build -o klique .

# Install to GOPATH/bin
install:
    go install .

# Build + install
bi: build install

# Run tests
test:
    go test ./...

# Run linter
lint:
    go vet ./...

# Run klique (pass-through args)
run *ARGS:
    go run . {{ARGS}}

# Dry-run release (no publish)
release-dry v:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "==> Dry run for klique v{{v}}"
    goreleaser release --snapshot --clean
    echo "==> Artifacts in dist/"

# Full release: just release 0.2.1
release v:
    #!/usr/bin/env bash
    set -euo pipefail

    VERSION="{{v}}"
    TAG="v${VERSION}"

    echo "==> Releasing klique ${TAG}"

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
    git tag -a "${TAG}" -m "klique ${TAG}"
    echo "    tagged ${TAG}"

    # 4. Push commit + tag
    git push origin "${BRANCH}"
    git push origin "${TAG}"
    echo "    pushed to origin"

    # 5. Goreleaser builds, creates GH release, pushes homebrew formula
    GITHUB_TOKEN="${GH_PAT}" goreleaser release --clean
    echo "==> Done: https://github.com/kastheco/klique/releases/tag/${TAG}"

# Clean build artifacts
clean:
    rm -f klique
    rm -rf dist/
