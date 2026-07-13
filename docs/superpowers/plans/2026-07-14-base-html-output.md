# Base HTML Output Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `--base` CLI mode that writes the raw decoded HTML MIME body to an HTML file without running the PDF pipeline.

**Architecture:** Keep MIME parsing in `internal/message` unchanged because it already returns transfer-decoded, charset-converted HTML. Add mode selection to `internal/app`, choose a mode-specific default destination, and return immediately after atomically publishing `message.Document.HTML` in base mode.

**Tech Stack:** Go, standard-library `flag` and filesystem APIs, existing `github.com/emersion/go-message` MIME extraction, Go `testing`.

## Global Constraints

- Base mode writes `message.Document.HTML` byte-for-byte with no document preparation.
- With no explicit destination, base mode writes `output.html` beside the `.eml` input.
- An explicit destination has no extension validation.
- `--force` keeps the existing atomic replacement behavior.
- `--verbose` is accepted and silent in base mode.
- Base mode must not create a rendering workspace, prepare HTML, discover or launch a browser, render a PDF, or verify a PDF.
- User-facing errors must not expose email contents or parser details.
- Existing PDF behavior, including `output.pdf`, must remain unchanged.
- Do not read, print, quote, or expose the contents of `receipt.eml`, `receipt.html`, or `receipt.pdf`.

---

### Task 1: Add The Base HTML CLI Mode

**Files:**
- Modify: `internal/app/app_test.go`
- Modify: `internal/app/app.go:40-114`

**Interfaces:**
- Consumes: `message.Document.HTML []byte` from the existing `runner.extract` function.
- Produces: CLI option `--base`; default base destination `output.html`; unchanged `runner.run(context.Context, []string, io.Writer, io.Writer) int` signature.

- [ ] **Step 1: Add failing tests for raw output and PDF-pipeline bypass**

Add these tests to `internal/app/app_test.go`:

```go
func TestBaseWritesRawDecodedHTMLAndBypassesPDFPipeline(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "receipt.eml")
	writeInput(t, input)
	rawHTML := []byte("<html><body><img src=\"https://example.invalid/private.png\"></body></html>")
	app := successRunner()
	app.extract = func(io.Reader, int64) (message.Document, error) {
		return message.Document{HTML: rawHTML}, nil
	}
	app.prepare = func(message.Document) ([]byte, error) {
		t.Fatal("prepare called in base mode")
		return nil, nil
	}
	app.findBrowser = func(func(string) string, func(string) bool) (string, error) {
		t.Fatal("findBrowser called in base mode")
		return "", nil
	}
	app.render = func(context.Context, string, string) (browser.Result, error) {
		t.Fatal("render called in base mode")
		return browser.Result{}, nil
	}
	app.verify = func([]byte) error {
		t.Fatal("verify called in base mode")
		return nil
	}
	var stdout bytes.Buffer

	if code := app.run(context.Background(), []string{"--base", "--verbose", input}, &stdout, io.Discard); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	got, err := os.ReadFile(filepath.Join(dir, "output.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, rawHTML) {
		t.Fatalf("output = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "output.pdf")); !os.IsNotExist(err) {
		t.Fatalf("PDF output exists: %v", err)
	}
}
```

- [ ] **Step 2: Add failing tests for explicit output and overwrite behavior**

Add these tests to `internal/app/app_test.go`:

```go
func TestBaseExplicitOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "receipt.eml")
	output := filepath.Join(dir, "decoded.custom")
	writeInput(t, input)
	app := successRunner()

	if code := app.run(context.Background(), []string{"--base", input, output}, io.Discard, io.Discard); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if got, err := os.ReadFile(output); err != nil || string(got) != "<html><head></head><body>ok</body></html>" {
		t.Fatalf("output = %q, %v", got, err)
	}
}

func TestBaseExistingOutputRequiresForce(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "receipt.eml")
	output := filepath.Join(dir, "output.html")
	writeInput(t, input)
	if err := os.WriteFile(output, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := successRunner()

	if code := app.run(context.Background(), []string{"--base", input}, io.Discard, io.Discard); code == 0 {
		t.Fatal("exit = 0 without --force")
	}
	if got, err := os.ReadFile(output); err != nil || string(got) != "old" {
		t.Fatalf("output = %q, %v", got, err)
	}
	if code := app.run(context.Background(), []string{"--base", "--force", input}, io.Discard, io.Discard); code != 0 {
		t.Fatalf("force exit = %d", code)
	}
	if got, err := os.ReadFile(output); err != nil || string(got) != "<html><head></head><body>ok</body></html>" {
		t.Fatalf("output = %q, %v", got, err)
	}
}
```

- [ ] **Step 3: Run the focused tests and confirm the new behavior is absent**

Run:

```powershell
go test ./internal/app -run 'TestBase' -count=1 -v
```

Expected: FAIL because `--base` is not registered, so each invocation returns exit code 1 and creates no HTML destination.

- [ ] **Step 4: Register `--base` and select the mode-specific default output**

In `internal/app/app.go`, add the flag beside the existing options and update argument handling:

```go
base := flags.Bool("base", false, "write decoded HTML without rendering a PDF")
force := flags.Bool("force", false, "replace an existing output")
verbose := flags.Bool("verbose", false, "report rendering details")
```

Use a format-neutral usage message and select the default destination before honoring an explicit path:

```go
if len(positional) < 1 || len(positional) > 2 {
	return fail(stderr, "usage: mail2receipt [--base] [--force] [--verbose] input.eml [output]")
}
input := positional[0]
if !strings.EqualFold(filepath.Ext(input), ".eml") {
	return fail(stderr, "input must be an .eml file")
}
outputName := "output.pdf"
if *base {
	outputName = "output.html"
}
output := filepath.Join(filepath.Dir(input), outputName)
if len(positional) == 2 {
	output = positional[1]
}
```

- [ ] **Step 5: Publish decoded HTML before entering the PDF pipeline**

Move the existing `os.MkdirTemp` block so it occurs after extraction and after this new early-return block:

```go
doc, err := app.extract(in, maxMessageBytes)
if err != nil {
	return fail(stderr, "could not read email message")
}
if *base {
	if err := publish(output, doc.HTML, *force); err != nil {
		return fail(stderr, "could not write output")
	}
	return 0
}

workspace, err := os.MkdirTemp("", "mail2receipt-")
if err != nil {
	return fail(stderr, "could not create private workspace")
}
defer os.RemoveAll(workspace)
```

Keep the existing preparation, browser, rendering, verification, publication, and verbose-scale logic after this block unchanged.

- [ ] **Step 6: Make the shared publication temporary filename format-neutral**

Change the first line of `publish` that creates the temporary file:

```go
temporary, err := os.CreateTemp(filepath.Dir(destination), ".mail2receipt-*")
```

This does not alter atomic replacement behavior; it only removes the misleading `.pdf` suffix when publishing HTML.

- [ ] **Step 7: Format and run focused app tests**

Run:

```powershell
gofmt -w internal/app/app.go internal/app/app_test.go
```

Expected: PASS, including all `TestBase...` tests and the pre-existing PDF-mode tests.

- [ ] **Step 8: Run repository verification**

Run:

```powershell
go test ./... -count=1 -timeout 120s
```

Expected: both commands exit 0. The full suite confirms the unchanged MIME selection, document preparation, browser rendering, PDF verification, and existing CLI behavior.

- [ ] **Step 9: Inspect the final diff**

Run:

```powershell
git diff --check
```

Expected: `git diff --check` exits 0. The diff contains only the approved design, this plan, app tests, and the minimal app implementation; it contains no fixture contents.
