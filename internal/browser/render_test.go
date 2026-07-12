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

	"github.com/chromedp/chromedp"

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

func TestRenderOrdinaryContentUsesMaximumScale(t *testing.T) {
	executable := testBrowser(t)
	result, err := Render(context.Background(), executable, writeHTML(t, `<div style="width:400px;height:400px">receipt</div>`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Scale != maxScale {
		t.Fatalf("Render() scale = %v, want %v", result.Scale, maxScale)
	}
}

func TestRenderFitsFixedOverflow(t *testing.T) {
	executable := testBrowser(t)
	result, err := Render(context.Background(), executable, writeHTML(t, `<div style="position:fixed;left:600px;top:0;width:100px;height:20px">fixed</div>`))
	if err != nil {
		t.Fatal(err)
	}
	want := printableWidth / 700
	if math.Abs(result.Scale-want) > 0.001 {
		t.Fatalf("Render() scale = %v, want approximately %v", result.Scale, want)
	}
}

func TestRenderFitsTransformedOverflow(t *testing.T) {
	executable := testBrowser(t)
	result, err := Render(context.Background(), executable, writeHTML(t, `<div style="width:100px;height:20px;transform:translateX(600px)">transformed</div>`))
	if err != nil {
		t.Fatal(err)
	}
	want := printableWidth / 700
	if math.Abs(result.Scale-want) > 0.001 {
		t.Fatalf("Render() scale = %v, want approximately %v", result.Scale, want)
	}
}

func TestRenderRejectsUnmeasurablePseudoElementGeometry(t *testing.T) {
	executable := testBrowser(t)
	tests := []struct {
		name     string
		selector string
		css      string
	}{
		{"before symmetry", "::before", `content:"x"`},
		{"fixed transformed", "::after", `content:"x";position:fixed;left:10px;top:10px;width:20px;height:20px;transform:scale(2)`},
		{"fixed content sized", "::after", `content:"content sized";position:fixed;left:10px;top:10px;width:auto;height:auto`},
		{"fixed border and padding", "::after", `content:"x";position:fixed;left:600px;top:0;width:10px;height:10px;border:20px solid;padding:20px`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := fmt.Sprintf(`<style>@media print{#generated%s{%s}}</style><div id="generated"></div>`, test.selector, test.css)
			_, err := Render(context.Background(), executable, writeHTML(t, body))
			if err == nil || err.Error() != "content cannot fit one A5 page" {
				t.Fatalf("Render() error = %v, want content cannot fit one A5 page", err)
			}
		})
	}
}

func TestRenderRejectsUnsupportedRenderingSurfaces(t *testing.T) {
	executable := testBrowser(t)
	tests := []struct {
		name string
		body string
	}{
		{"ordinary list marker", `<ul><li>safe ordinary list</li></ul>`},
		{"SVG marker", `<svg width="100" height="20"><defs><marker id="m"><path d="M0 0L10 5L0 10z"/></marker></defs><path d="M0 10L80 10" marker-end="url(#m)"/></svg>`},
		{"SVG element", `<svg width="100" height="20"><rect width="100" height="20"/></svg>`},
		{"native form control", `<input value="browser rendered">`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Render(context.Background(), executable, writeHTML(t, test.body))
			if err == nil || err.Error() != "content cannot fit one A5 page" {
				t.Fatalf("Render() error = %v, want content cannot fit one A5 page", err)
			}
		})
	}
}

func TestRenderRejectsAdditionalPaintOverflow(t *testing.T) {
	executable := testBrowser(t)
	tests := []struct {
		name string
		body string
	}{
		{"border image outset", `<div style="border:10px solid transparent;border-image:linear-gradient(black,black) 1 / 10px / 20px">outset</div>`},
		{"box reflection", `<div style="-webkit-box-reflect:right 10px">reflection</div>`},
		{"marker ink", `<style>li::marker{text-shadow:20px 0 black}</style><li>marker</li>`},
		{"first letter ink", `<style>p::first-letter{text-shadow:20px 0 black}</style><p>first letter</p>`},
		{"first line ink", `<style>p::first-line{text-shadow:20px 0 black}</style><p>first line</p>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Render(context.Background(), executable, writeHTML(t, test.body))
			if err == nil || err.Error() != "content cannot fit one A5 page" {
				t.Fatalf("Render() error = %v, want content cannot fit one A5 page", err)
			}
		})
	}
}

func TestRenderRejectsUnmeasuredInkOverflow(t *testing.T) {
	executable := testBrowser(t)
	tests := []struct {
		name string
		body string
	}{
		{"box shadow", `<div style="box-shadow:0 0 20px black">shadow</div>`},
		{"text shadow", `<div style="text-shadow:20px 0 black">shadow</div>`},
		{"filter", `<div style="filter:drop-shadow(20px 0 black)">filter</div>`},
		{"outline", `<div style="outline:10px solid black">outline</div>`},
		{"pseudo shadow", `<style>#generated::after{content:"x";box-shadow:0 0 20px black}</style><div id="generated"></div>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Render(context.Background(), executable, writeHTML(t, test.body))
			if err == nil || err.Error() != "content cannot fit one A5 page" {
				t.Fatalf("Render() error = %v, want content cannot fit one A5 page", err)
			}
		})
	}
}

func TestRenderRejectsNegativeVisualOverflow(t *testing.T) {
	executable := testBrowser(t)
	_, err := Render(context.Background(), executable, writeHTML(t, `<div style="width:100px;height:20px;transform:translateX(-1px)">negative</div>`))
	if err == nil || err.Error() != "content cannot fit one A5 page" {
		t.Fatalf("Render() error = %v, want content cannot fit one A5 page", err)
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
<img id="lockup" src="%s/google-play-crm-lockup-ic-h-transparent-w688px-h140px-2x.png">
<img id="logo" src="%s/google-play-crm-logo-transparent-w192px-h192px-2x.png">
</body></html>`, server.URL, server.URL))})
	if err != nil {
		t.Fatal(err)
	}
	var images []struct {
		ID            string  `json:"id"`
		Complete      bool    `json:"complete"`
		NaturalWidth  int64   `json:"naturalWidth"`
		NaturalHeight int64   `json:"naturalHeight"`
		Width         float64 `json:"width"`
		Height        float64 `json:"height"`
	}
	result, err := renderWithInspection(context.Background(), executable, writePreparedHTML(t, prepared), chromedp.Evaluate(`
Array.from(document.querySelectorAll('#lockup, #logo')).map(image => {
  const rect = image.getBoundingClientRect();
  return {id: image.id, complete: image.complete, naturalWidth: image.naturalWidth, naturalHeight: image.naturalHeight, width: rect.width, height: rect.height};
})`, &images))
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 {
		t.Fatalf("inspected images = %d, want 2", len(images))
	}
	for _, image := range images {
		if !image.Complete || image.NaturalWidth <= 0 || image.NaturalHeight <= 0 || image.Width <= 0 || image.Height <= 0 {
			t.Errorf("embedded image %q did not decode and render: %+v", image.ID, image)
		}
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
