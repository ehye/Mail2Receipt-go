package browser

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"mail2receipt/internal/document"
	"mail2receipt/internal/message"
)

func TestRenderBlocksRemoteRequestsAndProducesOnePagePDF(t *testing.T) {
	executable := testBrowser(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()

	body := fmt.Sprintf(`<link rel="stylesheet" href="%[1]s/style.css"><img src="%[1]s/logo.png"><div style="background:url('%[1]s/bg.png')">receipt</div><script src="%[1]s/app.js"></script>`, server.URL)
	result, err := Render(context.Background(), executable, writeHTML(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("remote requests = %d, want 0", got)
	}
	if !bytes.HasPrefix(result.PDF, []byte("%PDF-")) {
		t.Fatal("missing PDF header")
	}
	if pages := len(pdfPagePattern.FindAll(result.PDF, -1)); pages != 1 {
		t.Fatalf("PDF pages = %d, want 1", pages)
	}
}

func TestRenderDisablesJavaScript(t *testing.T) {
	executable := testBrowser(t)
	body := `<div>receipt</div><script>document.body.innerHTML = '<div style="height:1500px">script ran</div>'</script>`
	result, err := Render(context.Background(), executable, writeHTML(t, body))
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if result.Scale != maxScale {
		t.Fatalf("Render() scale = %v, want %v when script is disabled", result.Scale, maxScale)
	}
}

func TestRenderRejectsScaleBelowMinimum(t *testing.T) {
	executable := testBrowser(t)
	_, err := Render(context.Background(), executable, writeHTML(t, `<div style="height:1500px;width:400px">too long</div>`))
	if err == nil || err.Error() != "content cannot fit one A5 page" {
		t.Fatalf("Render() error = %v, want content cannot fit one A5 page", err)
	}
}

func TestRenderLongDocumentUsesIntermediateScale(t *testing.T) {
	executable := testBrowser(t)
	result, err := Render(context.Background(), executable, writeHTML(t, `<div style="height:1000px;width:400px">long receipt</div>`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Scale <= minScale || result.Scale >= maxScale {
		t.Fatalf("Render() scale = %v, want %v < scale < %v", result.Scale, minScale, maxScale)
	}
}

func TestRenderPreparedEmbeddedLogosOffline(t *testing.T) {
	executable := testBrowser(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()

	prepared, err := document.Prepare(message.Document{HTML: []byte(fmt.Sprintf(`<html><head></head><body>
<img src="%s/google-play-crm-lockup-ic-h-transparent-w688px-h140px-2x.png">
<img src="%s/google-play-crm-logo-transparent-w192px-h192px-2x.png">
</body></html>`, server.URL, server.URL))})
	if err != nil {
		t.Fatal(err)
	}
	result, err := Render(context.Background(), executable, writePreparedHTML(t, prepared))
	if err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("remote requests = %d, want 0", got)
	}
	if !bytes.HasPrefix(result.PDF, []byte("%PDF-")) || len(pdfPagePattern.FindAll(result.PDF, -1)) != 1 {
		t.Fatal("prepared embedded logos did not produce a one-page PDF")
	}
}

func TestScaleToFitAcceptsExactMinimumBoundary(t *testing.T) {
	scale, err := scaleToFit(printableWidth/minScale, printableHeight/minScale)
	if err != nil {
		t.Fatalf("scaleToFit() error = %v", err)
	}
	if scale != minScale {
		t.Fatalf("scaleToFit() = %v, want exactly %v", scale, minScale)
	}

	_, err = scaleToFit(printableWidth/minScale, math.Nextafter(printableHeight/minScale, math.Inf(1)))
	if err == nil || err.Error() != "content cannot fit one A5 page" {
		t.Fatalf("scaleToFit() above boundary error = %v, want content cannot fit one A5 page", err)
	}
}

func testBrowser(t *testing.T) string {
	t.Helper()
	executable, err := FindExecutable(os.Getenv, func(path string) bool {
		info, statErr := os.Stat(path)
		return statErr == nil && !info.IsDir()
	})
	if err != nil {
		t.Skipf("browser integration test skipped: %v", err)
	}
	return executable
}

func writeHTML(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "receipt.html")
	html := `<!doctype html><html><head><meta charset="utf-8"><style>html,body{margin:0;padding:0}</style></head><body>` + body + `</body></html>`
	if err := os.WriteFile(path, []byte(html), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writePreparedHTML(t *testing.T, prepared []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "receipt.html")
	if err := os.WriteFile(path, prepared, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
