package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mail2receipt/internal/browser"
	"mail2receipt/internal/message"
)

func TestRunRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"too many arguments", []string{"a.eml", "a.pdf", "extra"}},
		{"non eml input", []string{"receipt.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := Run(context.Background(), tt.args, io.Discard, &stderr); code == 0 {
				t.Fatal("exit = 0")
			}
			if stderr.Len() == 0 {
				t.Fatal("stderr is empty")
			}
		})
	}
}

func TestDefaultOutputIsBesideInput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "receipt.eml")
	writeInput(t, input)
	app := successRunner()

	if code := app.run(context.Background(), []string{input}, io.Discard, io.Discard); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "output.pdf")); err != nil || string(data) != "pdf" {
		t.Fatalf("output = %q, %v", data, err)
	}
}

func TestExplicitOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "receipt.eml")
	output := filepath.Join(dir, "chosen.pdf")
	writeInput(t, input)
	app := successRunner()

	if code := app.run(context.Background(), []string{input, output}, io.Discard, io.Discard); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
}

func TestExistingOutputRequiresForce(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "receipt.eml")
	output := filepath.Join(dir, "output.pdf")
	writeInput(t, input)
	if err := os.WriteFile(output, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := successRunner()

	if code := app.run(context.Background(), []string{input}, io.Discard, io.Discard); code == 0 {
		t.Fatal("exit = 0 without --force")
	}
	if got, _ := os.ReadFile(output); string(got) != "old" {
		t.Fatalf("output changed to %q", got)
	}
	if code := app.run(context.Background(), []string{"--force", input}, io.Discard, io.Discard); code != 0 {
		t.Fatalf("force exit = %d", code)
	}
	if got, _ := os.ReadFile(output); string(got) != "pdf" {
		t.Fatalf("output = %q", got)
	}
}

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

func TestBaseExtractionFailureLeavesNoOutputAndDoesNotLeakError(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "receipt.eml")
	output := filepath.Join(dir, "output.html")
	writeInput(t, input)
	app := successRunner()
	app.extract = func(io.Reader, int64) (message.Document, error) {
		return message.Document{}, errors.New("decoded account 123")
	}
	var stderr bytes.Buffer

	if code := app.run(context.Background(), []string{"--base", input}, io.Discard, &stderr); code == 0 {
		t.Fatal("exit = 0")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("destination exists: %v", err)
	}
	if strings.Contains(stderr.String(), "account") || strings.Contains(stderr.String(), "123") {
		t.Fatalf("sensitive error leaked: %q", stderr.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(input) {
		t.Fatalf("unexpected files: %v", entries)
	}
}

func TestBasePublicationFailureCleansTemporaryFileAndPreservesDestination(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "receipt.eml")
	output := filepath.Join(dir, "output.html")
	sentinel := filepath.Join(output, "sentinel")
	writeInput(t, input)
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := successRunner()

	if code := app.run(context.Background(), []string{"--base", "--force", input, output}, io.Discard, io.Discard); code == 0 {
		t.Fatal("exit = 0")
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "old" {
		t.Fatalf("sentinel = %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".mail2receipt-") {
			t.Fatalf("temporary artifact remains: %s", entry.Name())
		}
	}
}

func TestVerboseReportsScale(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "receipt.eml")
	writeInput(t, input)
	app := successRunner()
	var stdout bytes.Buffer

	if code := app.run(context.Background(), []string{"--verbose", input}, &stdout, io.Discard); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout.String(), "1.00") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestTemporaryDirectoryIsCleanedAfterRenderFailure(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "receipt.eml")
	writeInput(t, input)
	app := successRunner()
	var htmlPath string
	app.render = func(_ context.Context, _, path string) (browser.Result, error) {
		htmlPath = path
		return browser.Result{}, errors.New("https://secret.example/?account=private")
	}
	var stderr bytes.Buffer

	if code := app.run(context.Background(), []string{input}, io.Discard, &stderr); code == 0 {
		t.Fatal("exit = 0")
	}
	if _, err := os.Stat(filepath.Dir(htmlPath)); !os.IsNotExist(err) {
		t.Fatalf("temporary directory remains: %v", err)
	}
	if strings.Contains(stderr.String(), "secret") || strings.Contains(stderr.String(), "private") {
		t.Fatalf("sensitive error leaked: %q", stderr.String())
	}
}

func TestVerificationFailureLeavesNoPartialDestination(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "receipt.eml")
	output := filepath.Join(dir, "output.pdf")
	writeInput(t, input)
	app := successRunner()
	app.verify = func([]byte) error { return errors.New("decoded account 123") }
	var stderr bytes.Buffer

	if code := app.run(context.Background(), []string{input}, io.Discard, &stderr); code == 0 {
		t.Fatal("exit = 0")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("destination exists: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("unexpected files: %v", entries)
	}
	if strings.Contains(stderr.String(), "account") || strings.Contains(stderr.String(), "123") {
		t.Fatalf("sensitive error leaked: %q", stderr.String())
	}
}

func TestForceVerificationFailurePreservesExistingOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "receipt.eml")
	output := filepath.Join(dir, "output.pdf")
	writeInput(t, input)
	sentinel := []byte("existing destination sentinel")
	if err := os.WriteFile(output, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	app := successRunner()
	app.verify = func([]byte) error { return errors.New("verification failed") }

	if code := app.run(context.Background(), []string{"--force", input}, io.Discard, io.Discard); code == 0 {
		t.Fatal("exit = 0")
	}
	if got, err := os.ReadFile(output); err != nil || !bytes.Equal(got, sentinel) {
		t.Fatalf("output = %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("unexpected files: %v", entries)
	}
}

func successRunner() runner {
	return runner{
		extract: func(io.Reader, int64) (message.Document, error) {
			return message.Document{HTML: []byte("<html><head></head><body>ok</body></html>")}, nil
		},
		prepare:     func(message.Document) ([]byte, error) { return []byte("prepared"), nil },
		findBrowser: func(func(string) string, func(string) bool) (string, error) { return "browser", nil },
		render: func(context.Context, string, string) (browser.Result, error) {
			return browser.Result{PDF: []byte("pdf"), Scale: 1.0}, nil
		},
		verify: func([]byte) error { return nil },
	}
}

func writeInput(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("message"), 0o600); err != nil {
		t.Fatal(err)
	}
}
