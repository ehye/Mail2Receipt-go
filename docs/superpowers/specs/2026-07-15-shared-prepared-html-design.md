# Shared Prepared HTML Design

## Goal

Use one prepared HTML representation for both `--base` output and PDF
rendering while preserving receipt content around obsolete email-image
backgrounds.

## Shared Pipeline

The application extracts one HTML MIME body, then calls `document.Prepare`
once. In base mode, the prepared bytes are atomically published as
`output.html`. In PDF mode, the same bytes are written only to a private
workspace and rendered into `output.pdf`.

Base mode retains all document preparation safety transformations but does not
start a browser or create a PDF. PDF mode retains the isolated browser, blocked
HTTP(S), fixed-scale A5 portrait, and per-page verification requirements.

## Legacy Email Images

The legacy image basenames `email_top.png`, `email_mid.png`, and
`email_bottom.png` are matched case-insensitively after removing a URL query or
fragment. A matching CSS resource removes only its declaration from inline
styles or `<style>` rules. Elements, descendants, comments, and non-CSS
attributes remain in the document.

The existing general resource sanitizer independently removes unsupported
load-bearing image references while preserving their containing elements.

## Styling

All `#EDEDED` declarations remain unchanged. No special color filtering is
performed for these values.

## Verification

Tests cover legacy CSS cleanup without content loss, preservation of
`#EDEDED`, prepared base output, and byte-for-byte prepared HTML handoff to
PDF rendering and fixed-scale multi-page PDF output. Full verification includes the fixture-backed receipt E2E test,
the uncached Go suite, static checks, and the stripped release build.
