# Mail to A5 PDF Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Windows CLI that converts a receipt `.eml` into an exactly one-page A5 PDF named `output.pdf` beside the input by default.

**Architecture:** Parse and normalize the preferred HTML MIME body, inline CID images, embed the two approved Google Play logos, and remove other remote image resources and non-text `#EDEDED` styling. Installed Edge or Chrome blocks HTTP(S), uses no proxy, measures the offline page, and prints at a scale no greater than 0.79; a small PDF reader verifies exactly one A5 page before atomic output replacement.

**Tech Stack:** Go 1.24, `github.com/emersion/go-message` v0.18.2, `golang.org/x/net/html`, `github.com/chromedp/chromedp`, `rsc.io/pdf`, Go standard testing package.

## Global Constraints

- Windows is the only required initial platform.
- The stripped executable must be smaller than 20 MB.
- Edge is preferred; Chrome is the fallback; no browser is bundled.
- Browser rendering must block HTTP(S) and use no proxy.
- JavaScript execution must be disabled before loading untrusted email HTML.
- Prepared HTML must embed the two approved Google Play logo PNGs and remove other remote image resources and non-text `#EDEDED` styling.
- Output is A5 portrait, 148 x 210 mm, with 8 mm margins and exactly one page.
- Fitting starts at scale 0.79, may reduce to 0.50, and must never clip silently.
- Input is limited to 25 MB; each CID image to 10 MB; all CID images to 50 MB.
- Temporary files must be cleaned up on success and failure.
- Existing output is replaced only with `--force`.

---

## File Map

- `go.mod`, `go.sum`: module and pinned dependencies.
- `cmd/mail2receipt/main.go`: process entry point and exit handling.
- `internal/app/app.go`: argument parsing and end-to-end orchestration.
- `internal/message/extract.go`: MIME traversal, HTML selection, charset decoding, and CID indexing.
- `internal/document/prepare.go`: CID validation/replacement and print-style injection.
- `internal/browser/discover_windows.go`: Edge/Chrome path discovery.
- `internal/browser/render.go`: offline CDP lifecycle, fitting, and printing.
- `internal/pdfcheck/pdfcheck.go`: page count and A5 media-box verification.
- Matching `*_test.go` files: focused unit and integration tests.
- `AGENTS.md`: verified build and focused-test commands.

### Task 1: MIME Receipt Extraction

**Files:**
- Create: `go.mod`
- Create: `internal/message/extract.go`
- Test: `internal/message/extract_test.go`

**Interfaces:**
- Produces: `message.Extract(r io.Reader, maxBytes int64) (message.Document, error)`
- Produces: `message.Document{HTML []byte, CID map[string]message.Asset}`
- Produces: `message.Asset{MediaType string, Data []byte}`

- [ ] **Step 1: Initialize the module and write failing extraction tests**

Create `go.mod` with module `mail2receipt`, Go 1.24, and `github.com/emersion/go-message` v0.18.2. Tests must construct messages in memory and assert: HTML wins over plain text, Base64 and quoted-printable decode, nested multipart works, ISO-8859-1 becomes UTF-8, Content-ID keys are normalized without angle brackets, missing HTML fails, malformed MIME fails, and reading beyond 25 MB returns `message too large`.

```go
func TestExtractPrefersDecodedHTML(t *testing.T) {
    raw := "MIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=x\r\n\r\n" +
        "--x\r\nContent-Type: text/plain\r\n\r\nplain\r\n" +
        "--x\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\nPGI+cmVjZWlwdDwvYj4=\r\n--x--\r\n"
    got, err := Extract(strings.NewReader(raw), 1<<20)
    if err != nil { t.Fatal(err) }
    if string(got.HTML) != "<b>receipt</b>" { t.Fatalf("HTML = %q", got.HTML) }
}
```

- [ ] **Step 2: Run the focused tests and confirm failure**

Run: `go test ./internal/message -v`

Expected: FAIL because `Extract` and its types do not exist.

- [ ] **Step 3: Implement bounded MIME extraction**

Use `io.LimitReader(r, maxBytes+1)`, reject an extra byte, call `message.Read`, recursively walk multipart entities, and decode transfer encodings and declared charsets. Keep the final valid `text/html` alternative and index image parts by normalized `Content-ID`. Enforce individual and aggregate CID limits while reading parts. Return sentinel-style errors with context but never include message bodies or account data.

```go
type Asset struct { MediaType string; Data []byte }
type Document struct { HTML []byte; CID map[string]Asset }

func normalizeCID(v string) string {
    return strings.ToLower(strings.Trim(strings.TrimSpace(v), "<>"))
}
```

- [ ] **Step 4: Run extraction tests**

Run: `go test ./internal/message -v`

Expected: PASS for all MIME, encoding, charset, CID, and limit cases.

### Task 2: Self-Contained HTML Preparation

**Files:**
- Create: `internal/document/prepare.go`
- Test: `internal/document/prepare_test.go`

**Interfaces:**
- Consumes: `message.Document`
- Produces: `document.Prepare(doc message.Document) ([]byte, error)`

- [ ] **Step 1: Write failing CID and print-style tests**

Cover case-insensitive `cid:` references in `src`, `srcset`, `background`, and inline `style: url(...)`; missing CID; non-image CID; approved-logo embedding; removal of other remote image resources and non-text `#EDEDED` styling; preservation of ordinary hyperlinks; and injection of one print style into `<head>`.

```go
func TestPrepareInlinesCID(t *testing.T) {
    in := message.Document{
        HTML: []byte(`<html><head></head><body><img src="cid:Logo@ID"></body></html>`),
        CID: map[string]message.Asset{"logo@id": {MediaType: "image/png", Data: []byte{1, 2, 3}}},
    }
    got, err := Prepare(in)
    if err != nil { t.Fatal(err) }
    if !bytes.Contains(got, []byte(`data:image/png;base64,AQID`)) { t.Fatalf("HTML = %s", got) }
}
```

- [ ] **Step 2: Verify tests fail**

Run: `go test ./internal/document -v`

Expected: FAIL because `Prepare` does not exist.

- [ ] **Step 3: Implement DOM traversal and style injection**

Parse with `golang.org/x/net/html`, traverse every element attribute, replace only syntactically valid CID references, and fail when a referenced CID is absent or not `image/*`. Add this style as the final head child so it controls print setup without rewriting receipt layout:

```css
@page { size: A5 portrait; margin: 8mm; }
html, body { margin: 0; padding: 0; }
```

Serialize a complete UTF-8 HTML document. Preserve ordinary hyperlinks and embedded CID images, embed the two approved Google Play logo PNGs, and remove other remote image resources and active remote resources before browser rendering.

- [ ] **Step 4: Run preparation tests**

Run: `go test ./internal/document -v`

Expected: PASS.

### Task 3: Windows Browser Discovery

**Files:**
- Create: `internal/browser/discover_windows.go`
- Create: `internal/browser/discover_other.go`
- Test: `internal/browser/discover_windows_test.go`

**Interfaces:**
- Produces: `browser.FindExecutable(getenv func(string) string, exists func(string) bool) (string, error)`

- [ ] **Step 1: Write table-driven discovery tests**

Test Edge before Chrome across `%PROGRAMFILES(X86)%`, `%PROGRAMFILES%`, and `%LOCALAPPDATA%`; ignore empty environment variables; return `supported browser not found` when no candidate exists.

- [ ] **Step 2: Verify the Windows-targeted test fails**

Run: `go test ./internal/browser -run TestFindExecutable -v`

Expected: FAIL because discovery is undefined.

- [ ] **Step 3: Implement deterministic discovery**

Use Windows build tags for real discovery and a non-Windows stub returning an unsupported-platform error so unit tests and tooling remain comprehensible. Candidate order must be Edge system, Edge user, Chrome system, Chrome user; inject environment and existence functions for tests.

- [ ] **Step 4: Run discovery tests**

Run: `go test ./internal/browser -run TestFindExecutable -v`

Expected: PASS.

### Task 4: Offline CDP and One-Page Rendering

**Files:**
- Create: `internal/browser/render.go`
- Test: `internal/browser/render_test.go`

**Interfaces:**
- Produces: `browser.Render(ctx context.Context, executable, htmlPath string) (browser.Result, error)`
- Produces: `browser.Result{PDF []byte, Scale float64}`

- [ ] **Step 1: Write browser integration tests using `httptest.Server`**

Tests must skip with the discovery error if no browser exists. Serve remote `<img>`, stylesheet, CSS background, and script URLs from an HTTP test server, render successfully, and assert the server receives zero requests. Also cover disabled JavaScript, a long document that fits below 0.79, the exact 0.50 fitting boundary, and one-page PDF output.

- [ ] **Step 2: Run the focused renderer test and confirm failure**

Run: `go test ./internal/browser -run TestRender -v -timeout 60s`

Expected: FAIL because `Render` is undefined.

- [ ] **Step 3: Implement isolated browser startup and CDP monitoring**

Create a temporary browser profile and pass `--headless=new`, `--disable-gpu`, `--no-proxy-server`, `--no-first-run`, and `--no-default-browser-check`. In one CDP action sequence before navigation, enable the network domain, block `http://*` and `https://*`, enable the page domain, and disable JavaScript execution. Navigate only after those controls are active; do not track or wait for image downloads.

- [ ] **Step 4: Implement measurement and printing**

Measure `document.documentElement.scrollWidth/scrollHeight` after network idle. Compute the largest scale at or below 0.79 that fits the 132 x 194 mm printable area, clamp only when the computed value is at least 0.50, and otherwise return `content cannot fit one A5 page`. Call `page.PrintToPDF` with 5.826772 x 8.267717 inches, zero CDP margins because CSS owns the 8 mm margin, backgrounds enabled, headers/footers disabled, and the selected scale.

- [ ] **Step 5: Run renderer tests**

Run: `go test ./internal/browser -run TestRender -v -timeout 90s`

Expected: PASS or explicit SKIP only when no supported browser is installed.

### Task 5: PDF Verification

**Files:**
- Create: `internal/pdfcheck/pdfcheck.go`
- Test: `internal/pdfcheck/pdfcheck_test.go`

**Interfaces:**
- Produces: `pdfcheck.Verify(data []byte) error`

- [ ] **Step 1: Write failing PDF validation tests**

Check empty input, invalid PDF, zero/multiple pages, wrong media box, and one A5 portrait page. Generate fixtures in tests with minimal valid PDF objects rather than storing opaque binaries.

- [ ] **Step 2: Verify tests fail**

Run: `go test ./internal/pdfcheck -v`

Expected: FAIL because `Verify` is undefined.

- [ ] **Step 3: Implement verification with `rsc.io/pdf`**

Open bytes through `pdf.NewReader(bytes.NewReader(data), int64(len(data)))`, require `NumPage() == 1`, read page 1's media box, and accept dimensions within one PDF point of A5's 419.528 x 595.276 points. Return errors that distinguish invalid PDF, page count, and paper dimensions.

- [ ] **Step 4: Run PDF tests**

Run: `go test ./internal/pdfcheck -v`

Expected: PASS.

### Task 6: CLI Orchestration and Drag-and-Drop Defaults

**Files:**
- Create: `internal/app/app.go`
- Test: `internal/app/app_test.go`
- Create: `cmd/mail2receipt/main.go`

**Interfaces:**
- Produces: `app.Run(ctx context.Context, args []string, stdout, stderr io.Writer) int`
- Consumes all earlier package interfaces.

- [ ] **Step 1: Write failing CLI tests with injected dependencies**

Test no arguments, too many arguments, non-`.eml` input, sole input path mapping to `filepath.Join(filepath.Dir(input), "output.pdf")`, explicit output, existing output without and with `--force`, verbose scale reporting, cleanup after render failure, and no partial destination after verification failure.

```go
func TestDefaultOutputIsBesideInput(t *testing.T) {
    input := filepath.Join(t.TempDir(), "receipt.eml")
    // Inject successful extract/prepare/render/verify functions and capture destination.
    code := Run(context.Background(), []string{input}, io.Discard, io.Discard)
    if code != 0 { t.Fatalf("exit = %d", code) }
    if gotOutput != filepath.Join(filepath.Dir(input), "output.pdf") { t.Fatal(gotOutput) }
}
```

- [ ] **Step 2: Verify CLI tests fail**

Run: `go test ./internal/app -v`

Expected: FAIL because `Run` is undefined.

- [ ] **Step 3: Implement orchestration and atomic output**

Parse `--force` and `--verbose` with `flag.FlagSet`; accept one or two positional arguments. Validate input before creating a private temporary directory, call each package in order, verify PDF before output, create a temporary file in the destination directory, close it, and rename it atomically. On Windows, remove an existing destination only after successful verification and only with `--force`. Defer all temporary cleanup. User-facing errors must not include decoded email content.

- [ ] **Step 4: Add the process entry point**

```go
func main() {
    os.Exit(app.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
```

- [ ] **Step 5: Run CLI and full unit tests**

Run: `go test ./internal/app -v`

Expected: PASS.

Run: `go test ./... -timeout 120s`

Expected: PASS, with browser tests explicitly skipped only if Edge and Chrome are absent.

### Task 7: End-to-End Receipt and Release Constraints

**Files:**
- Create: `cmd/mail2receipt/e2e_test.go`
- Modify: `AGENTS.md`

**Interfaces:**
- Verifies the complete executable behavior and approved fixture.

- [ ] **Step 1: Write the failing end-to-end test**

Copy root `receipt.eml` into a temporary directory, invoke `app.Run` with only that path and `--force`, assert `output.pdf` exists beside it, call `pdfcheck.Verify`, and log the renderer scale. Skip only when browser discovery reports neither Edge nor Chrome.

- [ ] **Step 2: Run the end-to-end test**

Run: `go test ./cmd/mail2receipt -run TestReceiptEndToEnd -v -timeout 90s`

Expected before final tuning: FAIL if the scale, asset waiting, or A5 verification does not match the real receipt.

- [ ] **Step 3: Tune fitting from fixture evidence**

Adjust only the measurement conversion or initial scale within a narrow range around 0.79. Preserve the hard maximum of 0.79, minimum of 0.50, exactly-one-page verification, and no-clipping behavior. Re-run the focused end-to-end test until it passes.

- [ ] **Step 4: Build and enforce executable size**

Run: `go build -trimpath -ldflags="-s -w" -o dist/mail2receipt.exe ./cmd/mail2receipt`

Expected: build succeeds.

Run: PowerShell `(Get-Item -LiteralPath "dist/mail2receipt.exe").Length -lt 20MB`

Expected: `True`. If false, inspect dependency contribution with `go tool nm -size dist/mail2receipt.exe`; replace the oversized dependency rather than weakening the limit.

- [ ] **Step 5: Manually verify drag-and-drop-equivalent behavior**

Run: `dist\mail2receipt.exe receipt.eml`

Expected: exit 0 and root `output.pdf` is a nonempty, exactly one-page A5 PDF. Remove this generated output only if it is not the existing approved `receipt.pdf` fixture.

- [ ] **Step 6: Update repository instructions**

Replace the stale empty-repository statement in `AGENTS.md` with the exact commands above, browser prerequisite, fixture roles, Windows-only scope, default output behavior, embedded-logo and offline-blocking rules, and one-page scale limits. Do not expose receipt content in guidance or logs.

- [ ] **Step 7: Final verification**

Run: `go test ./... -count=1 -timeout 120s`

Expected: PASS.

Run: `go vet ./...`

Expected: no findings.

Run: `go build -trimpath -ldflags="-s -w" -o dist/mail2receipt.exe ./cmd/mail2receipt`

Expected: success and executable size below 20 MB.
