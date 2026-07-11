package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const (
	maxScale        = 0.79
	minScale        = 0.50
	printableWidth  = 132 * 96 / 25.4
	printableHeight = 194 * 96 / 25.4
)

var pdfPagePattern = regexp.MustCompile(`/Type\s*/Page(?:\s|[/<])`)

// Result is a rendered one-page PDF and the scale used to produce it.
type Result struct {
	PDF   []byte
	Scale float64
}

type imageState struct {
	mu      sync.Mutex
	urls    map[network.RequestID]string
	pending map[network.RequestID]struct{}
	failure string
	changed chan struct{}
}

// Render loads prepared HTML in an isolated installed browser and prints one A5 page.
func Render(ctx context.Context, executable, htmlPath string) (Result, error) {
	profile, err := os.MkdirTemp("", "mail2receipt-browser-")
	if err != nil {
		return Result{}, fmt.Errorf("create browser profile: %w", err)
	}
	defer os.RemoveAll(profile)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(executable),
		chromedp.UserDataDir(profile),
		chromedp.Flag("headless", "new"),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.WindowSize(int(math.Ceil(printableWidth)), int(math.Ceil(printableHeight))),
	)
	allocCtx, cancelAllocator := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAllocator()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer func() {
		_ = chromedp.Cancel(browserCtx)
		cancelBrowser()
	}()
	if err := chromedp.Run(browserCtx); err != nil {
		return Result{}, errors.New("browser could not start")
	}
	renderCtx, cancelRender := context.WithTimeout(browserCtx, 15*time.Second)
	defer cancelRender()

	images := &imageState{
		urls:    make(map[network.RequestID]string),
		pending: make(map[network.RequestID]struct{}),
		changed: make(chan struct{}, 1),
	}
	chromedp.ListenTarget(renderCtx, images.listen)

	absPath, err := filepath.Abs(htmlPath)
	if err != nil {
		return Result{}, fmt.Errorf("resolve HTML path: %w", err)
	}
	pageURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absPath)}).String()
	if err := chromedp.Run(renderCtx, network.Enable(), chromedp.Navigate(pageURL)); err != nil {
		if failed := images.failedOrPending(); failed != "" {
			return Result{}, fmt.Errorf("image failed: %s", sanitizeURL(failed))
		}
		return Result{}, errors.New("browser could not load receipt")
	}
	if failed := images.wait(renderCtx); failed != "" {
		return Result{}, fmt.Errorf("image failed: %s", sanitizeURL(failed))
	}

	var brokenImages []string
	var dimensions struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}
	if err := chromedp.Run(renderCtx,
		chromedp.Evaluate(`Array.from(document.images).filter(img => !img.complete || img.naturalWidth <= 0).map(img => img.currentSrc || img.src)`, &brokenImages),
		chromedp.Evaluate(`({width: document.documentElement.scrollWidth, height: document.documentElement.scrollHeight})`, &dimensions),
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, _, content, _, _, _, err := page.GetLayoutMetrics().Do(ctx)
			if err == nil && content != nil {
				dimensions.Width = math.Max(dimensions.Width, content.Width)
				dimensions.Height = math.Max(dimensions.Height, content.Height)
			}
			return err
		}),
	); err != nil {
		return Result{}, errors.New("browser could not inspect receipt")
	}
	if len(brokenImages) > 0 {
		return Result{}, fmt.Errorf("image failed: %s", sanitizeURL(brokenImages[0]))
	}

	scale := math.Min(maxScale, math.Min(printableWidth/dimensions.Width, printableHeight/dimensions.Height))
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale < minScale {
		return Result{}, errors.New("content cannot fit one A5 page")
	}

	var pdf []byte
	if err := chromedp.Run(renderCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		pdf, _, err = page.PrintToPDF().
			WithLandscape(false).
			WithDisplayHeaderFooter(false).
			WithPrintBackground(true).
			WithScale(scale).
			WithPaperWidth(5.826772).
			WithPaperHeight(8.267717).
			WithMarginTop(0).
			WithMarginBottom(0).
			WithMarginLeft(0).
			WithMarginRight(0).
			Do(ctx)
		return err
	})); err != nil {
		return Result{}, errors.New("browser could not print receipt")
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || len(pdfPagePattern.FindAll(pdf, -1)) != 1 {
		return Result{}, errors.New("browser did not produce exactly one PDF page")
	}
	return Result{PDF: pdf, Scale: scale}, nil
}

func (s *imageState) listen(event any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch event := event.(type) {
	case *network.EventRequestWillBeSent:
		if event.Type == network.ResourceTypeImage {
			s.urls[event.RequestID] = event.Request.URL
			s.pending[event.RequestID] = struct{}{}
		}
	case *network.EventResponseReceived:
		if event.Type == network.ResourceTypeImage {
			s.urls[event.RequestID] = event.Response.URL
			if event.Response.Status < 200 || event.Response.Status >= 400 {
				s.failure = event.Response.URL
			}
		}
	case *network.EventLoadingFailed:
		if event.Type == network.ResourceTypeImage {
			if s.failure == "" {
				s.failure = s.urls[event.RequestID]
			}
			delete(s.pending, event.RequestID)
		}
	case *network.EventLoadingFinished:
		if _, ok := s.pending[event.RequestID]; ok {
			delete(s.pending, event.RequestID)
		}
	}
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func (s *imageState) wait(ctx context.Context) string {
	for {
		s.mu.Lock()
		if s.failure != "" || len(s.pending) == 0 {
			failure := s.failure
			s.mu.Unlock()
			return failure
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return s.failedOrPending()
		case <-s.changed:
		}
	}
}

func (s *imageState) failedOrPending() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != "" {
		return s.failure
	}
	for id := range s.pending {
		return s.urls[id]
	}
	return ""
}

func sanitizeURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		if parsed != nil && parsed.Scheme != "" {
			return parsed.Scheme + ":"
		}
		return "unknown image"
	}
	return parsed.Host + parsed.EscapedPath()
}
