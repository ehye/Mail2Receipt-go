# Final Geometry Fix Report

## Status

Task 3 is implemented in code/test commit `2a04f95` (`Fix print geometry measurement`).

## TDD Evidence

RED command:

```text
go test ./internal/browser -count=1 -run "TestRender(OrdinaryContentUsesMaximumScale|FitsFixedOverflow|FitsTransformedOverflow|FitsPrintOnlyPseudoElementOverflow|RejectsNegativeVisualOverflow|LongDocumentUsesIntermediateScale)$" -v
```

Expected regressions failed before the implementation: fixed overflow remained at scale 0.79, print-only pseudo-element overflow remained at scale 0.79, and negative transformed overflow returned no error. The ordinary and intermediate controls passed. Chromium already included the positive transformed case in its scroll dimensions, so that regression passed before the union implementation and remains as coverage.

GREEN evidence:

```text
go test ./internal/browser -count=1 -v
PASS (11.066s package result)

go test ./cmd/mail2receipt -run TestReceiptEndToEnd -v -timeout 90s
PASS; reported scale 0.77

go test ./... -count=1 -timeout 120s
PASS for all packages

go vet ./...
PASS (no output)

git diff --check
PASS; only line-ending conversion warnings were emitted
```

## Implementation Review

- Print media is emulated after navigation and before optional inspection or geometry measurement.
- Root/body scroll dimensions remain the baseline and are unioned with visible element client rectangles.
- Viewport-relative rectangles are converted to document coordinates with the current scroll offsets, covering fixed and transformed elements.
- Generated `::before` and `::after` boxes are considered. CSSOM does not expose pseudo-element client rectangles, so fixed generated boxes are included when computed width, height, and offsets provide usable geometry; normal-flow generated overflow remains covered by scroll dimensions.
- Negative left/top geometry and non-finite measured geometry fail with the existing cannot-fit error. Positive right/bottom extents participate in scale selection.
- Scale limits remain inclusively 0.50 through 0.79, printable dimensions and A5 portrait PDF settings are unchanged, and exact one-page validation remains in place.
- HTTP(S) blocking, no-proxy browser startup, and disabled page JavaScript are unchanged.
- The real receipt E2E remains one page and now asserts the inclusive scale range without exposing fixture content.

## Residual Concerns

CSSOM cannot directly return `::before`/`::after` client rectangles. The follow-up below supersedes the original computed-offset approximation with broad fail-closed handling.

## Geometry Review Follow-up

Follow-up code/test commit: `7ce0ee7` (`Fail closed on unmeasurable print ink`).

### TDD Evidence

RED browser command:

```text
go test ./internal/browser -count=1 -run "TestRenderRejects(UnmeasurablePseudoElementGeometry|UnmeasuredInkOverflow)$" -v
```

All requested cases failed for the expected reason before implementation: transformed fixed generated content, auto/content-sized fixed generated content, border/padding generated content, box shadow, text shadow, drop-shadow filter, outline, and generated-content shadow were accepted instead of returning the generic cannot-fit error.

RED E2E-scale command:

```text
go test ./cmd/mail2receipt -count=1 -run TestValidReportedScaleRejectsNonFiniteValues -v
```

The test build failed because the finite-range predicate did not yet exist.

GREEN evidence:

```text
go test ./internal/browser -count=1 -v
PASS (17.468s package result)

go test ./cmd/mail2receipt -count=1 -run TestValidReportedScaleRejectsNonFiniteValues -v
PASS

go test ./cmd/mail2receipt -run TestReceiptEndToEnd -v -timeout 90s
PASS; reported scale 0.77

go test ./... -count=1 -timeout 120s
PASS for all packages

go vet ./...
PASS (no output)

git diff --check
PASS; only line-ending conversion warnings were emitted
```

### Follow-up Self-review

- Removed all manual pseudo-element bounds approximation. Every active generated `::before` or `::after` fails closed because CSSOM cannot provide exact client rectangles; this safely includes fixed, absolute, transformed, auto-sized, border/padding, and ink-effect cases.
- Visible ordinary elements fail closed for box shadow, text shadow, any filter (including drop-shadow and blur), nonzero outline, text stroke, and SVG stroke. No attempt is made to estimate visual-ink expansion.
- Ordinary element geometry still uses actual client rectangles plus document scroll offsets and scroll dimensions. Existing fixed, transformed, ordinary, intermediate-fit, negative-overflow, offline-logo, HTTP-blocking, and JavaScript-disabled regressions pass.
- Parsed E2E scales now explicitly reject NaN and either infinity before inclusive 0.50 through 0.79 range acceptance.
- Print media remains enabled before inspection. Network blocking, no-proxy startup, disabled page JavaScript, A5 portrait dimensions, scale limits, and exact one-page validation are unchanged.
- Receipt fixture contents were not printed, quoted, committed, or included in this report.

### Residual Concerns

The intentionally conservative pseudo-element policy can reject otherwise harmless generated content. This is required fail-closed behavior because browser CSSOM does not expose exact pseudo-element visual bounds. The approved real receipt does not trigger the rejection and continues to pass end-to-end.
