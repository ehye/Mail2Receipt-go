# Mail2Receipt-go

Convert the HTML body of a receipt email (`.eml`) into an A5 portrait PDF on Windows.

## Requirements

- Windows
- Microsoft Edge or Google Chrome installed (the program uses an installed browser to render PDFs)
- Either the release executable or Go 1.24 to build it yourself

## Build

```powershell
go build -trimpath -ldflags="-s -w" -o dist/mail2receipt.exe ./cmd/mail2receipt
```

## Usage

Drop one `.eml` file onto `mail2receipt.exe`, or run:

```powershell
.\dist\mail2receipt.exe "C:\Receipts\receipt.eml"
```

With no output argument, the PDF is written as `output.pdf` beside the input email, not in the current working directory.

Choose an output file explicitly:

```powershell
.\dist\mail2receipt.exe "C:\Receipts\receipt.eml" "C:\Receipts\receipt.pdf"
```

Existing output files are protected. Use `--force` to replace one:

```powershell
.\dist\mail2receipt.exe --force "C:\Receipts\receipt.eml"
```

Write the prepared HTML instead of rendering a PDF:

```powershell
.\dist\mail2receipt.exe --base "C:\Receipts\receipt.eml"
```

`--base` writes `output.html` beside the input unless you provide an explicit output path.

## Notes

- One email with an HTML body is processed per invocation.
- Rendering uses installed Edge or Chrome, runs offline, removes unsupported remote images, and blocks HTTP(S) requests.
- PDF output is rendered at a fixed scale of 0.79 and every page is A5 portrait.

## License

[MIT](LICENSE)
