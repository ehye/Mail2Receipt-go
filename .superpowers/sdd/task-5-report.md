# Task 5: PDF Verification Report

## Files Changed

- `internal/pdfcheck/pdfcheck.go`: added `Verify(data []byte) error` using `rsc.io/pdf`, requiring exactly one A5 portrait page within one PDF point.
- `internal/pdfcheck/pdfcheck_test.go`: added generated minimal PDF fixtures covering empty input, invalid PDF data, zero pages, multiple pages, wrong media box, and one valid A5 portrait page.
- `go.mod`: added the direct `rsc.io/pdf v0.1.1` dependency.
- `go.sum`: added checksums for `rsc.io/pdf v0.1.1`.

No existing `message`, `document`, or `browser` files were changed. Receipt fixture contents were not read or exposed.

## RED

Command:

```text
go test ./internal/pdfcheck -v
```

Output:

```text
# mail2receipt/internal/pdfcheck [mail2receipt/internal/pdfcheck.test]
internal\pdfcheck\pdfcheck_test.go:31:12: undefined: Verify
internal\pdfcheck\pdfcheck_test.go:38:9: undefined: Verify
FAIL    mail2receipt/internal/pdfcheck [build failed]
FAIL
```

The failure was expected and was caused only by the missing production API.

## GREEN

Command:

```text
gofmt -w internal/pdfcheck/pdfcheck.go internal/pdfcheck/pdfcheck_test.go && go test ./internal/pdfcheck -v
```

Output:

```text
=== RUN   TestVerifyRejectsEmptyInput
--- PASS: TestVerifyRejectsEmptyInput (0.00s)
=== RUN   TestVerifyRejectsInvalidPDF
--- PASS: TestVerifyRejectsInvalidPDF (0.00s)
=== RUN   TestVerifyRejectsZeroPages
--- PASS: TestVerifyRejectsZeroPages (0.00s)
=== RUN   TestVerifyRejectsMultiplePages
--- PASS: TestVerifyRejectsMultiplePages (0.00s)
=== RUN   TestVerifyRejectsWrongMediaBox
--- PASS: TestVerifyRejectsWrongMediaBox (0.00s)
=== RUN   TestVerifyAcceptsOneA5PortraitPage
--- PASS: TestVerifyAcceptsOneA5PortraitPage (0.00s)
PASS
ok      mail2receipt/internal/pdfcheck  0.885s
```

After `go mod tidy`, the same focused suite passed again.

## Full Verification

Command: `go test ./...`

```text
ok      mail2receipt/internal/browser   10.296s
ok      mail2receipt/internal/document  1.016s
ok      mail2receipt/internal/message   (cached)
ok      mail2receipt/internal/pdfcheck  0.630s
```

Command: `go vet ./...`

```text
(no output; exit code 0)
```

Command: `git diff --check`

```text
(exit code 0; only Git's Windows line-ending notices for go.mod and go.sum)
```

## Self-Review

- The implementation follows the brief's required `pdf.NewReader(bytes.NewReader(data), int64(len(data)))` call.
- Errors have distinct `invalid PDF`, `page count`, and `paper dimensions` categories.
- Media-box width and height account for non-zero lower-left coordinates.
- The one-point tolerance is inclusive because only differences greater than one are rejected.
- Fixtures are generated from transparent minimal PDF objects; no opaque binaries or receipt data are used.
- Scope is limited to the new package and required module dependency.

## Commit

Implementation commit hash: `fd42e76c04760c122835b95a45c586829c6fe53c`

## Review Fixes

### Files Changed

- `internal/pdfcheck/pdfcheck.go`: replaced the wrapped parser error with the stable diagnostic `invalid PDF`, preventing parser details or input-derived content from reaching callers.
- `internal/pdfcheck/pdfcheck_test.go`: added the sensitive-marker regression and coverage for inclusive one-point boundaries, beyond-tolerance dimensions, landscape A5, and non-zero MediaBox origins.

### RED

Command:

```text
go test ./internal/pdfcheck -run TestVerifyDoesNotExposeInvalidPDFContent -v
```

Output:

```text
=== RUN   TestVerifyDoesNotExposeInvalidPDFContent
    pdfcheck_test.go:23: Verify() error = "invalid PDF: not a PDF file: missing %%EOF", want stable "invalid PDF"
--- FAIL: TestVerifyDoesNotExposeInvalidPDFContent (0.00s)
FAIL
FAIL    mail2receipt/internal/pdfcheck  0.894s
FAIL
```

The regression failed because the parser cause remained visible in the public error.

### GREEN

Command:

```text
gofmt -w internal/pdfcheck/pdfcheck.go && go test ./internal/pdfcheck -v
```

Result: all 12 top-level tests passed, including both tolerance-boundary subtests; package result was `ok mail2receipt/internal/pdfcheck 0.956s`.

### Full Verification

Command:

```text
go test ./...
```

Output:

```text
ok      mail2receipt/internal/browser   (cached)
ok      mail2receipt/internal/document  (cached)
ok      mail2receipt/internal/message   (cached)
ok      mail2receipt/internal/pdfcheck  0.456s
```

Command: `git diff --check`

Result: exit code 0, with only Git's Windows line-ending notices for the two modified Go files.

### Fix Self-Review

- The public invalid-PDF error is an exact constant and neither wraps nor exposes the `rsc.io/pdf` cause.
- The regression includes a distinctive marker and checks both the exact stable category and marker absence.
- Boundary tests cover simultaneous width and height differences of exactly minus and plus one point.
- A width difference of 1.001 points is rejected, proving the inclusive limit does not extend beyond one point.
- Landscape dimensions are rejected and translated MediaBox coordinates are accepted based on width/height differences.
- Changes are scoped to `internal/pdfcheck`; unrelated untracked files remain untouched.

Review fix commit hash: `0faed93b865ee9b6f5f4a22ebaf92470b52ea877`
