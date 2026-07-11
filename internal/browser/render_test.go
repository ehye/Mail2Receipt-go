package browser

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var onePixelPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0xf0, 0x1f,
	0x00, 0x05, 0x00, 0x01, 0xff, 0x89, 0x99, 0x3d,
	0x1d, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
	0x44, 0xae, 0x42, 0x60, 0x82,
}

func TestRenderLoadsImagesAndProducesOnePagePDF(t *testing.T) {
	executable := testBrowser(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/image.png", servePNG)
	mux.HandleFunc("/background.png", servePNG)
	mux.HandleFunc("/redirect.png", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/image.png", http.StatusFound)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	tests := []struct {
		name string
		body string
	}{
		{
			name: "img, CSS background, and redirect",
			body: fmt.Sprintf(`<img src="%s/image.png"><div style="width:20px;height:20px;background-image:url('%s/background.png')"></div><img src="%s/redirect.png">`, server.URL, server.URL, server.URL),
		},
		{name: "long document scales below initial scale", body: fmt.Sprintf(`<img src="%s/image.png"><div style="height:1100px;width:400px">receipt</div>`, server.URL)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			htmlPath := writeHTML(t, tt.body)
			result, err := Render(context.Background(), executable, htmlPath)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if !strings.HasPrefix(string(result.PDF), "%PDF-") {
				t.Fatalf("Render() PDF prefix = %q", result.PDF[:min(len(result.PDF), 5)])
			}
			if result.Scale < 0.50 || result.Scale > 0.79 {
				t.Fatalf("Render() scale = %v, want 0.50 through 0.79", result.Scale)
			}
			if tt.name == "long document scales below initial scale" && result.Scale >= 0.79 {
				t.Fatalf("Render() scale = %v, want below 0.79", result.Scale)
			}
		})
	}
}

func TestRenderRejectsFailedImagesWithoutLeakingQuery(t *testing.T) {
	executable := testBrowser(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/missing.png", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	})
	mux.HandleFunc("/slow.png", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(16 * time.Second)
		servePNG(w, r)
	})
	mux.HandleFunc("/malformed.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("not an image"))
	})
	mux.HandleFunc("/empty.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedURL := "http://" + listener.Addr().String() + "/unreachable.png"
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		url     string
		wantURL string
	}{
		{name: "HTTP 404", url: server.URL + "/missing.png?account=secret", wantURL: server.URL + "/missing.png"},
		{name: "connection failure", url: closedURL + "?account=secret", wantURL: closedURL},
		{name: "slow image", url: server.URL + "/slow.png?account=secret", wantURL: server.URL + "/slow.png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Render(context.Background(), executable, writeHTML(t, `<img src="`+tt.url+`">`))
			if err == nil {
				t.Fatal("Render() error = nil, want image failure")
			}
			if !strings.Contains(err.Error(), strings.TrimPrefix(tt.wantURL, "http://")) {
				t.Fatalf("Render() error = %q, want host/path %q", err, tt.wantURL)
			}
			if strings.Contains(err.Error(), "account=secret") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("Render() leaked query in error: %q", err)
			}
		})
	}

	cssFailures := []struct {
		name string
		path string
		body func(string) string
	}{
		{
			name: "malformed CSS background",
			path: "/malformed.png",
			body: func(imageURL string) string {
				return `<div style="width:20px;height:20px;background-image:url('` + imageURL + `')"></div>`
			},
		},
		{
			name: "empty pseudo-element background",
			path: "/empty.png",
			body: func(imageURL string) string {
				return `<style>#receipt::before{content:'';display:block;width:20px;height:20px;background-image:url('` + imageURL + `')}</style><div id="receipt"></div>`
			},
		},
	}
	for _, tt := range cssFailures {
		t.Run(tt.name, func(t *testing.T) {
			imageURL := server.URL + tt.path + "?account=secret"
			_, err := Render(context.Background(), executable, writeHTML(t, tt.body(imageURL)))
			if err == nil {
				t.Fatal("Render() error = nil, want CSS image decode failure")
			}
			if !strings.Contains(err.Error(), strings.TrimPrefix(server.URL, "http://")+tt.path) {
				t.Fatalf("Render() error = %q, want sanitized CSS image host/path", err)
			}
			if strings.Contains(err.Error(), "account=secret") || strings.Contains(err.Error(), "secret") {
				t.Fatalf("Render() leaked query in error: %q", err)
			}
		})
	}
}

func TestRenderDisablesJavaScriptWhileLoadingStaticImages(t *testing.T) {
	executable := testBrowser(t)
	var staticRequests atomic.Int32
	var scriptRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/script.png" {
			scriptRequests.Add(1)
		} else {
			staticRequests.Add(1)
		}
		if r.URL.Path == "/static-img.png" {
			time.Sleep(700 * time.Millisecond)
		}
		servePNG(w, r)
	}))
	t.Cleanup(server.Close)

	body := fmt.Sprintf(`<img src="%s/static-img.png"><div style="width:20px;height:20px;background-image:url('%s/static-background.png')"></div><script>
setTimeout(() => {
  const img = document.createElement('img');
  img.src = '%s/script.png';
  document.body.appendChild(img);
}, 400);
</script>`, server.URL, server.URL, server.URL)
	result, err := Render(context.Background(), executable, writeHTML(t, body))
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.HasPrefix(string(result.PDF), "%PDF-") {
		t.Fatalf("Render() PDF prefix = %q", result.PDF[:min(len(result.PDF), 5)])
	}
	if got := staticRequests.Load(); got != 2 {
		t.Fatalf("static image requests = %d, want 2", got)
	}
	if got := scriptRequests.Load(); got != 0 {
		t.Fatalf("script-scheduled image requests = %d, want 0", got)
	}
}

func TestRenderPrintsAfterNearDeadlineImage(t *testing.T) {
	if testing.Short() {
		t.Skip("near-deadline browser integration test")
	}
	executable := testBrowser(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(12 * time.Second)
		servePNG(w, r)
	}))
	t.Cleanup(server.Close)

	result, err := Render(context.Background(), executable, writeHTML(t, `<img src="`+server.URL+`/near-deadline.png">`))
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.HasPrefix(string(result.PDF), "%PDF-") {
		t.Fatalf("Render() PDF prefix = %q", result.PDF[:min(len(result.PDF), 5)])
	}
}

func TestRenderRejectsScaleBelowMinimum(t *testing.T) {
	executable := testBrowser(t)
	_, err := Render(context.Background(), executable, writeHTML(t, `<div style="height:1500px;width:400px">too long</div>`))
	if err == nil || err.Error() != "content cannot fit one A5 page" {
		t.Fatalf("Render() error = %v, want content cannot fit one A5 page", err)
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

func servePNG(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(onePixelPNG)
}
