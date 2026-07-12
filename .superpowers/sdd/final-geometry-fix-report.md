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

## Conservative Rendering Surface Follow-up

Code/test commit: `6b294a7` (`Restrict accepted browser paint surface`).

### TDD Evidence

RED command:

```text
go test ./internal/browser -count=1 -run "TestRenderRejects(UnmeasurablePseudoElementGeometry|UnsupportedRenderingSurfaces|AdditionalPaintOverflow)$" -v
```

Before implementation, ordinary list markers, SVG with and without marker paint, a native input, positive border-image outset, Chromium box reflection, and ink effects on marker/first-letter/first-line were accepted. Each new adversarial case failed with an unexpected nil error. The added `::before` symmetry case passed because broad before/after rejection was already present.

GREEN evidence:

```text
go test ./internal/browser -count=1 -v
PASS (24.733s package result)

go test ./cmd/mail2receipt -run TestReceiptEndToEnd -v -timeout 90s
PASS; reported scale 0.77

go test ./... -count=1 -timeout 120s
PASS for all packages

go vet ./...
PASS (no output)

git diff --check
PASS; only line-ending conversion warnings were emitted
```

### Exact Conservative Policy

The renderer accepts visible ordinary HTML boxes and `img` elements when their document-coordinate client rectangles and document scroll dimensions are finite and nonnegative and none of the following enumerated rejection rules applies:

- Reject computed `display:list-item`, including otherwise ordinary lists, rather than relying on `::marker` content reporting or potentially incomplete marker bounds.
- Reject visible SVG and MathML namespace elements.
- Reject visible `audio`, `button`, `canvas`, `embed`, `iframe`, `input`, `meter`, `object`, `optgroup`, `option`, `progress`, `select`, `textarea`, and `video` elements because browser-owned/internal paint cannot be enumerated reliably.
- Reject every active generated `::before` and `::after` pseudo-element because CSSOM exposes computed style but no exact pseudo client rectangles.
- Inspect computed styles for `::marker`, `::first-letter`, and `::first-line` where Chromium exposes them, without using marker `content` as an activity or safety signal.
- Reject visible element or inspected pseudo styles with box shadow, text shadow, any filter, nonzero outline, nonzero WebKit text stroke, nonzero SVG stroke, any nonzero/non-finite `border-image-outset` component, or non-`none` WebKit box reflection.
- Continue rejecting non-finite geometry, negative left/top visual extents, and content that needs a scale below 0.50. Positive right/bottom element extents continue to participate in fitting up to scale 0.79.

This is an enumerated policy for the currently identified browser rendering mechanisms. It does not claim automatic coverage of future CSS pseudos, properties, or browser paint features.

### Self-review And Residual Concern

- Images remain allowed and are measured by client rectangles; prepared receipt image trust and offline controls are unchanged.
- Print media remains enabled before inspection. HTTP(S) blocking, no-proxy startup, disabled JavaScript, A5 portrait output, inclusive scale bounds, and exact one-page validation are unchanged.
- The approved receipt passes this policy without fixture disclosure.
- The conservative exclusions intentionally reject safe-looking lists and controls. Newly introduced CSS/browser paint mechanisms require explicit review and may need another rejection rule; no unbounded safety claim is made.

## Animation And Transition TOCTOU Follow-up

Code/test commit: `9d6d23e` (`Reject time-varying print styles`).

### TDD Evidence

RED command:

```text
go test ./internal/browser -count=1 -run "TestRender(RejectsTimeVaryingCSS|AcceptsStaticTimingCSS)$" -v
```

Before implementation, moving transform animation, delayed animation, paused/infinite animation, print-media transition duration, and print-media transition delay all rendered instead of returning the generic cannot-fit error. The comma-separated zero-duration/no-animation control passed.

GREEN evidence:

```text
go test ./internal/browser -count=1 -v
PASS (30.746s package result)

go test ./cmd/mail2receipt -count=1 -run TestReceiptEndToEnd -v -timeout 90s
PASS; reported scale 0.77

go test ./... -count=1 -timeout 120s
PASS for all packages

go vet ./...
PASS (no output)

git diff --check
PASS; only line-ending conversion warnings were emitted
```

### Exact Timing Policy

- During print-media inspection, reject every element whose computed `animation-name` list contains any name other than `none`. Delay, duration, iteration count, and play state do not make a named animation acceptable.
- Apply the same animation-name rule to inspected `::before`, `::after`, `::marker`, `::first-letter`, and `::first-line` styles.
- Reject transitions when the computed property list contains a property other than `none` and any computed duration or delay component is nonzero. This includes zero-duration transitions with a nonzero delay.
- Parse every comma-separated duration and delay component as a finite CSS time in `s` or `ms`. Empty, malformed, non-finite, or otherwise unparseable computed lists fail closed.
- Check the standard computed animation/transition properties and WebKit-prefixed computed aliases when Chromium exposes them.
- Zero-duration, zero-delay transitions and `animation-name:none` remain accepted when no other rendering-surface rejection applies.
- Timing checks occur before element visibility filtering, preventing an animation that changes visibility from evading inspection. No waits, sleeps, timing sampling, or race-prone style freezing are used.

### Self-review And Residual Concern

- Print-media emulation still precedes inspection, so print-only animation and transition definitions are evaluated.
- Network blocking, no-proxy startup, disabled JavaScript, geometry union, conservative paint exclusions, A5 output, inclusive scale limits, and exact one-page validation are unchanged.
- The approved receipt remains accepted without exposing fixture content.
- The policy deliberately rejects harmless named animations and delayed zero-duration transitions because their stability cannot be guaranteed between measurement and printing.

## Direct Text Geometry Follow-up

Code/test commit: `d0cbbb5` (`Measure direct text print geometry`).

### TDD Evidence

RED command:

```text
go test ./internal/browser -count=1 -run "TestRender(RejectsNegativeDirectTextFromZeroAreaContainer|RejectsFarPositiveDirectTextFromZeroAreaContainer|FitsTransformedDirectTextFromZeroAreaContainer|RejectsNestedTextWithoutElementRects|AcceptsEmptyZeroAreaElement)$" -v
```

Before implementation, negative and far-positive direct text in fixed zero-area containers rendered without error, transformed direct text remained at scale 0.79, and nested text under `display:contents` rendered without error. The empty zero-area control passed.

GREEN evidence:

```text
go test ./internal/browser -count=1 -v
PASS (31.864s package result)

go test ./cmd/mail2receipt -count=1 -run TestReceiptEndToEnd -v -timeout 90s
PASS; reported scale 0.77

go test ./... -count=1 -timeout 120s
PASS for all packages

go vet ./...
PASS (no output)

git diff --check
PASS; only line-ending conversion warnings were emitted
```

### Exact Text Geometry Policy

- Retain every visible element client rectangle, including zero-width and zero-height rectangles, in the document-coordinate union. Their anchor coordinates therefore participate in non-finite, negative, and positive-extent checks.
- Build an accepted-element chain in document order. An element is accepted for text traversal only when its parent is accepted, it is visible, it has no rejected opaque/custom rendering surface or known ink-overflow effect, and it has no rejected animation/transition timing.
- For each accepted element, inspect each direct non-whitespace text child with a DOM Range and union every Range client rectangle after adding the current document scroll offsets.
- Nested accepted text is covered when its own parent element is visited. Text below hidden, opaque, custom-rendered, paint-rejected, or timing-rejected ancestors is not traversed; those surfaces already fail closed or do not paint.
- Chromium Range rectangles provide the actual laid-out text geometry, including fixed and transformed ancestor effects. No font-width approximation is used.
- Empty zero-area elements remain accepted when their retained anchor geometry is finite and nonnegative and no other conservative rejection applies.

### Self-review And Residual Concern

- Negative text or anchor geometry fails with the generic cannot-fit error; positive text extents participate in scale fitting and fail when scale would fall below 0.50.
- Existing print-media timing, pseudo, list, opaque-surface, and visual-ink controls remain in force. Network blocking, no-proxy startup, disabled JavaScript, A5 output, inclusive scale limits, and exact one-page validation are unchanged.
- The approved receipt remains accepted at the established scale without fixture disclosure.
- Range geometry measures layout rectangles, while the separately enumerated ink-overflow policy continues to reject known paint that can extend beyond those rectangles.
