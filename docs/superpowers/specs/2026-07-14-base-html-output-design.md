# Base HTML Output Design

## Goal

Add a `--base` CLI mode that extracts the selected HTML MIME body from one
`.eml` file and writes the decoded bytes to an HTML file without preparing or
rendering a PDF.

## Command Line

The supported form is:

```text
mail2receipt --base [--force] [--verbose] input.eml [output.html]
```

When the output argument is omitted, base mode writes `output.html` beside the
input file, independent of the process working directory. An explicit output
path is accepted without extension validation, consistent with PDF mode.

`--force` retains its existing overwrite behavior. `--verbose` is accepted in
base mode but produces no output because no rendering scale exists.

The existing PDF command and its defaults remain unchanged.

## Output Semantics

Base mode writes the bytes returned by `document.Prepare` after the existing
MIME extractor selects and transfer-decodes the eligible HTML part, including
Base64, quoted-printable, and supported charset conversion. The prepared HTML
is the same representation PDF mode writes into its private workspace before
rendering.

Prepared base HTML applies the offline asset policy, approved logo and CID
embedding, legacy email-image CSS cleanup, typography handling, and print CSS.
It does not launch a browser, render a PDF, or verify a PDF.

## Application Flow

Argument parsing determines the output default from the selected mode. The
application retains the existing input validation and destination overwrite
check, then invokes the existing size-limited MIME extractor.

After successful extraction, the application prepares HTML once. Base mode
atomically publishes the prepared HTML and returns. PDF mode writes those exact
prepared bytes to its private rendering workspace before browser discovery,
rendering, and PDF verification.

The publication helper remains shared by both modes and uses a format-neutral
temporary filename before atomically replacing the destination.

## Errors And Safety

Base mode uses the existing generic errors for input, extraction, destination,
and publication failures. Errors must not include message contents or parser
details. An existing destination is preserved unless `--force` is supplied,
and a failure must not leave a partial destination.

Raw decoded HTML is untrusted and may retain remote references. The command
only writes it to disk; it does not open or execute the HTML.

## Testing

Application tests will verify:

- the default `output.html` path beside the input;
- explicit output paths;
- publication of the prepared HTML result;
- preparation plus bypass of browser discovery, rendering, and PDF
  verification;
- exact prepared-HTML bytes handed to PDF rendering;
- existing-output rejection and `--force` replacement;
- accepted, silent `--verbose` behavior in base mode;
- extraction or publication failure leaves no partial destination; and
- existing PDF behavior remains unchanged.

Verification will run the focused application tests, the full uncached Go test
suite, and `go vet ./...`.
