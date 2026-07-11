package document

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
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
			attrs := node.Attr[:0]
			for _, attr := range node.Attr {
				keep := true
				switch strings.ToLower(attr.Key) {
				case "src", "background":
					attr.Val, keep, err = normalizeImageReference(attr.Val, doc.CID)
				case "srcset":
					attr.Val, err = normalizeSrcset(attr.Val, doc.CID)
					keep = strings.TrimSpace(attr.Val) != ""
				case "style":
					attr.Val, err = normalizeDeclarations(attr.Val, doc.CID)
					keep = strings.TrimSpace(attr.Val) != ""
				}
				if err != nil {
					return err
				}
				if keep {
					attrs = append(attrs, attr)
				}
			}
			node.Attr = attrs
			if node.Data == "style" {
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type == html.TextNode {
						child.Data, err = normalizeStylesheet(child.Data, doc.CID)
						if err != nil {
							return err
						}
					}
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
	asset, ok := lookupAsset(assets, id)
	if !ok {
		return "", ErrMissingCID
	}
	if !strings.HasPrefix(strings.ToLower(asset.MediaType), "image/") {
		return "", ErrNonImageCID
	}
	return "data:" + asset.MediaType + ";base64," + base64.StdEncoding.EncodeToString(asset.Data), nil
}

func lookupAsset(assets map[string]message.Asset, id string) (message.Asset, bool) {
	for key, asset := range assets {
		if strings.EqualFold(key, id) {
			return asset, true
		}
	}
	return message.Asset{}, false
}

func normalizeImageReference(value string, assets map[string]message.Asset) (string, bool, error) {
	replaced, err := replaceCID(value, assets)
	if err != nil {
		return "", false, err
	}
	if replaced != value {
		return replaced, true, nil
	}
	if logo, ok := embeddedLogo(value); ok {
		return logo, true, nil
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err == nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) {
		return "", false, nil
	}
	return value, true, nil
}

func normalizeSrcset(value string, assets map[string]message.Asset) (string, error) {
	var candidates []string
	for pos := 0; pos < len(value); {
		for pos < len(value) && (isASCIISpace(value[pos]) || value[pos] == ',') {
			pos++
		}
		urlStart := pos
		for pos < len(value) && !isASCIISpace(value[pos]) {
			pos++
		}
		urlEnd := pos
		for urlEnd > urlStart && value[urlEnd-1] == ',' {
			urlEnd--
		}
		if urlStart == urlEnd {
			continue
		}

		replaced, keep, err := normalizeImageReference(value[urlStart:urlEnd], assets)
		if err != nil {
			return "", err
		}

		descriptorStart := pos
		parentheses := 0
		for pos < len(value) {
			switch value[pos] {
			case '(':
				parentheses++
			case ')':
				if parentheses > 0 {
					parentheses--
				}
			case ',':
				if parentheses == 0 {
					if keep {
						candidates = append(candidates, replaced+value[descriptorStart:pos])
					}
					pos++
					goto nextCandidate
				}
			}
			pos++
		}
		if keep {
			candidates = append(candidates, replaced+value[descriptorStart:pos])
		}
	nextCandidate:
	}
	return strings.Join(candidates, ", "), nil
}

func isASCIISpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\f' || value == '\r'
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
		normalized, keep, err := normalizeImageReference(inner, assets)
		if err != nil {
			replaceErr = err
			return match
		}
		if !keep || normalized == inner {
			return match
		}
		if quote != 0 {
			return "url(" + string(quote) + normalized + string(quote) + ")"
		}
		return "url(" + normalized + ")"
	})
	return replaced, replaceErr
}

func containsRemoteCSSURL(value string) bool {
	for _, match := range cssURLPattern.FindAllString(value, -1) {
		open := strings.IndexByte(match, '(')
		inner := strings.Trim(strings.TrimSpace(match[open+1:len(match)-1]), "'\"")
		parsed, err := url.Parse(strings.TrimSpace(inner))
		if err == nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) {
			return true
		}
	}
	return false
}

func keepDeclaration(property, value string) bool {
	property = strings.TrimSpace(strings.ToLower(property))
	lowerValue := strings.ToLower(value)
	if property != "color" && strings.Contains(lowerValue, "#ededed") {
		return false
	}
	return !containsRemoteCSSURL(value)
}

func normalizeDeclarations(value string, assets map[string]message.Asset) (string, error) {
	replaced, err := replaceStyleURLs(value, assets)
	if err != nil {
		return "", err
	}
	parts := splitCSS(replaced, ';')
	kept := parts[:0]
	for _, declaration := range parts {
		colon := indexCSS(declaration, ':')
		if colon < 0 {
			if !containsRemoteCSSURL(declaration) {
				kept = append(kept, declaration)
			}
			continue
		}
		if keepDeclaration(declaration[:colon], declaration[colon+1:]) {
			kept = append(kept, declaration)
		}
	}
	return strings.Join(kept, ";"), nil
}

func splitCSS(value string, separator byte) []string {
	var parts []string
	start := 0
	quote := byte(0)
	depth := 0
	for i := 0; i < len(value); i++ {
		char := value[i]
		if quote != 0 {
			if char == quote && (i == 0 || value[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if char == separator && depth == 0 {
				parts = append(parts, value[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, value[start:])
}

func indexCSS(value string, separator byte) int {
	parts := splitCSS(value, separator)
	if len(parts) < 2 {
		return -1
	}
	return len(parts[0])
}

func normalizeStylesheet(value string, assets map[string]message.Asset) (string, error) {
	var output strings.Builder
	for pos := 0; pos < len(value); {
		open := strings.IndexByte(value[pos:], '{')
		if open < 0 {
			output.WriteString(value[pos:])
			break
		}
		open += pos
		output.WriteString(value[pos : open+1])
		depth := 1
		quote := byte(0)
		close := open + 1
		for ; close < len(value) && depth > 0; close++ {
			char := value[close]
			if quote != 0 {
				if char == quote && value[close-1] != '\\' {
					quote = 0
				}
				continue
			}
			switch char {
			case '\'', '"':
				quote = char
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		if depth != 0 {
			output.WriteString(value[open+1:])
			break
		}
		body := value[open+1 : close-1]
		var normalized string
		var err error
		if strings.Contains(body, "{") {
			normalized, err = normalizeStylesheet(body, assets)
		} else {
			normalized, err = normalizeDeclarations(body, assets)
		}
		if err != nil {
			return "", err
		}
		output.WriteString(normalized)
		output.WriteByte('}')
		pos = close
	}
	return output.String(), nil
}
