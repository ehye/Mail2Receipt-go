# Repository Guidance

- The approved design is `docs/superpowers/specs/2026-07-11-mail-to-a5-pdf-design.md`; treat it as the source of truth until implementation exists.
- The planned application is a Windows-first Go CLI distributed as a stripped executable under 20 MB. It uses installed Edge, then Chrome, through CDP; do not add a bundled browser or language runtime.
- A dropped `.eml` is passed as the sole argument and must create `output.pdf` beside the input file. An explicit output path remains supported.
- Rendering must embed the two approved Google Play logo PNGs, remove other remote images and non-text `#EDEDED` styling, block HTTP(S), and use no proxy. It must produce A5 portrait and verify exactly one PDF page. Start fitting near 0.79 scale and do not silently clip content.
- `receipt.eml`, `receipt.html`, and `receipt.pdf` are the current end-to-end input, decoded-body reference, and expected one-page visual reference. They contain real-looking receipt/account data; do not expose their contents in logs or error messages.
- Build the CLI with `go build ./cmd/mail2receipt`; run focused CLI tests with `go test ./internal/app -v`.
- Run the full test suite with `go test ./... -timeout 120s` and static checks with `go vet ./...`.
