package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mail2receipt/internal/browser"
	"mail2receipt/internal/document"
	"mail2receipt/internal/message"
	"mail2receipt/internal/pdfcheck"
)

const maxMessageBytes = 25 << 20

type runner struct {
	extract     func(io.Reader, int64) (message.Document, error)
	prepare     func(message.Document) ([]byte, error)
	findBrowser func(func(string) string, func(string) bool) (string, error)
	render      func(context.Context, string, string) (browser.Result, error)
	verify      func([]byte) error
}

// Run converts one .eml receipt and returns a process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return (runner{
		extract:     message.Extract,
		prepare:     document.Prepare,
		findBrowser: browser.FindExecutable,
		render:      browser.Render,
		verify:      pdfcheck.Verify,
	}).run(ctx, args, stdout, stderr)
}

func (app runner) run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("mail2receipt", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	base := flags.Bool("base", false, "write decoded HTML without rendering a PDF")
	force := flags.Bool("force", false, "replace an existing output")
	verbose := flags.Bool("verbose", false, "report rendering details")
	if err := flags.Parse(args); err != nil {
		return fail(stderr, "invalid command line")
	}
	positional := flags.Args()
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

	in, err := os.Open(input)
	if err != nil {
		return fail(stderr, "could not open input")
	}
	defer in.Close()
	if info, err := in.Stat(); err != nil || !info.Mode().IsRegular() {
		return fail(stderr, "input is not a readable file")
	}
	if _, err := os.Stat(output); err == nil && !*force {
		return fail(stderr, "output already exists; use --force to replace it")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(stderr, "could not inspect output")
	}

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

	html, err := app.prepare(doc)
	if err != nil {
		return fail(stderr, "could not prepare receipt")
	}
	htmlPath := filepath.Join(workspace, "receipt.html")
	if err := os.WriteFile(htmlPath, html, 0o600); err != nil {
		return fail(stderr, "could not prepare private workspace")
	}
	executable, err := app.findBrowser(os.Getenv, fileExists)
	if err != nil {
		return fail(stderr, "supported browser was not found")
	}
	result, err := app.render(ctx, executable, htmlPath)
	if err != nil {
		return fail(stderr, "could not render receipt")
	}
	if err := app.verify(result.PDF); err != nil {
		return fail(stderr, "rendered PDF failed verification")
	}
	if err := publish(output, result.PDF, *force); err != nil {
		return fail(stderr, "could not write output")
	}
	if *verbose {
		fmt.Fprintf(stdout, "scale: %.2f\n", result.Scale)
	}
	return 0
}

func publish(destination string, data []byte, force bool) error {
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".mail2receipt-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return atomicReplace(temporaryPath, destination, force)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func fail(stderr io.Writer, text string) int {
	fmt.Fprintln(stderr, "mail2receipt:", text)
	return 1
}
