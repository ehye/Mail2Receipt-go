# GitHub Actions Automation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically maintain Go dependencies through Dependabot, merge passing Dependabot updates, and publish the Windows executable for version tags.

**Architecture:** Dependabot creates Go module update pull requests. A Windows CI workflow validates all changes, while a privileged but non-checkout `pull_request_target` workflow only enables GitHub's native auto-merge for Dependabot-authored pull requests. A separate tag-only Windows workflow builds and publishes the release asset.

**Tech Stack:** GitHub Actions, Dependabot, GitHub CLI, Go 1.24, Windows hosted runners.

## Global Constraints

- Keep the application Windows-only and build `dist/mail2receipt.exe` with `go build -trimpath -ldflags="-s -w" -o dist/mail2receipt.exe ./cmd/mail2receipt`.
- Verify release size with `(Get-Item -LiteralPath "dist/mail2receipt.exe").Length -lt 20MB`.
- Run the uncached Go suite with `go test ./... -count=1 -timeout 120s` and static checks with `go vet ./...`.
- Dependabot's privileged workflow must never check out or execute pull-request code.
- Release only version tags matching `v*`; branch pushes never publish a release.

---

### Task 1: Configure Dependency Updates

**Files:**
- Create: `.github/dependabot.yml`

**Interfaces:**
- Produces: weekly individual `gomod` Dependabot pull requests from `/`.

- [ ] **Step 1: Add the Dependabot configuration**

```yaml
version: 2
updates:
  - package-ecosystem: gomod
    directory: /
    schedule:
      interval: weekly
```

- [ ] **Step 2: Inspect the resulting configuration**

Run: `git diff --check -- .github/dependabot.yml`

Expected: no whitespace errors. GitHub validates Dependabot configuration when it is pushed.

### Task 2: Add Windows Continuous Integration

**Files:**
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: repository Go module and source tree.
- Produces: a required `CI` check for pull requests and pushes.

- [ ] **Step 1: Add the workflow trigger and Windows Go setup**

```yaml
name: CI

on:
  push:
  pull_request:

permissions:
  contents: read

jobs:
  test:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
```

- [ ] **Step 2: Add the required validation and release-build commands**

```yaml
      - run: go test ./... -count=1 -timeout 120s
      - run: go vet ./...
      - run: go build -trimpath -ldflags="-s -w" -o dist/mail2receipt.exe ./cmd/mail2receipt
```

- [ ] **Step 3: Validate the workflow**

Run: `actionlint .github/workflows/ci.yml`

Expected: validation succeeds without diagnostics.

### Task 3: Enable Dependabot Auto-Merge Safely

**Files:**
- Create: `.github/workflows/dependabot-auto-merge.yml`

**Interfaces:**
- Consumes: Dependabot-authored pull request events.
- Produces: native GitHub auto-merge enabled with merge commits after required checks pass.

- [ ] **Step 1: Add a privileged event filter without repository checkout**

```yaml
name: Dependabot Auto-Merge

on:
  pull_request_target:
    types: [opened, reopened, synchronize]

permissions:
  contents: write
  pull-requests: write

jobs:
  enable-auto-merge:
    if: github.event.pull_request.user.login == 'dependabot[bot]'
    runs-on: ubuntu-latest
```

- [ ] **Step 2: Enable GitHub's native auto-merge**

```yaml
    steps:
      - name: Enable auto-merge
        env:
          GH_TOKEN: ${{ github.token }}
          PR_URL: ${{ github.event.pull_request.html_url }}
        run: gh pr merge "$PR_URL" --auto --merge
```

- [ ] **Step 3: Validate the workflow**

Run: `actionlint .github/workflows/dependabot-auto-merge.yml`

Expected: validation succeeds without diagnostics.

### Task 4: Publish Tagged Windows Releases

**Files:**
- Create: `.github/workflows/release.yml`

**Interfaces:**
- Consumes: a pushed tag matching `v*`.
- Produces: a GitHub Release with `mail2receipt.exe` attached.

- [ ] **Step 1: Add the tag trigger, permissions, and Windows build setup**

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
```

- [ ] **Step 2: Add test, static-analysis, build, size, and publishing steps**

```yaml
      - run: go test ./... -count=1 -timeout 120s
      - run: go vet ./...
      - run: go build -trimpath -ldflags="-s -w" -o dist/mail2receipt.exe ./cmd/mail2receipt
      - run: if (-not ((Get-Item -LiteralPath "dist/mail2receipt.exe").Length -lt 20MB)) { throw "Release executable must be smaller than 20 MB" }
      - name: Publish release
        env:
          GH_TOKEN: ${{ github.token }}
          TAG: ${{ github.ref_name }}
        run: gh release create "$env:TAG" "dist/mail2receipt.exe#mail2receipt.exe" --generate-notes
```

- [ ] **Step 3: Validate the workflow**

Run: `actionlint .github/workflows/release.yml`

Expected: validation succeeds without diagnostics.

### Task 5: Verify the Repository Locally

**Files:**
- Verify: `.github/dependabot.yml`
- Verify: `.github/workflows/ci.yml`
- Verify: `.github/workflows/dependabot-auto-merge.yml`
- Verify: `.github/workflows/release.yml`

- [ ] **Step 1: Lint every workflow**

Run: `actionlint`

Expected: validation succeeds without diagnostics.

- [ ] **Step 2: Run the full application suite**

Run: `go test ./... -count=1 -timeout 120s`

Expected: all packages pass.

- [ ] **Step 3: Run static checks**

Run: `go vet ./...`

Expected: no diagnostics.
