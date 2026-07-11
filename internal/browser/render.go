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

	"github.com/chromedp/cdproto/emulation"
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

// Render loads prepared HTML in an isolated installed browser and prints one A5 page.
func Render(ctx context.Context, executable, htmlPath string) (Result, error) {
	return renderWithInspection(ctx, executable, htmlPath, nil)
}

func renderWithInspection(ctx context.Context, executable, htmlPath string, inspection chromedp.Action) (Result, error) {
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
		chromedp.Flag("no-proxy-server", true),
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

	absPath, err := filepath.Abs(htmlPath)
	if err != nil {
		return Result{}, fmt.Errorf("resolve HTML path: %w", err)
	}
	pageURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absPath)}).String()
	if err := chromedp.Run(browserCtx,
		network.Enable(),
		network.SetBlockedURLs([]string{"http://*", "https://*"}),
		page.Enable(),
		emulation.SetScriptExecutionDisabled(true),
		chromedp.Navigate(pageURL),
	); err != nil {
		return Result{}, errors.New("browser could not load receipt")
	}
	if inspection != nil {
		if err := chromedp.Run(browserCtx, inspection); err != nil {
			return Result{}, errors.New("browser could not inspect receipt")
		}
	}

	var dimensions struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}
	if err := chromedp.Run(browserCtx,
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

	scale, err := scaleToFit(dimensions.Width, dimensions.Height)
	if err != nil {
		return Result{}, err
	}

	var pdf []byte
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
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

func scaleToFit(width, height float64) (float64, error) {
	scale := math.Min(maxScale, math.Min(printableWidth/width, printableHeight/height))
	if math.IsNaN(scale) || math.IsInf(scale, 0) || scale < minScale {
		return 0, errors.New("content cannot fit one A5 page")
	}
	return scale, nil
}
