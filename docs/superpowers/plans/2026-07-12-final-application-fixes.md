# Final Application Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the final security, MIME-selection, and one-page clipping findings with regression coverage and release evidence.

**Architecture:** MIME traversal selects one body structurally while retaining CID assets. Document preparation removes untrusted source data URLs before inserting only validated CID images or embedded logos. Browser inspection emulates print media and fits the union of rendered geometry.

**Tech Stack:** Go, go-message, x/net/html, chromedp/CDP, installed Edge or Chrome.

## Global Constraints

- Windows-only executable using installed Edge before Chrome; no bundled runtime or browser.
- All HTTP(S) requests blocked and no proxy.
- A5 portrait, exactly one page, scale inclusively between 0.50 and 0.79; fail instead of clipping.
- Never print, log, quote, or commit receipt fixture contents.
- Keep public APIs stable unless correctness requires a change.

---

### Task 1: Structural MIME Body Selection

**Files:**
- Modify: `internal/message/extract.go`
- Test: `internal/message/extract_test.go`

**Interfaces:**
- Consumes: `Extract(io.Reader, int64) (Document, error)`
- Produces: unchanged `Document`; deterministic first eligible non-attachment subtree selection.

- [ ] Add tests for an HTML attachment after the body, unrelated later branches, nested alternatives, and reordered mixed children.
- [ ] Run `go test ./internal/message -count=1` and confirm the new tests fail for selection behavior.
- [ ] Replace flat HTML overwrite behavior with recursive entity selection: alternative chooses its eligible HTML representation; other multiparts choose the first non-attachment child subtree with HTML.
- [ ] Preserve transfer/charset decoding and CID limits, then rerun `go test ./internal/message -count=1`.
- [ ] Commit message and MIME tests as one coherent change.

### Task 2: Trusted Image Data and CID Validation

**Files:**
- Modify: `internal/document/prepare.go`
- Test: `internal/document/prepare_test.go`
- Test: `internal/message/extract_test.go`

**Interfaces:**
- Consumes: `message.Document` and `message.Asset`.
- Produces: unchanged `Prepare(message.Document) ([]byte, error)`; only internally generated image data URLs survive.

- [ ] Add tests covering source data URLs in `src`, `srcset`, `background`, inline/style CSS, image-set mixtures, SVG and nested-resource payloads.
- [ ] Add in-memory valid PNG/JPEG/GIF helpers and tests for valid CID formats, declared/decoded mismatch, corrupt bytes, SVG, and unreferenced invalid assets.
- [ ] Run `go test ./internal/document ./internal/message -count=1` and confirm focused failures.
- [ ] Remove source-authored data image references and add trusted replacement flow; validate referenced CID bytes with registered stdlib PNG/JPEG/GIF decoders and exact media-type match.
- [ ] Update obsolete dummy-byte tests and rerun both focused packages.
- [ ] Commit document security and CID validation together.

### Task 3: Print Geometry and E2E Scale Bounds

**Files:**
- Modify: `internal/browser/render.go`
- Test: `internal/browser/render_test.go`
- Modify: `cmd/mail2receipt/e2e_test.go`

**Interfaces:**
- Consumes: unchanged `Render(context.Context, string, string) (Result, error)`.
- Produces: print-media rendered extents and inclusive E2E scale assertion.

- [ ] Add real-browser tests for transformed, fixed, print-only overflow, ordinary content, and intermediate fitting; add E2E scale bounds.
- [ ] Run focused browser tests and confirm geometry regressions fail.
- [ ] Emulate print media before inspection; gather visible client-rect and useful pseudo-element extents, reject negative geometry, and fit positive right/bottom extents.
- [ ] Rerun `go test ./internal/browser -count=1` and the receipt E2E command.
- [ ] Commit browser and E2E changes.

### Task 4: Verification and Evidence

**Files:**
- Create: `.superpowers/sdd/final-application-fix-report.md`

**Interfaces:**
- Produces: sanitized verification evidence, self-review, commit hashes, and residual visual-comparison risk.

- [ ] Run focused tests, receipt E2E, `go test ./... -count=1 -timeout 120s`, touched-package race tests, and `go vet ./...`.
- [ ] Build `dist/mail2receipt.exe` stripped and verify its size is below 20 MB.
- [ ] Run drag-drop-equivalent conversion and remove only the generated root `output.pdf`.
- [ ] Review diffs for security, fixture disclosure, API stability, and exact finding coverage.
- [ ] Write sanitized evidence without reading or quoting `receipt.pdf`; commit only the report and intended source/test/plan files.
