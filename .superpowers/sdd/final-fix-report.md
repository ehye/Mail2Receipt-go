# Final Whole-Change Review Fix Report

## Result

Implementation commit: `7955cf21c7e6a35fe93fd4a3e362dbc64472e028`

The final review findings were addressed without staging or modifying the unrelated receipt fixtures or the pre-existing untracked offline-assets plan.

## RED Evidence

Command:

```text
go test -count=1 ./internal/document -run 'TestPrepare(RemovesAllNonEmbeddedResources|DecodesCSSResourceEscapes)' -v
```

Result: exit 1 as expected.

```text
TestPrepareRemovesAllNonEmbeddedResources: retained <base, protocol-relative host, private local paths, local.css, sprite.svg, and root/image.png markers
TestPrepareDecodesCSSResourceEscapes: retained u\72l, h\74tps, remote host, local path, UNC, and private path markers
FAIL mail2receipt/internal/document
```

The failures demonstrated the pre-fix behavior rather than test setup errors. The two browser coverage additions passed immediately because they restore missing integration proof rather than require a production behavior change:

```text
TestRenderLongDocumentUsesIntermediateScale PASS
TestRenderPreparedEmbeddedLogosOffline PASS
ok mail2receipt/internal/browser
```

## GREEN Evidence

Focused document package:

```text
go test -count=1 ./internal/document -v
PASS
ok mail2receipt/internal/document 0.975s
```

Focused browser package, using the installed Chromium browser:

```text
go test -count=1 ./internal/browser -v
TestRenderLongDocumentUsesIntermediateScale PASS
TestRenderPreparedEmbeddedLogosOffline PASS
PASS
ok mail2receipt/internal/browser 5.620s
```

Required final verification:

```text
go test -count=1 ./...
ok mail2receipt/internal/browser 6.588s
ok mail2receipt/internal/document 1.269s
ok mail2receipt/internal/message 1.022s

go test -race ./internal/document ./internal/browser
ok mail2receipt/internal/document 2.188s
ok mail2receipt/internal/browser 8.633s

go vet ./...
exit 0, no output

git diff --check
exit 0; only Git LF-to-CRLF working-copy warnings, no whitespace errors
```

## Files

- `internal/document/prepare.go`: embedded-resource allowlist, base removal, local meta-refresh neutralization, non-navigation href sanitization, and CSS escape decoding/classification.
- `internal/document/prepare_test.go`: regressions for file, protocol-relative, Windows, UNC, relative, CSS/import, escaped CSS, navigation, data, CID, and embedded-logo behavior.
- `internal/browser/render_test.go`: valid intermediate-scale integration and prepared real-logo offline integration.
- `docs/superpowers/plans/2026-07-12-final-review-fixes.md`: executed TDD fix-wave plan.
- `.superpowers/sdd/final-fix-report.md`: this evidence report.

## Decisions

- Load-bearing HTML attributes now retain only image `data:` URLs, resolved CID image data, or approved embedded-logo data. Every other local or external reference is removed.
- Ordinary `a` and `area` href values and form actions remain unchanged as required.
- All `<base>` elements are removed, and meta-refresh targets are neutralized regardless of scheme so local files cannot be reached through document navigation.
- All CSS imports are removed because a prepared receipt must be self-contained.
- CSS escapes are decoded only for security classification. Safe original CSS text remains serialized unchanged, avoiding changes to receipt-visible text.
- Browser integration uses `document.Prepare` with both exact approved filenames, writes the serialized output, renders actual embedded PNG bytes, verifies one PDF page, and observes zero HTTP requests.

## Self-Review

- Security: checked `src`, `srcset`, `background`, `poster`, object `data`, all non-navigation `href`, inline declarations, stylesheets, imports, nested `srcdoc`, base elements, and meta refresh. No retained non-data resource can read a local path or contact SMB/HTTP.
- Privacy: production errors remain generic and never include rejected values or paths. New security-test failures report fixed marker names rather than serialized receipt HTML.
- Behavior: user-visible receipt text is not rewritten. Navigation targets explicitly required by the design remain present.
- Browser: installed-browser runs exercised intermediate fitting and both real embedded logo assets; both produced exactly one PDF page.
- Scope: `receipt.eml`, `receipt.html`, `receipt.pdf`, and `docs/superpowers/plans/2026-07-11-offline-receipt-assets.md` remain untracked and untouched.

## Remaining Findings Fix Wave

Implementation commit: `07e78b298ff52e6a500f9647184d758e03e4bc1d`

### RED Evidence

CSS string-resource regression command:

```text
go test -count=1 ./internal/document -run TestPrepareFiltersImageSetStringResources -v
```

Result: exit 1 as expected. Safe data string, safe data `url()`, and safe vendor-function cases passed, while file, UNC, relative, protocol-relative, HTTP(S), mixed safe/unsafe, escaped function, and escaped scheme cases all failed because their declarations remained.

Embedded-logo integration command:

```text
go test -count=1 ./internal/browser -run TestRenderPreparedEmbeddedLogosOffline -v
```

Result: build failed as expected with `undefined: renderWithInspection`, proving the requested browser-observed dimension assertion required a new test seam.

Self-review boundary regression command:

```text
go test -count=1 ./internal/document -run 'TestPrepareFiltersImageSetStringResources/unrelated_function_suffix' -v
```

Result: exit 1 as expected because the first matcher incorrectly treated `my-image-set()` as the standard function.

### GREEN Evidence

Focused CSS command:

```text
go test -count=1 ./internal/document -run TestPrepareFiltersImageSetStringResources -v
PASS
ok mail2receipt/internal/document 1.021s
```

All 14 cases passed: safe string candidates with `type()` descriptors, safe `url()` candidates, vendor syntax, unrelated function suffixes, file/UNC/relative/protocol-relative/HTTP strings, unsafe `url()`, mixed candidates, and escaped functions/schemes.

Focused embedded-logo command:

```text
go test -count=1 ./internal/browser -run TestRenderPreparedEmbeddedLogosOffline -v
PASS
ok mail2receipt/internal/browser 2.866s
```

The installed browser reported both exact prepared images complete with positive natural width, natural height, rendered width, and rendered height. The same test retained zero HTTP request and exactly-one-page PDF assertions.

Required final verification after self-review changes:

```text
go test -count=1 ./...
ok mail2receipt/internal/browser 6.693s
ok mail2receipt/internal/document 0.584s
ok mail2receipt/internal/message 0.928s

go test -race ./internal/document ./internal/browser
ok mail2receipt/internal/document 2.490s
ok mail2receipt/internal/browser 8.688s

go vet ./...
exit 0, no output

git diff --check
exit 0; only Git LF-to-CRLF working-copy warnings, no whitespace errors
```

### Decisions And Self-Review

- `image-set()` and `-webkit-image-set()` are classified after CSS escape decoding. Every leading string candidate must be an image data URL; decoded `url()` candidates remain covered by the existing URL classifier.
- Candidate descriptors such as `type("image/png")` are not mistaken for resource strings, and safe non-resource CSS remains unchanged.
- Function-token boundaries prevent suffix matches in unrelated identifiers such as `my-image-set()`.
- Other Chromium CSS image functions were considered. Their load-bearing arguments use `<image>`/`url()` rather than the bare `<string>` source syntax specific to image-set, so the existing decoded URL scan covers them without broad string filtering.
- The package-private inspection seam runs an optional CDP action after offline navigation and before measurement/printing. Public `Render` always passes `nil`, so production behavior and unrelated failed-image handling are unchanged.
- The logo integration passes HTML through `document.Prepare`, uses both exact approved filenames and actual embedded PNG bytes, then verifies browser decoding/rendering, zero HTTP requests, and one PDF page.
- No production error includes a rejected CSS resource or sensitive path.
- Untracked receipt fixtures and the pre-existing untracked plan were not staged or modified.
