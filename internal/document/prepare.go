package document

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"

	"mail2receipt/internal/message"
)

const printCSS = `@page { size: A5 portrait; margin: 2mm; }
html, body { margin: 0; padding: 0; }
[data-mail2receipt-emphasis], [data-mail2receipt-emphasis] * { font-size: 12px !important; line-height: 18px !important; }`

const maxSrcdocDepth = 4

var (
	ErrMissingCID  = errors.New("referenced CID is missing")
	ErrNonImageCID = errors.New("referenced CID is not an image")
	simpleFontSize = regexp.MustCompile(`(?i)^(\s*)([+]?(?:\d+(?:\.\d*)?|\.\d+))(px|pt|pc|in|cm|mm|q|em|rem|ex|ch|cap|ic|lh|rlh|vw|vh|vi|vb|vmin|vmax|svw|svh|svi|svb|svmin|svmax|lvw|lvh|lvi|lvb|lvmin|lvmax|dvw|dvh|dvi|dvb|dvmin|dvmax|%)(\s*(?:!\s*important\s*)?)$`)
)

func Prepare(doc message.Document) ([]byte, error) {
	return prepareHTML(doc.HTML, doc.CID, 0, true)
}

func prepareHTML(source []byte, assets map[string]message.Asset, srcdocDepth int, addPrintCSS bool) ([]byte, error) {
	root, err := html.Parse(bytes.NewReader(source))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

	var head *html.Node
	var walk func(*html.Node) error
	walk = func(node *html.Node) error {
		if node.Type == html.ElementNode {
			if node.Data == "base" {
				node.Parent.RemoveChild(node)
				return nil
			}
			if node.Data == "head" {
				head = node
			}
			attrs := node.Attr[:0]
			refreshTarget := node.Data == "meta" && hasMetaRefreshTarget(node.Attr)
			for _, attr := range node.Attr {
				name := strings.ToLower(attr.Key)
				if strings.HasPrefix(name, "shadowroot") {
					continue
				}
				if name == "data-mail2receipt-emphasis" {
					continue
				}
				keep := true
				switch name {
				case "data":
					if node.Data == "object" {
						attr.Val, keep, err = normalizeImageReference(attr.Val, assets)
					}
				case "href":
					if node.Data != "a" && node.Data != "area" {
						attr.Val, keep, err = normalizeImageReference(attr.Val, assets)
					}
				case "poster":
					attr.Val, keep, err = normalizeImageReference(attr.Val, assets)
				case "src", "background":
					attr.Val, keep, err = normalizeImageReference(attr.Val, assets)
				case "srcdoc":
					if node.Data == "iframe" {
						if srcdocDepth >= maxSrcdocDepth {
							keep = false
						} else {
							var nested []byte
							nested, err = prepareHTML([]byte(attr.Val), assets, srcdocDepth+1, false)
							attr.Val = string(nested)
						}
					}
				case "srcset":
					attr.Val, err = normalizeSrcset(attr.Val, assets)
					keep = strings.TrimSpace(attr.Val) != ""
				case "style":
					attr.Val, err = normalizeDeclarations(attr.Val, assets, addPrintCSS)
					keep = strings.TrimSpace(attr.Val) != ""
				case "content":
					if refreshTarget {
						keep = false
					}
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
						child.Data, err = normalizeStylesheet(child.Data, assets, addPrintCSS)
						if err != nil {
							return err
						}
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; {
			next := child.NextSibling
			if err := walk(child); err != nil {
				return err
			}
			child = next
		}
		if addPrintCSS && node.Type == html.ElementNode && emphasisPrefix(descendantText(node)) && !hasMarkedDescendant(node) {
			node.Attr = append(node.Attr, html.Attribute{Key: "data-mail2receipt-emphasis"})
			forceEmphasisTypography(node)
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}

	if addPrintCSS {
		style := &html.Node{Type: html.ElementNode, Data: "style"}
		style.AppendChild(&html.Node{Type: html.TextNode, Data: printCSS})
		head.AppendChild(style)
	}

	var output bytes.Buffer
	if err := html.Render(&output, root); err != nil {
		return nil, fmt.Errorf("serialize HTML: %w", err)
	}
	return output.Bytes(), nil
}

func hasMetaRefreshTarget(attrs []html.Attribute) bool {
	var refresh bool
	var content string
	for _, attr := range attrs {
		switch strings.ToLower(attr.Key) {
		case "http-equiv":
			refresh = strings.EqualFold(strings.TrimSpace(attr.Val), "refresh")
		case "content":
			content = attr.Val
		}
	}
	if !refresh {
		return false
	}
	semicolon := strings.IndexByte(content, ';')
	if semicolon < 0 {
		return false
	}
	target := strings.TrimSpace(content[semicolon+1:])
	if len(target) >= 3 && strings.EqualFold(target[:3], "url") {
		afterURL := strings.TrimSpace(target[3:])
		if len(afterURL) > 0 && afterURL[0] == '=' {
			target = strings.TrimSpace(afterURL[1:])
		}
	}
	if len(target) >= 2 && (target[0] == '\'' || target[0] == '"') && target[len(target)-1] == target[0] {
		target = target[1 : len(target)-1]
	}
	return strings.TrimSpace(target) != ""
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
	mediaType := strings.ToLower(strings.TrimSpace(asset.MediaType))
	wantFormat := map[string]string{
		"image/png":  "png",
		"image/jpeg": "jpeg",
		"image/gif":  "gif",
	}[mediaType]
	if wantFormat == "" {
		return "", ErrNonImageCID
	}
	_, format, err := image.Decode(bytes.NewReader(asset.Data))
	if err != nil || format != wantFormat {
		return "", ErrNonImageCID
	}
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(asset.Data), nil
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
	return "", false, nil
}

func isImageDataURL(value string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "data:image/")
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
	resources := scanCSSResources(value)
	for pos := len(resources) - 1; pos >= 0; pos-- {
		resource := resources[pos]
		if resource.kind != cssURLResource {
			continue
		}
		normalized, keep, err := normalizeImageReference(resource.value, assets)
		if err != nil {
			return "", err
		}
		if keep && normalized != resource.value {
			value = value[:resource.start] + `url("` + normalized + `")` + value[resource.end:]
		}
	}
	return value, nil
}

func containsNonEmbeddedCSSURL(value string) bool {
	for _, resource := range scanCSSResources(value) {
		if !isImageDataURL(resource.value) {
			return true
		}
	}
	return false
}

func isCSSIdentifierByte(value byte) bool {
	return value == '-' || value == '_' || value >= '0' && value <= '9' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= 0x80
}

func matchingCSSParen(value string, open int) int {
	depth := 0
	quote := byte(0)
	inComment := false
	for pos := open; pos < len(value); pos++ {
		char := value[pos]
		if inComment {
			if char == '*' && pos+1 < len(value) && value[pos+1] == '/' {
				inComment = false
				pos++
			}
			continue
		}
		if quote != 0 {
			if char == quote && !isEscaped(value, pos) {
				quote = 0
			}
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
		case '/':
			if pos+1 < len(value) && value[pos+1] == '*' {
				inComment = true
				pos++
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return pos
			}
		}
	}
	return -1
}

func keepDeclaration(property, value string) bool {
	property = strings.TrimSpace(strings.ToLower(property))
	lowerValue := strings.ToLower(value)
	if property != "color" && strings.Contains(lowerValue, "#ededed") {
		return false
	}
	return !containsNonEmbeddedCSSURL(value)
}

func normalizeDeclarations(value string, assets map[string]message.Asset, scaleFonts bool) (string, error) {
	parts := splitCSS(value, ';')
	kept := parts[:0]
	for _, declaration := range parts {
		if err := validateCSSCIDReferences(declaration, assets); err != nil {
			return "", err
		}
		if containsSourceDataImage(declaration) {
			continue
		}
		replaced, err := replaceStyleURLs(declaration, assets)
		if err != nil {
			return "", err
		}
		declaration = replaced
		colon := indexCSS(declaration, ':')
		if colon < 0 {
			if !containsNonEmbeddedCSSURL(declaration) {
				kept = append(kept, declaration)
			}
			continue
		}
		if keepDeclaration(declaration[:colon], declaration[colon+1:]) {
			if scaleFonts && strings.EqualFold(strings.TrimSpace(declaration[:colon]), "font-size") {
				declaration = declaration[:colon+1] + increaseFontSize(declaration[colon+1:])
			}
			kept = append(kept, declaration)
		}
	}
	return strings.Join(kept, ";"), nil
}

func increaseFontSize(value string) string {
	parts := simpleFontSize.FindStringSubmatch(value)
	if parts == nil {
		return value
	}
	number, err := strconv.ParseFloat(parts[2], 64)
	if err != nil || number == 0 {
		return value
	}
	return parts[1] + strconv.FormatFloat(number*1.05, 'f', -1, 64) + parts[3] + parts[4]
}

func descendantText(node *html.Node) string {
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			text.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(text.String()), " ")
}

func emphasisPrefix(text string) bool {
	lower := strings.ToLower(text)
	return hasTextPrefix(lower, "by subscribing, you authorize us to") || hasTextPrefix(lower, "see your")
}

func hasTextPrefix(text, prefix string) bool {
	if !strings.HasPrefix(text, prefix) {
		return false
	}
	if len(text) == len(prefix) {
		return true
	}
	next, _ := utf8.DecodeRuneInString(text[len(prefix):])
	return !unicode.IsLetter(next) && !unicode.IsNumber(next)
}

func forceEmphasisTypography(node *html.Node) {
	if node.Type == html.ElementNode {
		styleIndex := -1
		for i := range node.Attr {
			if strings.EqualFold(node.Attr[i].Key, "style") {
				styleIndex = i
				break
			}
		}
		var kept []string
		if styleIndex >= 0 {
			for _, declaration := range splitCSS(node.Attr[styleIndex].Val, ';') {
				colon := indexCSS(declaration, ':')
				if colon >= 0 {
					property := strings.TrimSpace(declaration[:colon])
					if strings.EqualFold(property, "font-size") || strings.EqualFold(property, "line-height") {
						continue
					}
				}
				if strings.TrimSpace(declaration) != "" {
					kept = append(kept, declaration)
				}
			}
			node.Attr[styleIndex].Val = strings.Join(append(kept, "font-size:12px !important", "line-height:18px !important"), ";")
		} else {
			node.Attr = append(node.Attr, html.Attribute{Key: "style", Val: "font-size:12px !important;line-height:18px !important"})
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		forceEmphasisTypography(child)
	}
}

func hasMarkedDescendant(node *html.Node) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			for _, attr := range child.Attr {
				if attr.Key == "data-mail2receipt-emphasis" {
					return true
				}
			}
		}
		if hasMarkedDescendant(child) {
			return true
		}
	}
	return false
}

func validateCSSCIDReferences(value string, assets map[string]message.Asset) error {
	for _, resource := range scanCSSResources(value) {
		if _, err := replaceCID(resource.value, assets); err != nil {
			return err
		}
	}
	return nil
}

func containsSourceDataImage(value string) bool {
	for _, resource := range scanCSSResources(value) {
		if isImageDataURL(resource.value) {
			return true
		}
	}
	return false
}

type cssResourceKind uint8

const (
	cssURLResource cssResourceKind = iota
	cssImageSetStringResource
)

type cssResource struct {
	start int
	end   int
	kind  cssResourceKind
	value string
}

func scanCSSResources(value string) []cssResource {
	return scanCSSResourceRange(value, 0, len(value))
}

func scanCSSResourceRange(value string, start, end int) []cssResource {
	var resources []cssResource
	for pos := start; pos < end; {
		if value[pos] == '/' && pos+1 < end && value[pos+1] == '*' {
			pos = skipCSSComment(value, pos, end)
			continue
		}
		if value[pos] == '\'' || value[pos] == '"' {
			pos = skipCSSString(value, pos, end)
			continue
		}
		if !isCSSIdentifierStart(value[pos]) {
			pos++
			continue
		}
		identifierStart := pos
		pos = scanCSSIdentifier(value, pos, end)
		identifier := strings.ToLower(decodeCSSEscapes(value[identifierStart:pos]))
		open := skipCSSSpaceAndComments(value, pos, end)
		if open >= end || value[open] != '(' {
			continue
		}
		close := matchingCSSParen(value, open)
		if close < 0 || close >= end {
			if identifier == "url" || identifier == "image-set" || identifier == "-webkit-image-set" {
				resources = append(resources, cssResource{start: identifierStart, end: end, kind: cssURLResource})
			}
			continue
		}
		switch identifier {
		case "url":
			resources = append(resources, cssResource{
				start: identifierStart,
				end:   close + 1,
				kind:  cssURLResource,
				value: cssResourceValue(value[open+1 : close]),
			})
		case "image-set", "-webkit-image-set":
			bodyStart := open + 1
			for _, bounds := range splitCSSBounds(value, bodyStart, close, ',') {
				candidateStart := skipCSSSpaceAndComments(value, bounds[0], bounds[1])
				if candidateStart < bounds[1] && (value[candidateStart] == '\'' || value[candidateStart] == '"') {
					candidateEnd := skipCSSString(value, candidateStart, bounds[1])
					if candidateEnd <= bounds[1] && candidateEnd > candidateStart+1 {
						resources = append(resources, cssResource{
							start: candidateStart,
							end:   candidateEnd,
							kind:  cssImageSetStringResource,
							value: decodeCSSEscapes(value[candidateStart+1 : candidateEnd-1]),
						})
					}
				}
				resources = append(resources, scanCSSResourceRange(value, bounds[0], bounds[1])...)
			}
		default:
			resources = append(resources, scanCSSResourceRange(value, open+1, close)...)
		}
		pos = close + 1
	}
	return resources
}

func isCSSIdentifierStart(value byte) bool {
	return value == '-' || value == '_' || value == '\\' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= 0x80
}

func scanCSSIdentifier(value string, pos, end int) int {
	for pos < end {
		if isCSSIdentifierByte(value[pos]) {
			pos++
			continue
		}
		if value[pos] != '\\' || pos+1 >= end {
			break
		}
		pos++
		start := pos
		for pos < end && pos-start < 6 && isHex(value[pos]) {
			pos++
		}
		if start == pos {
			pos++
		} else if pos < end && isASCIISpace(value[pos]) {
			pos++
		}
	}
	return pos
}

func skipCSSSpaceAndComments(value string, pos, end int) int {
	for pos < end {
		if isASCIISpace(value[pos]) {
			pos++
			continue
		}
		if value[pos] == '/' && pos+1 < end && value[pos+1] == '*' {
			pos = skipCSSComment(value, pos, end)
			continue
		}
		break
	}
	return pos
}

func skipCSSComment(value string, pos, end int) int {
	pos += 2
	for pos+1 < end {
		if value[pos] == '*' && value[pos+1] == '/' {
			return pos + 2
		}
		pos++
	}
	return end
}

func skipCSSString(value string, pos, end int) int {
	quote := value[pos]
	for pos++; pos < end; pos++ {
		if value[pos] == quote && !isEscaped(value, pos) {
			return pos + 1
		}
	}
	return end
}

func cssResourceValue(value string) string {
	value = stripCSSComments(value)
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') && value[len(value)-1] == value[0] {
		value = value[1 : len(value)-1]
	}
	return decodeCSSEscapes(value)
}

func stripCSSComments(value string) string {
	var output strings.Builder
	for pos := 0; pos < len(value); {
		if value[pos] == '/' && pos+1 < len(value) && value[pos+1] == '*' {
			pos = skipCSSComment(value, pos, len(value))
			continue
		}
		output.WriteByte(value[pos])
		pos++
	}
	return output.String()
}

func splitCSSBounds(value string, start, end int, separator byte) [][2]int {
	parts := splitCSS(value[start:end], separator)
	bounds := make([][2]int, 0, len(parts))
	pos := start
	for _, part := range parts {
		bounds = append(bounds, [2]int{pos, pos + len(part)})
		pos += len(part) + 1
	}
	return bounds
}

func splitCSS(value string, separator byte) []string {
	var parts []string
	start := 0
	quote := byte(0)
	depth := 0
	inComment := false
	for i := 0; i < len(value); i++ {
		char := value[i]
		if inComment {
			if char == '*' && i+1 < len(value) && value[i+1] == '/' {
				inComment = false
				i++
			}
			continue
		}
		if quote != 0 {
			if char == quote && !isEscaped(value, i) {
				quote = 0
			}
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
		case '/':
			if i+1 < len(value) && value[i+1] == '*' {
				inComment = true
				i++
			}
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

func isEscaped(value string, pos int) bool {
	backslashes := 0
	for pos--; pos >= 0 && value[pos] == '\\'; pos-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func indexCSS(value string, separator byte) int {
	parts := splitCSS(value, separator)
	if len(parts) < 2 {
		return -1
	}
	return len(parts[0])
}

func normalizeStylesheet(value string, assets map[string]message.Asset, scaleFonts bool) (string, error) {
	return normalizeStylesheetLevel(value, assets, scaleFonts, false)
}

func normalizeStylesheetLevel(value string, assets map[string]message.Asset, scaleFonts, declarationsAroundRules bool) (string, error) {
	var output strings.Builder
	for pos := 0; pos < len(value); {
		open := findCSSBrace(value, pos)
		if open < 0 {
			tail := removeRemoteImports(value[pos:])
			if declarationsAroundRules {
				var err error
				tail, err = normalizeDeclarations(tail, assets, scaleFonts)
				if err != nil {
					return "", err
				}
			}
			output.WriteString(tail)
			break
		}
		prelude := removeRemoteImports(value[pos:open])
		if declarationsAroundRules {
			parts := splitCSS(prelude, ';')
			if len(parts) > 1 {
				declarations, err := normalizeDeclarations(strings.Join(parts[:len(parts)-1], ";"), assets, scaleFonts)
				if err != nil {
					return "", err
				}
				output.WriteString(declarations)
				output.WriteByte(';')
				prelude = parts[len(parts)-1]
			}
		}
		output.WriteString(prelude)
		output.WriteByte('{')
		depth := 1
		quote := byte(0)
		inComment := false
		close := open + 1
		for ; close < len(value) && depth > 0; close++ {
			char := value[close]
			if inComment {
				if char == '*' && close+1 < len(value) && value[close+1] == '/' {
					inComment = false
					close++
				}
				continue
			}
			if quote != 0 {
				if char == quote && !isEscaped(value, close) {
					quote = 0
				}
				continue
			}
			switch char {
			case '\'', '"':
				quote = char
			case '/':
				if close+1 < len(value) && value[close+1] == '*' {
					inComment = true
					close++
				}
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		if depth != 0 {
			body := value[open+1:]
			var normalized string
			var err error
			if findCSSBrace(body, 0) >= 0 {
				normalized, err = normalizeStylesheetLevel(body, assets, scaleFonts, true)
			} else {
				normalized, err = normalizeDeclarations(body, assets, scaleFonts)
			}
			if err != nil {
				return "", err
			}
			output.WriteString(normalized)
			break
		}
		body := value[open+1 : close-1]
		var normalized string
		var err error
		if findCSSBrace(body, 0) >= 0 {
			normalized, err = normalizeStylesheetLevel(body, assets, scaleFonts, true)
		} else {
			normalized, err = normalizeDeclarations(body, assets, scaleFonts)
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

func findCSSBrace(value string, start int) int {
	quote := byte(0)
	inComment := false
	for i := start; i < len(value); i++ {
		char := value[i]
		if inComment {
			if char == '*' && i+1 < len(value) && value[i+1] == '/' {
				inComment = false
				i++
			}
			continue
		}
		if quote != 0 {
			if char == quote && !isEscaped(value, i) {
				quote = 0
			}
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
		case '/':
			if i+1 < len(value) && value[i+1] == '*' {
				inComment = true
				i++
			}
		case '{':
			return i
		}
	}
	return -1
}

func removeRemoteImports(value string) string {
	parts := splitCSS(value, ';')
	var output strings.Builder
	for i, part := range parts {
		if !isRemoteImport(part) {
			output.WriteString(part)
			if i < len(parts)-1 {
				output.WriteByte(';')
			}
		}
	}
	return output.String()
}

func isRemoteImport(value string) bool {
	trimmed := trimCSSSpaceAndComments(value)
	decoded := decodeCSSEscapes(trimmed)
	if len(decoded) < len("@import") || !strings.EqualFold(decoded[:len("@import")], "@import") {
		return false
	}
	return true
}

func decodeCSSEscapes(value string) string {
	var output strings.Builder
	for pos := 0; pos < len(value); {
		if value[pos] != '\\' || pos+1 >= len(value) {
			output.WriteByte(value[pos])
			pos++
			continue
		}
		pos++
		if value[pos] == '\n' || value[pos] == '\f' {
			pos++
			continue
		}
		if value[pos] == '\r' {
			pos++
			if pos < len(value) && value[pos] == '\n' {
				pos++
			}
			continue
		}
		start := pos
		for pos < len(value) && pos-start < 6 && isHex(value[pos]) {
			pos++
		}
		if start != pos {
			code, _ := strconv.ParseInt(value[start:pos], 16, 32)
			if code == 0 || code > unicode.MaxRune || code >= 0xD800 && code <= 0xDFFF {
				code = unicode.ReplacementChar
			}
			output.WriteRune(rune(code))
			if pos < len(value) && isASCIISpace(value[pos]) {
				pos++
			}
			continue
		}
		output.WriteByte(value[pos])
		pos++
	}
	return output.String()
}

func isHex(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func trimCSSSpaceAndComments(value string) string {
	trimmed := strings.TrimSpace(value)
	for strings.HasPrefix(trimmed, "/*") {
		end := strings.Index(trimmed[2:], "*/")
		if end < 0 {
			return trimmed
		}
		trimmed = strings.TrimSpace(trimmed[end+4:])
	}
	return trimmed
}
