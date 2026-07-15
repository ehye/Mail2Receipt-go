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
	fixedScale      = 0.79
	printableWidth  = 148 * 96 / 25.4
	printableHeight = 210 * 96 / 25.4
)

var pdfPagePattern = regexp.MustCompile(`/Type\s*/Page(?:\s|[/<])`)

// Result is a rendered PDF and the scale used to produce it.
type Result struct {
	PDF   []byte
	Scale float64
}

// Render loads prepared HTML in an isolated installed browser and prints A5 pages.
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
		chromedp.WindowSize(int(math.Floor(printableWidth)), int(math.Floor(printableHeight))),
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
		emulation.SetEmulatedMedia().WithMedia("print"),
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
		Valid  bool    `json:"valid"`
	}
	if err := chromedp.Run(browserCtx,
		chromedp.Evaluate(`(() => {
  const root = document.documentElement;
  const body = document.body;
  const scrollX = window.scrollX;
  const scrollY = window.scrollY;
  let minLeft = 0;
  let minTop = 0;
  let maxRight = Math.max(root.scrollWidth, body ? body.scrollWidth : 0);
  let maxBottom = Math.max(root.scrollHeight, body ? body.scrollHeight : 0);
  let valid = [scrollX, scrollY, maxRight, maxBottom].every(Number.isFinite);

  const include = (left, top, right, bottom) => {
    if (![left, top, right, bottom].every(Number.isFinite)) {
      valid = false;
      return;
    }
    minLeft = Math.min(minLeft, left);
    minTop = Math.min(minTop, top);
    maxRight = Math.max(maxRight, right);
    maxBottom = Math.max(maxBottom, bottom);
  };
  const visible = style => style.display !== 'none' &&
    style.visibility !== 'hidden' && style.visibility !== 'collapse' &&
    Number.parseFloat(style.opacity || '1') !== 0;
  const hasNonzeroComponent = value => value.split(/\s+/).some(component => {
    const number = Number.parseFloat(component);
    return !Number.isFinite(number) || number !== 0;
  });
  const hasInkOverflow = style => style.boxShadow !== 'none' ||
    style.textShadow !== 'none' || style.filter !== 'none' ||
    (style.outlineStyle !== 'none' && Number.parseFloat(style.outlineWidth) !== 0) ||
    Number.parseFloat(style.webkitTextStrokeWidth || '0') !== 0 ||
    (style.stroke !== 'none' && Number.parseFloat(style.strokeWidth || '0') !== 0) ||
    hasNonzeroComponent(style.borderImageOutset || '0') ||
    (style.webkitBoxReflect || 'none') !== 'none';
  const opaqueTags = new Set([
    'audio', 'button', 'canvas', 'embed', 'iframe', 'input', 'meter', 'object',
    'optgroup', 'option', 'progress', 'select', 'textarea', 'video'
  ]);
  const opaqueNamespace = element => element.namespaceURI === 'http://www.w3.org/2000/svg' ||
    element.namespaceURI === 'http://www.w3.org/1998/Math/MathML';
  const commaList = value => {
    if (typeof value !== 'string') return null;
    const parts = value.split(',').map(part => part.trim());
    return parts.length > 0 && parts.every(Boolean) ? parts : null;
  };
  const timeList = value => {
    const parts = commaList(value);
    if (parts === null) return null;
    let nonzero = false;
    for (const part of parts) {
      const match = part.match(/^([+-]?(?:\d+(?:\.\d*)?|\.\d+))(ms|s)$/i);
      if (match === null) return null;
      const amount = Number(match[1]);
      if (!Number.isFinite(amount)) return null;
      if (amount !== 0) nonzero = true;
    }
    return nonzero;
  };
  const activeNames = value => {
    const names = commaList(value);
    return names === null || names.some(name => name.toLowerCase() !== 'none');
  };
  const unsafeTransition = (propertyValue, durationValue, delayValue) => {
    const properties = commaList(propertyValue);
    const duration = timeList(durationValue);
    const delay = timeList(delayValue);
    if (properties === null || duration === null || delay === null) return true;
    return properties.some(property => property.toLowerCase() !== 'none') && (duration || delay);
  };
  const hasTimeVariation = style => activeNames(style.animationName) ||
    (typeof style.webkitAnimationName === 'string' && activeNames(style.webkitAnimationName)) ||
    unsafeTransition(style.transitionProperty, style.transitionDuration, style.transitionDelay) ||
    (typeof style.webkitTransitionProperty === 'string' &&
      unsafeTransition(style.webkitTransitionProperty, style.webkitTransitionDuration, style.webkitTransitionDelay));

  for (const element of document.querySelectorAll('*')) {
    const style = getComputedStyle(element);
    const timeVarying = hasTimeVariation(style);
    const surfaceRejected = style.display === 'list-item' || opaqueNamespace(element) ||
      opaqueTags.has(element.localName) || hasInkOverflow(style);
    if (element.shadowRoot !== null) valid = false;
    if (timeVarying) valid = false;
    if (visible(style)) {
      if (surfaceRejected) valid = false;
      for (const rect of element.getClientRects()) {
        include(rect.left + scrollX, rect.top + scrollY, rect.right + scrollX, rect.bottom + scrollY);
      }
    }

    for (const pseudo of ['::before', '::after']) {
      const pseudoStyle = getComputedStyle(element, pseudo);
      if (hasTimeVariation(pseudoStyle)) valid = false;
      if (!visible(pseudoStyle) || pseudoStyle.content === 'none' || pseudoStyle.content === 'normal') continue;

      // CSSOM does not expose pseudo-element client rectangles. Reject generated
      // content rather than infer bounds that can omit transforms or visual ink.
      valid = false;
    }

    for (const pseudo of ['::marker', '::first-letter', '::first-line']) {
      const pseudoStyle = getComputedStyle(element, pseudo);
      if (hasTimeVariation(pseudoStyle)) valid = false;
      if (visible(pseudoStyle) && hasInkOverflow(pseudoStyle)) valid = false;
    }
  }

  const textNodes = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  for (let node = textNodes.nextNode(); node !== null; node = textNodes.nextNode()) {
    if (!/\S/.test(node.data)) continue;
    const range = document.createRange();
    range.selectNodeContents(node);
    const textStyle = getComputedStyle(node.parentElement);
    if (textStyle.visibility === 'hidden' || textStyle.visibility === 'collapse') continue;
    for (const rect of range.getClientRects()) {
      include(rect.left + scrollX, rect.top + scrollY, rect.right + scrollX, rect.bottom + scrollY);
    }
  }
  return {width: maxRight, height: maxBottom, valid: valid && minLeft >= 0 && minTop >= 0};
})()`, &dimensions),
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
	if !dimensions.Valid {
		return Result{}, errors.New("content cannot fit one A5 page")
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
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) || len(pdfPagePattern.FindAll(pdf, -1)) == 0 {
		return Result{}, errors.New("browser did not produce PDF pages")
	}
	return Result{PDF: pdf, Scale: scale}, nil
}

func scaleToFit(width, height float64) (float64, error) {
	if math.IsNaN(width) || math.IsNaN(height) || math.IsInf(width, 0) || math.IsInf(height, 0) ||
		width < 0 || height < 0 || fixedScale > printableWidth/width {
		return 0, errors.New("content cannot fit one A5 page")
	}
	return fixedScale, nil
}
