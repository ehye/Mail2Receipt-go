# Repository Guidance

- The approved design is `docs/superpowers/specs/2026-07-11-mail-to-a5-pdf-design.md`; preserve its security and output constraints.
- The application is Windows-only and requires an installed Microsoft Edge or Google Chrome. Browser discovery prefers Edge, then Chrome. Do not add a bundled browser or language runtime.
- Dropping one `.eml` onto `mail2receipt.exe` passes it as the sole argument and creates `output.pdf` beside the input, independent of the working directory. An explicit output path and `--force` remain supported.
- Rendering embeds the two approved Google Play logo PNGs, removes other remote images and non-text `#EDEDED` styling, blocks all HTTP(S) requests, and uses no proxy.
- Output must be A5 portrait and exactly one PDF page. CSS owns the 2 mm page margin, leaving a 144 x 206 mm printable area, so CDP PDF margins remain zero. Fitting starts at and never exceeds scale `1.0`, never goes below `0.50`, and must fail rather than silently clip content.
- Before measurement, numeric CSS `font-size` declarations in the top-level document increase 5%; unsupported or complex values and nested `srcdoc` typography remain unchanged. Selected footer and legal containers recognized by approved normalized text prefixes are forced to `12px` font size and `18px` line height for themselves and descendants. Source-authored copies of the private emphasis marker are stripped before trusted markers are added.
- `receipt.eml` is the end-to-end input, `receipt.html` is the decoded-body reference, and `receipt.pdf` is the approved one-page visual reference. They contain real-looking receipt/account data; never print, log, quote, commit, or expose their contents.
- Run the receipt E2E test with `go test ./cmd/mail2receipt -run TestReceiptEndToEnd -v -timeout 90s`.
- Build the stripped release executable with `go build -trimpath -ldflags="-s -w" -o dist/mail2receipt.exe ./cmd/mail2receipt`.
- Verify the release size in PowerShell with `(Get-Item -LiteralPath "dist/mail2receipt.exe").Length -lt 20MB`; it must return `True`.
- Verify drag-and-drop-equivalent behavior with `dist\mail2receipt.exe receipt.eml`; remove the generated root `output.pdf` afterward only when it was created by the verification.
- Run the full uncached suite with `go test ./... -count=1 -timeout 120s` and static checks with `go vet ./...`.
