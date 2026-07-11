# Task 4 Report: CDP Image Validation and One-Page Rendering

## Status

Implemented and verified.

## Changes

- Added `browser.Render(ctx, executable, htmlPath) (browser.Result, error)` and `browser.Result`.
- Starts the selected installed browser with an isolated temporary profile and explicit headless/GPU/first-run flags.
- Does not set a proxy override, allowing Edge or Chrome to inherit the Windows system proxy.
- Tracks actual CDP image requests, including CSS background images, redirects, HTTP failures, loading failures, and completion.
- Protects all listener state with a mutex and uses a notification channel for race-free waiting.
- Applies a 15-second navigation/image deadline after browser startup.
- Validates all `document.images` using `complete` and `naturalWidth`.
- Reports failed remote images as host/path only, excluding query strings and receipt content.
- Measures DOM scroll dimensions and CDP `CSSContentSize`.
- Selects the largest scale at or below 0.79 that fits the 132 x 194 mm printable area and rejects scales below 0.50.
- Prints A5 portrait with backgrounds, no browser margins, and no headers or footers.
- Verifies the output starts with `%PDF-` and contains exactly one PDF page.
- Explicitly closes the browser and allocator, then removes the temporary profile.
- Added browser integration coverage for successful `<img>`, CSS background, redirect, HTTP 404, connection failure, an image exceeding 15 seconds, and a long document requiring scale below 0.79.
- Added `chromedp v0.14.2` and CDP dependency metadata while preserving the module's Go 1.24 version.

## TDD Evidence

Initial focused test command:

```text
go test ./internal/browser -run TestRender -v -timeout 60s
```

It failed as expected because `Render` was undefined:

```text
internal\browser\render_test.go:53:19: undefined: Render
internal\browser\render_test.go:105:14: undefined: Render
FAIL mail2receipt/internal/browser [build failed]
```

The first green attempt exposed that a 900 px fixture correctly remained at the 0.79 cap. The fixture was corrected to 1100 px, requiring a scale near 0.67. Repeated verification later exposed that chromedp's lazy browser startup was consuming the image deadline; browser startup was moved before the 15-second deadline.

## Verification

All of the following passed on Windows with installed Edge:

```text
go test ./internal/browser -run TestRender -v -timeout 90s
go test ./... -timeout 120s
go test -race ./internal/browser -run TestRender -timeout 120s
go vet ./...
git diff --check
```

Focused integration result: 5 render scenarios passed across the success and failure groups, with no failures or skips.

## Concerns

- Browser integration duration depends on installed Edge or Chrome startup and shutdown speed. The image deadline deliberately starts after browser startup so it measures resource loading rather than process launch.
- PDF page verification relies on Chromium's emitted `/Type /Page` page objects. This matches current Edge/Chrome PDF output and is covered by every successful integration render.

## Confirmed Finding Fixes

### Proxy Inheritance

No proxy flag was added or overridden. The pinned `chromedp v0.14.2` `DefaultExecAllocatorOptions` definition in `allocate.go:56-84` contains no `no-proxy-server` or other proxy override. The renderer therefore continues to let installed Edge or Chrome inherit the Windows system proxy.

### CSS Decode Validation

- Enumerates computed `background-image` URLs for every element and its `::before` and `::after` pseudo-elements.
- Creates browser-side `Image` objects, awaits `decode()`, and requires positive natural width and height.
- Rejects malformed and empty HTTP 200 image bodies.
- Sanitizes CSS decode failures to URL host/path, excluding query strings.

### Loading Lifecycle

- Keeps the CDP listener on the parent browser target context.
- Uses raw `Page.navigate`, allows up to 15 seconds for document parsing to finish or begin an image request, and starts the 15-second initial asset deadline when the first actual image request is observed.
- After document load, uses a fresh 15-second delayed-asset/quiescence/decode context.
- Requires 300 ms with no image request lifecycle activity and no pending image requests before validation proceeds.
- Runs DOM image inspection, layout measurement, and `PrintToPDF` on the caller-controlled parent browser context, not an expiring asset context.

### Added Regressions

- Malformed HTTP 200 CSS background decoding and sanitized error reporting.
- Empty HTTP 200 pseudo-element background decoding and sanitized error reporting.
- Delayed `<img>` and CSS background requests, with an exact server request-count assertion proving rendering did not return early.
- A successful image consuming 12 of the 15 asset-loading seconds, followed by PDF generation.
- Content below the 0.50 scale minimum is rejected.
- Content at the 0.50 fitting boundary is accepted.

### Fix TDD Evidence

Before production changes, the delayed-request regression failed with:

```text
TestRenderWaitsForDelayedImageRequests: delayed image requests = 0, want 2
```

The malformed CSS regression also failed before production changes with:

```text
TestRenderRejectsFailedImagesWithoutLeakingQuery/malformed_CSS_background:
Render() error = nil, want CSS image decode failure
```

After adding quiescence and CSS decoding, the near-deadline test exposed the shared-context defect:

```text
TestRenderPrintsAfterNearDeadlineImage:
Render() error = browser could not load receipt images
```

It passed after separating the loading/decode contexts from inspection and printing. The fixture was set to a deterministic 12-second response delay after 13- and 14-second delays proved vulnerable to local navigation/request startup consuming the remaining deadline.

### Fix Verification

The final verification passed with installed Edge:

```text
go test ./internal/browser -run TestRender -v -timeout 150s
go test ./internal/browser -run TestRenderPrintsAfterNearDeadlineImage -count=2 -v -timeout 45s
go test ./... -timeout 150s
go test -race ./internal/browser -run TestRender -timeout 180s
go vet ./...
git diff --check
```

### Post-Commit Timing Correction

An uncached post-commit full run exposed that starting the initial asset timer before high-level navigation could intermittently consume the deadline in CDP/file-navigation setup rather than image transfer:

```text
TestRenderLoadsImagesAndProducesOnePagePDF/long_document_scales_below_initial_scale:
Render() error = image failed: 127.0.0.1:<port>/image.png
```

The navigation flow now issues raw `Page.navigate`, listens for `Page.loadEventFired`, and starts the 15-second asset timer from `Network.requestWillBeSent` for the first image. This preserves the 16-second slow-image rejection while allowing the 12-second near-deadline image to complete independent of CDP setup latency.

The focused timing regressions then passed twice each:

```text
go test ./internal/browser -run TestRenderPrintsAfterNearDeadlineImage -count=2 -v -timeout 45s
go test ./internal/browser -run TestRenderRejectsFailedImagesWithoutLeakingQuery/slow -count=2 -v -timeout 50s
```

Fresh final verification after this correction passed:

```text
go test ./... -count=1 -timeout 180s
go test -race ./internal/browser -run TestRender -count=1 -timeout 180s
go vet ./...
git diff --check
```

## JavaScript Security Decision

The user resolved the prior plan conflict: JavaScript execution must be disabled before navigating to untrusted email HTML. This supersedes the earlier delayed-script request regression; quiescence now exists only to settle static image network events.

Implementation:

- Calls CDP `Emulation.setScriptExecutionDisabled(true)` in the ordered action sequence before `Page.navigate`.
- Keeps the 300 ms quiescence period for static `<img>`, CSS background, redirect, and decode-related network events.
- Continues to use trusted CDP evaluation for DOM image checks, computed CSS background decoding, and layout measurement; installed Edge permits these inspector evaluations while page script execution is disabled.

TDD RED evidence before adding the CDP command:

```text
TestRenderDisablesJavaScriptWhileLoadingStaticImages:
script-scheduled image requests = 1, want 0
```

The regression includes a static `<img>` delayed by 700 ms and a static CSS background, while untrusted inline script attempts to schedule another image after 400 ms, beyond the 300 ms quiescence period. After the fix, exactly two static image requests occur and the script-scheduled request count remains zero.

The fitting boundary test was also corrected. The previous 1460 px integration fixture exercised approximately 0.502 scale rather than the exact lower bound. `scaleToFit` is now directly tested with dimensions `printableWidth / 0.50` and `printableHeight / 0.50`, requiring an exact `0.50` result; `math.Nextafter` verifies the immediately larger height is rejected.

Exact-boundary RED evidence:

```text
internal\browser\render_test.go:233:16: undefined: scaleToFit
internal\browser\render_test.go:241:11: undefined: scaleToFit
```

Final verification passed:

```text
go test ./internal/browser -run TestRender -v -timeout 150s
go test ./... -count=1 -timeout 180s
go test -race ./internal/browser -run TestRender -count=1 -timeout 180s
go vet ./...
git diff --check
```
