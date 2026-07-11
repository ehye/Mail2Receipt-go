package document

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/html"

	"mail2receipt/internal/message"
)

const printCSS = `@page { size: A5 portrait; margin: 8mm; }
html, body { margin: 0; padding: 0; }`

var (
	ErrMissingCID  = errors.New("referenced CID is missing")
	ErrNonImageCID = errors.New("referenced CID is not an image")
	cssURLPattern  = regexp.MustCompile(`(?i)url\(\s*(?:"[^"]*"|'[^']*'|[^)]*)\s*\)`)
)

func Prepare(doc message.Document) ([]byte, error) {
	root, err := html.Parse(bytes.NewReader(doc.HTML))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	var head *html.Node
	var walk func(*html.Node) error
	walk = func(node *html.Node) error {
		if node.Type == html.ElementNode {
			if node.Data == "head" {
				head = node
			}
			for i := range node.Attr {
				attr := &node.Attr[i]
				switch strings.ToLower(attr.Key) {
				case "src", "background":
					attr.Val, err = replaceCID(attr.Val, doc.CID)
				case "srcset":
					attr.Val, err = replaceSrcset(attr.Val, doc.CID)
				case "style":
					attr.Val, err = replaceStyleURLs(attr.Val, doc.CID)
				}
				if err != nil {
					return err
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}

	style := &html.Node{Type: html.ElementNode, Data: "style"}
	style.AppendChild(&html.Node{Type: html.TextNode, Data: printCSS})
	head.AppendChild(style)

	var output bytes.Buffer
	if err := html.Render(&output, root); err != nil {
		return nil, fmt.Errorf("serialize HTML: %w", err)
	}
	return output.Bytes(), nil
}

func replaceCID(value string, assets map[string]message.Asset) (string, error) {
	if len(value) < 4 || !strings.EqualFold(value[:4], "cid:") {
		return value, nil
	}
	id := value[4:]
	if id == "" || strings.IndexFunc(id, unicode.IsSpace) >= 0 {
		return value, nil
	}
	asset, ok := assets[strings.ToLower(id)]
	if !ok {
		return "", ErrMissingCID
	}
	if !strings.HasPrefix(strings.ToLower(asset.MediaType), "image/") {
		return "", ErrNonImageCID
	}
	return "data:" + asset.MediaType + ";base64," + base64.StdEncoding.EncodeToString(asset.Data), nil
}

func replaceSrcset(value string, assets map[string]message.Asset) (string, error) {
	parts := strings.Split(value, ",")
	for i, part := range parts {
		leading := len(part) - len(strings.TrimLeftFunc(part, unicode.IsSpace))
		candidate := part[leading:]
		urlEnd := strings.IndexFunc(candidate, unicode.IsSpace)
		if urlEnd < 0 {
			urlEnd = len(candidate)
		}
		replaced, err := replaceCID(candidate[:urlEnd], assets)
		if err != nil {
			return "", err
		}
		parts[i] = part[:leading] + replaced + candidate[urlEnd:]
	}
	return strings.Join(parts, ","), nil
}

func replaceStyleURLs(value string, assets map[string]message.Asset) (string, error) {
	var replaceErr error
	replaced := cssURLPattern.ReplaceAllStringFunc(value, func(match string) string {
		if replaceErr != nil {
			return match
		}
		open := strings.IndexByte(match, '(')
		inner := strings.TrimSpace(match[open+1 : len(match)-1])
		quote := byte(0)
		if len(inner) >= 2 && (inner[0] == '\'' || inner[0] == '"') && inner[len(inner)-1] == inner[0] {
			quote = inner[0]
			inner = inner[1 : len(inner)-1]
		}
		url, err := replaceCID(inner, assets)
		if err != nil {
			replaceErr = err
			return match
		}
		if url == inner {
			return match
		}
		if quote != 0 {
			return "url(" + string(quote) + url + string(quote) + ")"
		}
		return "url(" + url + ")"
	})
	return replaced, replaceErr
}
