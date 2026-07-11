package browser

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
