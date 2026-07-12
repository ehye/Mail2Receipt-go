//go:build windows

package main

import (
	"bytes"
	"context"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"mail2receipt/internal/app"
	"mail2receipt/internal/browser"
	"mail2receipt/internal/pdfcheck"
)

func TestReceiptEndToEnd(t *testing.T) {
	executable, err := browser.FindExecutable(os.Getenv, func(path string) bool {
		info, statErr := os.Stat(path)
		return statErr == nil && info.Mode().IsRegular()
	})
	if err != nil {
		t.Skip("supported browser not installed")
	}
	if executable == "" {
		t.Fatal("browser discovery returned an empty executable")
	}

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate end-to-end test source")
	}
	fixture := filepath.Join(filepath.Dir(sourceFile), "..", "..", "receipt.eml")
	input, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("required root receipt fixture is unavailable: %v", err)
	}
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "receipt.eml")
	if err := os.WriteFile(inputPath, input, 0o600); err != nil {
		t.Fatal("could not copy receipt fixture")
	}

	var stdout bytes.Buffer
	if code := app.Run(context.Background(), []string{"--force", "--verbose", inputPath}, &stdout, io.Discard); code != 0 {
		t.Fatalf("mail2receipt exit code = %d", code)
	}
	output, err := os.ReadFile(filepath.Join(tempDir, "output.pdf"))
	if err != nil {
		t.Fatal("default output.pdf was not created beside input")
	}
	if err := pdfcheck.Verify(output); err != nil {
		t.Fatalf("output verification failed: %v", err)
	}
	scaleText := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(stdout.String()), "scale:"))
	scale, err := strconv.ParseFloat(scaleText, 64)
	if err != nil {
		t.Fatal("renderer did not report a numeric scale")
	}
	if !validReportedScale(scale) {
		t.Fatalf("renderer scale = %v, want 0.83", scale)
	}
	t.Logf("scale: %.2f", scale)
}

func TestValidReportedScaleRejectsNonFiniteValues(t *testing.T) {
	for _, scale := range []float64{math.NaN(), math.Inf(-1), math.Inf(1)} {
		if validReportedScale(scale) {
			t.Fatalf("validReportedScale(%v) = true, want false", scale)
		}
	}
}

func TestValidReportedScaleAcceptsOnlyFixedScale(t *testing.T) {
	for _, scale := range []float64{0.50, 0.829999, 0.830001, 1.0} {
		if validReportedScale(scale) {
			t.Fatalf("validReportedScale(%v) = true, want false", scale)
		}
	}
	if !validReportedScale(0.83) {
		t.Fatal("validReportedScale(0.83) = false, want true")
	}
}

func validReportedScale(scale float64) bool {
	return !math.IsNaN(scale) && !math.IsInf(scale, 0) && scale == 0.83
}
