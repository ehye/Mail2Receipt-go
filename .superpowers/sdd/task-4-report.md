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
