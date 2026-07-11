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
