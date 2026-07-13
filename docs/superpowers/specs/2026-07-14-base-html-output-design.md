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

Base mode writes `message.Document.HTML` exactly as returned by the existing
MIME extractor. The extractor selects the same eligible HTML MIME part used by
PDF mode and applies MIME transfer decoding, including Base64 and
quoted-printable decoding, plus supported charset conversion.

Base mode does not run document preparation. It therefore does not sanitize
HTML, rewrite CID or remote references, replace logos, change typography, add
print CSS, launch a browser, render a PDF, or verify a PDF. CID assets are not
written separately or embedded into the HTML.

## Application Flow

Argument parsing determines the output default from the selected mode. The
application retains the existing input validation and destination overwrite
check, then invokes the existing size-limited MIME extractor.

After successful extraction, base mode atomically publishes the extracted HTML
and returns. The private rendering workspace is created only for PDF mode, so
base mode cannot invoke document preparation, browser discovery, rendering, or
PDF verification.

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
- byte-for-byte publication of the extractor's HTML result;
- bypass of preparation, browser discovery, rendering, and PDF verification;
- existing-output rejection and `--force` replacement;
- accepted, silent `--verbose` behavior in base mode;
- extraction or publication failure leaves no partial destination; and
- existing PDF behavior remains unchanged.

Verification will run the focused application tests, the full uncached Go test
suite, and `go vet ./...`.
