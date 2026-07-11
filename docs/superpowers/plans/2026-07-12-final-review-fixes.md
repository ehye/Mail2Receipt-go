# Final Review Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make prepared receipt HTML fully self-contained and restore Chromium integration coverage for scaled and embedded-logo documents.

**Architecture:** `internal/document` will use an allowlist for every load-bearing HTML and CSS reference: only resolved CID image data and approved embedded-logo data remain. CSS resource recognition will decode standards-compliant escapes before classification. `internal/browser` integration tests will render prepared real assets and a valid long document through an installed Chromium browser.

**Tech Stack:** Go, `golang.org/x/net/html`, embedded PNG assets, chromedp/CDP, Go testing.

## Global Constraints

- Preserve ordinary `a`/`area` href and form action navigation targets.
- Remove base elements and all non-embedded load-bearing resources, including local paths and imports.
- Do not expose receipt data or sensitive paths in errors.
- Produce exactly one A5 portrait PDF at scale 0.50 through 0.79 without clipping.
- Browser integration tests may skip only when Edge or Chrome is unavailable.

---

### Task 1: Self-Contained Document Resources

**Files:**
- Modify: `internal/document/prepare_test.go`
- Modify: `internal/document/prepare.go`

**Interfaces:**
- Consumes: `message.Document` HTML and CID image assets.
- Produces: unchanged `Prepare(message.Document) ([]byte, error)`.

- [ ] **Step 1: Write failing tests**

Add table-driven assertions covering `file:`, protocol-relative, UNC, Windows, and relative load-bearing HTML references; local and escaped CSS `url`/`@import`; base removal; navigation preservation; and approved data/CID/logo preservation.

- [ ] **Step 2: Verify RED**

Run: `go test -count=1 ./internal/document -run 'TestPrepare(RemovesAllNonEmbeddedResources|DecodesCSSResourceEscapes)' -v`

Expected: FAIL because local references, base values, and escaped CSS references survive.

- [ ] **Step 3: Implement the minimal allowlist**

Remove `<base>` nodes, retain only generated/validated embedded image data in resource attributes and CSS, remove every non-embedded import, and decode CSS escapes for resource token and scheme classification.

- [ ] **Step 4: Verify GREEN**

Run: `go test -count=1 ./internal/document -v`

Expected: PASS.

### Task 2: Chromium Success Integrations

**Files:**
- Modify: `internal/browser/render_test.go`

**Interfaces:**
- Consumes: `document.Prepare` output and `Render`.
- Produces: integration evidence for intermediate scaling and real embedded PNG serialization with zero HTTP requests.

- [ ] **Step 1: Write integration tests**

Add a valid long document that renders at `0.50 < scale < 0.79`, plus prepared HTML using both approved logo filenames, passed to Chromium and asserted as a one-page PDF while an HTTP server records zero requests.

- [ ] **Step 2: Verify focused tests**

Run: `go test -count=1 ./internal/browser -run 'TestRender(LongDocumentUsesIntermediateScale|PreparedEmbeddedLogosOffline)' -v`

Expected: PASS with an installed browser, or explicit browser-unavailable skips.

### Task 3: Final Verification and Commit

**Files:**
- Create: `.superpowers/sdd/final-fix-report.md`

**Interfaces:**
- Produces: complete evidence and one fix-wave commit.

- [ ] **Step 1: Run required verification**

Run `go test -count=1 ./...`, `go test -race ./internal/document ./internal/browser`, `go vet ./...`, and `git diff --check` with sufficient timeouts.

- [ ] **Step 2: Write report and self-review**

Record RED failures, GREEN outputs, changed files, decisions, test outputs, privacy/security review, and the resulting commit hash.

- [ ] **Step 3: Commit only fix-wave files**

Stage the implementation, tests, this plan, and report; exclude unrelated receipt fixtures and the prior untracked plan. Commit with `Fix self-contained receipt resources`.
