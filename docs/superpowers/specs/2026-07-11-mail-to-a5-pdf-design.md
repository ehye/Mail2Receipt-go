# Mail to A5 PDF Design

## Goal

Build a Windows-first command-line program that converts the HTML body of an
`.eml` receipt into an A5 portrait PDF. Distribution is a
single Go executable smaller than 20 MB; Microsoft Edge or Google Chrome must
already be installed and is used as the HTML renderer.

## Interface

```text
mail2receipt [--force] [--verbose] input.eml [output.pdf]
```

Dropping one `.eml` file onto the Windows executable is equivalent to passing
that file as the sole argument. If the output argument is omitted, the program
writes `output.pdf` beside the input file, independent of the process working
directory.

The program refuses to replace an existing output unless `--force` is set.
Success returns exit code 0. Invalid input, MIME decoding, missing CID assets,
browser discovery, rendering, or output verification returns a nonzero exit
code and a concise error on stderr.

## Email Extraction

The parser recursively walks MIME entities and handles nested multiparts. For
`multipart/alternative`, it prefers a valid `text/html` body over `text/plain`.
Transfer encoding is selected from MIME headers rather than assuming Base64;
Base64 and quoted-printable are supported. The declared character set is
converted to UTF-8.

The initial release requires an HTML body and does not synthesize HTML from a
plain-text-only message. MIME parts indexed by `Content-ID` are available for
resolving `cid:` image references. Referenced CID images are validated as image
content and replaced with `data:` URLs before rendering.

Input limits protect the command from accidental excessive resource use:

- Maximum `.eml` size: 25 MB
- Maximum individual decoded CID image: 10 MB
- Maximum combined decoded CID image data: 50 MB

## Offline Assets

The executable embeds these two Google Play logo files at build time:

- `google-play-crm-lockup-ic-h-transparent-w688px-h140px-2x.png`
- `google-play-crm-logo-transparent-w192px-h192px-2x.png`

During HTML preparation, an image source whose URL-path basename exactly
matches either filename is replaced with the corresponding PNG `data:` URL.
This applies to both relative and HTTP(S) source values, so rendering does not
depend on the process working directory or companion files.

All other HTTP(S) image references are removed from `src`, `srcset`, HTML
`background` attributes, inline CSS URLs, and `<style>` CSS URLs. Ordinary
hyperlinks are preserved. CID images remain supported because they are already
converted to embedded `data:` URLs.

CSS declarations containing a resource URL whose basename exactly matches
`email_top.png`, `email_mid.png`, or `email_bottom.png` are removed
case-insensitively. This applies to inline CSS and `<style>` blocks, including
recognized nested CSS resource functions. HTML elements, descendants, comments,
and non-CSS attributes that reference those names are preserved; ordinary image
reference normalization still applies independently.

All `#EDEDED` declarations are preserved unchanged.

Before measurement, numeric CSS `font-size` declarations in the top-level
document are increased by 5%. Unsupported or complex `font-size` values remain
unchanged, and typography inside nested `srcdoc` documents is not modified.
Selected footer and legal containers recognized by approved normalized text
prefixes are forced to a 12 px font size and 18 px line height for both the
containers and their descendants. Source-authored copies of the private
emphasis marker are stripped before trusted markers are added to selected
containers.

## Browser

The program discovers Microsoft Edge first and Google Chrome second in their
standard Windows installation locations. It starts the browser headlessly and
controls it through Chrome DevTools Protocol (CDP).

Rendering is fully offline. CDP blocks all HTTP(S) requests before navigation,
and the browser does not configure or inherit a proxy for conversion. Because
the prepared document contains only embedded images, conversion has no image
download timeout or remote-image failure mode. An attempted remote request is
blocked rather than allowed to delay or alter the PDF.

Local file access is restricted to the private temporary conversion directory.
Temporary files are removed after success or failure.

JavaScript execution is disabled before loading email HTML. Receipt rendering
uses the static document only; this prevents untrusted email scripts from
running and makes the set of image resources deterministic before printing.

## A5 Rendering

CDP `PrintToPDF` sets A5 portrait dimensions explicitly (148 x 210 mm), prints
backgrounds, and omits browser headers and footers. Print CSS removes default
body margins and sets the page margin to zero, yielding the full 148 x 210 mm
printable area while preserving the receipt's remaining HTML and inline styles.
CDP PDF margins are also zero.

Printing uses a fixed scale of 0.83. The renderer rejects unsupported or
horizontally overflowing content, and prints vertically overflowing content
across as many A5 portrait pages as necessary. After rendering, the program
verifies the PDF is nonempty, contains at least one page, and uses A5 portrait
dimensions on every page before moving it to the requested destination.

Output is first written in the destination directory under a temporary name,
then atomically renamed to avoid leaving a partial output.

## Components

- `cmd/mail2receipt`: arguments, user-facing errors, and exit status
- `internal/message`: MIME traversal, transfer decoding, charset conversion,
  HTML selection, and CID indexing
- `internal/document`: HTML parsing, CID and embedded-logo replacement, remote
  image removal, receipt-style cleanup, and print CSS injection
- `internal/browser`: browser discovery, offline CDP lifecycle, scale
  measurement, and PDF printing
- `internal/pdfcheck`: PDF size and page-count verification

Package boundaries use small data structures rather than sharing browser or
MIME implementation details across packages.

## Testing

Unit tests cover nested MIME structures, HTML preference, Base64,
quoted-printable, charset conversion, malformed messages, missing HTML,
CID replacement, embedded-logo replacement, legacy CSS declaration cleanup,
remote image removal, style cleanup, top-level typography adjustment,
nested `srcdoc` isolation, selected
footer and legal typography, private-marker stripping, missing CID parts,
resource limits, and output overwrite rules.

Browser integration tests verify HTTP(S) requests are blocked, embedded images
render without network access, fixed-scale multi-page output works, and every
PDF page is A5 portrait. Tests
that require Edge or Chrome skip with an explicit reason if neither browser
exists.

An end-to-end test converts `receipt.eml` and checks that the output is a
nonempty A5 portrait PDF reported at scale 0.83.

Release verification runs all tests, builds a stripped Windows executable, and
fails if its size exceeds 20 MB.

## Initial Scope

The first release supports Windows only, requires installed Edge or Chrome,
requires an HTML MIME body, and processes one input file per invocation. Batch
conversion, macOS/Linux browser discovery, configuration files, GUI support,
and plain-text receipt layout are outside this release.
