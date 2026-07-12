# Offline Receipt Assets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render receipts without network or proxy access by embedding the two required logos, removing other remote images and `#EDEDED` decorations, and retaining one-page A5 PDF generation.

**Architecture:** `internal/document` owns all asset normalization: it embeds the two PNGs, converts matching image sources to `data:` URLs, strips other HTTP(S) image references, and removes non-text declarations containing `#EDEDED`. `internal/browser` then blocks HTTP(S), disables proxy use and JavaScript, loads the deterministic offline document, measures it, and prints it without image-network tracking.

**Tech Stack:** Go 1.24, `embed`, `encoding/base64`, `golang.org/x/net/html`, `github.com/chromedp/chromedp`, Chrome DevTools Protocol, Go standard testing package.

## Global Constraints

- Windows is the only required initial platform.
- The stripped executable must be smaller than 20 MB.
- Edge is preferred; Chrome is the fallback; no browser is bundled.
- The two Google Play PNGs must be compiled into the executable.
- Rendering must issue no HTTP(S) request and must not use a proxy.
- Ordinary hyperlinks and embedded CID images must remain intact.
- JavaScript execution must be disabled before loading untrusted email HTML.
- Output is A5 portrait, 148 x 210 mm, with 8 mm margins and exactly one page.
- Fitting starts at scale 0.79, may reduce to 0.50, and must never clip silently.
- Receipt bodies, account data, and URL queries must not appear in errors or logs.

---

## File Map

- `internal/document/assets.go`: embeds the two fixed PNG files and maps exact URL-path basenames to PNG `data:` URLs.
- `internal/document/assets/google-play-crm-lockup-ic-h-transparent-w688px-h140px-2x.png`: embedded horizontal Google Play logo.
- `internal/document/assets/google-play-crm-logo-transparent-w192px-h192px-2x.png`: embedded square Google Play logo.
- `internal/document/prepare.go`: normalizes image attributes and CSS while preserving CID processing and print CSS.
- `internal/document/prepare_test.go`: verifies substitutions, removals, preserved links/CIDs/text colors, and style cleanup.
- `internal/browser/render.go`: blocks remote URLs, disables proxy use and JavaScript, and prints without image download tracking.
- `internal/browser/render_test.go`: verifies no remote requests occur and offline documents still produce one-page PDFs.
- `docs/superpowers/plans/2026-07-11-mail-to-a5-pdf.md`: removes obsolete system-proxy and image-failure requirements from the parent plan.
- `AGENTS.md`: replaces obsolete proxy/image-failure repository guidance with the approved offline policy.

### Task 1: Embed Logos and Normalize HTML Offline

**Files:**
- Create: `internal/document/assets.go`
- Create by moving: `internal/document/assets/google-play-crm-lockup-ic-h-transparent-w688px-h140px-2x.png`
- Create by moving: `internal/document/assets/google-play-crm-logo-transparent-w192px-h192px-2x.png`
- Modify: `internal/document/prepare.go`
- Modify: `internal/document/prepare_test.go`

**Interfaces:**
- Consumes: `message.Document{HTML []byte, CID map[string]message.Asset}`
- Produces: `document.Prepare(doc message.Document) ([]byte, error)` with no signature change
- Produces internally: `embeddedLogo(source string) (dataURL string, matched bool)`
- Produces internally: `normalizeImageReference(value string, assets map[string]message.Asset) (value string, keep bool, err error)`

- [ ] **Step 1: Move the approved PNG inputs under the document package**

Run:

```powershell
New-Item -ItemType Directory -Path "internal/document/assets"
Move-Item -LiteralPath "google-play-crm-lockup-ic-h-transparent-w688px-h140px-2x.png" -Destination "internal/document/assets/"
Move-Item -LiteralPath "google-play-crm-logo-transparent-w192px-h192px-2x.png" -Destination "internal/document/assets/"
```

Expected: both PNGs exist only under `internal/document/assets/`, where `go:embed` can include them.

- [ ] **Step 2: Write failing embedded-logo and offline-normalization tests**

Add tests that pass representative HTML through `Prepare` and assert the serialized HTML properties rather than exact formatting:

```go
func TestPrepareEmbedsKnownLogosAndRemovesRemoteImages(t *testing.T) {
    input := `<html><head><style>
      .remote { background-image: url("https://images.example/bg.png"); }
      .gray { border-bottom: 1px solid #ededed; color: #EDEDED; }
    </style></head><body background="https://images.example/body.png">
      <a href="https://example.com/order?id=private">order</a>
      <img id="lockup" src="https://images.example/google-play-crm-lockup-ic-h-transparent-w688px-h140px-2x.png">
      <img id="logo" src="./google-play-crm-logo-transparent-w192px-h192px-2x.png">
      <img id="tracking" src="https://images.example/tracking.png">
      <div style="background: #EDEDED url('https://images.example/tile.png'); color: #123456">text</div>
    </body></html>`

    got, err := Prepare(message.Document{HTML: []byte(input)})
    if err != nil { t.Fatal(err) }
    text := string(got)
    if strings.Count(text, "data:image/png;base64,") != 2 { t.Fatalf("embedded PNG count = %d", strings.Count(text, "data:image/png;base64,")) }
    if strings.Contains(text, "images.example") { t.Fatalf("prepared HTML retained remote image host") }
    if !strings.Contains(text, `href="https://example.com/order?id=private"`) { t.Fatalf("ordinary hyperlink was removed") }
    if strings.Contains(strings.ToLower(text), "border-bottom: 1px solid #ededed") { t.Fatalf("gray separator was retained") }
    if !strings.Contains(strings.ToLower(text), "color: #ededed") { t.Fatalf("text color was removed") }
    if !strings.Contains(strings.ToLower(text), "color: #123456") { t.Fatalf("unrelated text color was removed") }
}
```

Extend this with table cases for `srcset`, HTML `background`, inline `style`, and `<style>` content. Include a CID image and assert its `data:` URL remains.

- [ ] **Step 3: Run the focused tests and verify failure**

Run: `go test ./internal/document -run 'TestPrepare(EmbedsKnownLogosAndRemovesRemoteImages|.*Offline.*)' -v`

Expected: FAIL because logo files are not embedded and remote image/style references remain.

- [ ] **Step 4: Add the embedded asset map**

Create `internal/document/assets.go` with package-local embedded bytes and precomputed data URLs:

```go
package document

import (
    "embed"
    "encoding/base64"
    "net/url"
    "path"
    "strings"
)

const (
    lockupFilename = "google-play-crm-lockup-ic-h-transparent-w688px-h140px-2x.png"
    logoFilename   = "google-play-crm-logo-transparent-w192px-h192px-2x.png"
)

//go:embed assets/*.png
var logoFiles embed.FS

var embeddedLogos = map[string]string{
    lockupFilename: pngDataURL("assets/" + lockupFilename),
    logoFilename:   pngDataURL("assets/" + logoFilename),
}

func pngDataURL(name string) string {
    data, err := logoFiles.ReadFile(name)
    if err != nil { panic("embedded receipt logo missing") }
    return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

func embeddedLogo(source string) (string, bool) {
    parsed, err := url.Parse(strings.TrimSpace(source))
    if err != nil { return "", false }
    dataURL, ok := embeddedLogos[path.Base(parsed.Path)]
    return dataURL, ok
}
```

- [ ] **Step 5: Normalize image-bearing attributes during DOM traversal**

In `prepare.go`, preserve the existing CID replacement first, then apply these rules:

```go
func normalizeImageReference(value string, assets map[string]message.Asset) (string, bool, error) {
    replaced, err := replaceCID(value, assets)
    if err != nil { return "", false, err }
    if replaced != value { return replaced, true, nil }
    if logo, ok := embeddedLogo(value); ok { return logo, true, nil }
    parsed, err := url.Parse(strings.TrimSpace(value))
    if err == nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) {
        return "", false, nil
    }
    return value, true, nil
}
```

When visiting image-bearing attributes, rebuild `node.Attr` rather than leaving empty remote attributes. Apply `normalizeImageReference` to `src` and `background`; rebuild `srcset` candidate-by-candidate, replacing known logos and CIDs and dropping remote candidates. Do not apply these rules to anchor `href` attributes.

- [ ] **Step 6: Remove remote CSS images and non-text `#EDEDED` declarations**

Introduce declaration normalization used for inline `style` and each declaration block in `<style>` text:

```go
func keepDeclaration(property, value string) bool {
    property = strings.TrimSpace(strings.ToLower(property))
    lowerValue := strings.ToLower(value)
    if property != "color" && strings.Contains(lowerValue, "#ededed") { return false }
    return !containsRemoteCSSURL(value)
}
```

Split declaration lists only at semicolons outside quotes and parentheses, split each declaration at its first colon outside quotes and parentheses, and retain malformed declarations unchanged unless they contain a remote `url(...)`. For `<style>` nodes, apply the same declaration filter inside each `{...}` block while preserving selectors and at-rules. Existing CID replacement remains active before filtering. Empty style attributes may be removed.

- [ ] **Step 7: Run preparation tests**

Run: `go test ./internal/document -v`

Expected: PASS, including existing CID and `srcset` coverage plus the new offline normalization cases.

- [ ] **Step 8: Commit Task 1**

```powershell
git add -- "internal/document/assets.go" "internal/document/assets" "internal/document/prepare.go" "internal/document/prepare_test.go"
git commit -m "Embed receipt logos and strip remote images"
```

### Task 2: Make Browser Rendering Strictly Offline

**Files:**
- Modify: `internal/browser/render.go`
- Modify: `internal/browser/render_test.go`
- Modify: `docs/superpowers/plans/2026-07-11-mail-to-a5-pdf.md`
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: prepared offline HTML path
- Produces: `browser.Render(ctx context.Context, executable, htmlPath string) (browser.Result, error)` with no signature change
- Preserves: `browser.Result{PDF []byte, Scale float64}` and `scaleToFit(width, height float64) (float64, error)`

- [ ] **Step 1: Replace image-network failure tests with a no-network test**

Delete tests whose contract requires successful image downloads, redirects, image timeout errors, or sanitized failed-image URLs. Add an HTTP server whose handler increments an atomic counter, then render HTML containing remote `<img>`, stylesheet, CSS background, and script URLs:

```go
func TestRenderBlocksRemoteRequestsAndProducesOnePagePDF(t *testing.T) {
    executable := testBrowser(t)
    var requests atomic.Int32
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        requests.Add(1)
        http.Error(w, "unexpected request", http.StatusInternalServerError)
    }))
    defer server.Close()

    body := fmt.Sprintf(`<link rel="stylesheet" href="%[1]s/style.css"><img src="%[1]s/logo.png"><div style="background:url('%[1]s/bg.png')">receipt</div><script src="%[1]s/app.js"></script>`, server.URL)
    result, err := Render(context.Background(), executable, writeHTML(t, body))
    if err != nil { t.Fatal(err) }
    if got := requests.Load(); got != 0 { t.Fatalf("remote requests = %d, want 0", got) }
    if !bytes.HasPrefix(result.PDF, []byte("%PDF-")) { t.Fatal("missing PDF header") }
    if pages := len(pdfPagePattern.FindAll(result.PDF, -1)); pages != 1 { t.Fatalf("PDF pages = %d, want 1", pages) }
}
```

Keep the exact `0.50` fitting-boundary tests and the test proving JavaScript remains disabled.

- [ ] **Step 2: Run the new browser test and verify failure**

Run: `go test ./internal/browser -run TestRenderBlocksRemoteRequestsAndProducesOnePagePDF -v`

Expected: FAIL because the current renderer allows HTTP(S) image requests and treats blocked/broken images as fatal.

- [ ] **Step 3: Disable proxies and block HTTP(S) before navigation**

Add the allocator flag and CDP blocked URL setup:

```go
opts := append(chromedp.DefaultExecAllocatorOptions[:],
    chromedp.ExecPath(executable),
    chromedp.UserDataDir(profile),
    chromedp.Flag("headless", "new"),
    chromedp.Flag("disable-gpu", true),
    chromedp.Flag("no-proxy-server", true),
    chromedp.Flag("no-first-run", true),
    chromedp.Flag("no-default-browser-check", true),
    chromedp.WindowSize(int(math.Ceil(printableWidth)), int(math.Ceil(printableHeight))),
)

if err := chromedp.Run(browserCtx,
    network.Enable(),
    network.SetBlockedURLs([]string{"http://*", "https://*"}),
    page.Enable(),
    emulation.SetScriptExecutionDisabled(true),
    chromedp.Navigate(pageURL),
); err != nil {
    return Result{}, errors.New("browser could not load receipt")
}
```

The blocked URL rules and script disabling must execute before `chromedp.Navigate` in the same action sequence.

- [ ] **Step 4: Remove obsolete image download state and validation**

Delete `networkQuiet`, `imageState`, its listeners/wait methods, `sanitizeURL`, `cssImageValidationScript`, `awaitPromise`, image load deadlines, DOM broken-image rejection, and now-unused imports. After navigation, inspect dimensions directly with the existing DOM evaluation and `page.GetLayoutMetrics`, then call unchanged fitting and PDF printing logic.

The resulting flow is: create isolated profile, start browser with no proxy, enable CDP blocking, disable scripts, navigate to local HTML, measure, fit, print, and verify exactly one PDF page.

- [ ] **Step 5: Run focused browser tests**

Run: `go test ./internal/browser -v`

Expected: PASS; the HTTP server records zero requests, JavaScript remains disabled, and scale boundary tests pass.

- [ ] **Step 6: Update parent plan and repository guidance**

In `docs/superpowers/plans/2026-07-11-mail-to-a5-pdf.md`, replace every system-proxy, remote-image download, timeout, and image-failure requirement with the approved embedded-logo and offline-blocking behavior. In `AGENTS.md`, replace the stale rendering bullet with:

```markdown
- Rendering must embed the two approved Google Play logo PNGs, remove other remote images and non-text `#EDEDED` styling, block HTTP(S), and use no proxy. It must produce A5 portrait and verify exactly one PDF page. Fitting starts at and never exceeds scale 0.79; do not silently clip content.
```

- [ ] **Step 7: Run complete verification**

Run:

```powershell
go test -count=1 ./...
go test -race ./internal/document ./internal/browser
go vet ./...
git diff --check
```

Expected: every command exits 0. Browser-dependent tests may skip only when neither Edge nor Chrome is installed.

- [ ] **Step 8: Commit Task 2**

```powershell
git add -- "internal/browser/render.go" "internal/browser/render_test.go" "docs/superpowers/plans/2026-07-11-mail-to-a5-pdf.md" "AGENTS.md"
git commit -m "Render receipts without network access"
```
