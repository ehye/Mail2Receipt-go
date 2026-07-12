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
