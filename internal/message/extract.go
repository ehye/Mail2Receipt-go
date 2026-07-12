package message

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	gomessage "github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
)

const (
	maxCIDBytes    = 10 << 20
	maxAllCIDBytes = 50 << 20
)

var (
	ErrMessageTooLarge  = errors.New("message too large")
	ErrReadMessage      = errors.New("read message")
	ErrInvalidLimit     = errors.New("invalid message size limit")
	ErrMalformedMIME    = errors.New("malformed MIME")
	ErrMissingHTML      = errors.New("missing HTML")
	ErrCIDTooLarge      = errors.New("CID image too large")
	ErrCIDTotalTooLarge = errors.New("combined CID images too large")
)

type Asset struct {
	MediaType string
	Data      []byte
}

type Document struct {
	HTML []byte
	CID  map[string]Asset
}

type mimePart struct {
	mediaType  string
	attachment bool
	html       []byte
	children   []*mimePart
}

func Extract(r io.Reader, maxBytes int64) (Document, error) {
	var doc Document
	if maxBytes < 0 || maxBytes == math.MaxInt64 {
		return doc, ErrInvalidLimit
	}

	raw, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return doc, ErrReadMessage
	}
	if int64(len(raw)) > maxBytes {
		return doc, ErrMessageTooLarge
	}

	entity, err := gomessage.Read(bytes.NewReader(raw))
	if err != nil {
		return doc, fmt.Errorf("%w: read entity", ErrMalformedMIME)
	}

	doc.CID = make(map[string]Asset)
	var cidBytes int64
	parts := make(map[string]*mimePart)
	var root *mimePart
	err = entity.Walk(func(path []int, part *gomessage.Entity, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("%w: walk entity", ErrMalformedMIME)
		}

		mediaType, _, err := part.Header.ContentType()
		if err != nil {
			return fmt.Errorf("%w: content type", ErrMalformedMIME)
		}
		mediaType = strings.ToLower(mediaType)
		var disposition string
		if part.Header.Get("Content-Disposition") != "" {
			disposition, _, err = part.Header.ContentDisposition()
			if err != nil {
				return fmt.Errorf("%w: content disposition", ErrMalformedMIME)
			}
		}
		node := &mimePart{
			mediaType:  mediaType,
			attachment: strings.EqualFold(disposition, "attachment"),
		}
		key := fmt.Sprint(path)
		parts[key] = node
		if len(path) == 0 {
			root = node
		} else {
			parent := parts[fmt.Sprint(path[:len(path)-1])]
			parent.children = append(parent.children, node)
		}

		if mediaType == "text/html" {
			body, err := io.ReadAll(part.Body)
			if err != nil {
				return fmt.Errorf("%w: read HTML part", ErrMalformedMIME)
			}
			node.html = body
			return nil
		}

		cid := normalizeCID(part.Header.Get("Content-ID"))
		if cid == "" || !strings.HasPrefix(mediaType, "image/") {
			return nil
		}
		data, err := io.ReadAll(io.LimitReader(part.Body, maxCIDBytes+1))
		if err != nil {
			return fmt.Errorf("%w: read CID image", ErrMalformedMIME)
		}
		if len(data) > maxCIDBytes {
			return ErrCIDTooLarge
		}
		cidBytes += int64(len(data))
		if cidBytes > maxAllCIDBytes {
			return ErrCIDTotalTooLarge
		}
		doc.CID[cid] = Asset{MediaType: mediaType, Data: data}
		return nil
	})
	if err != nil {
		return Document{}, err
	}
	doc.HTML = selectHTML(root)
	if doc.HTML == nil {
		return Document{}, ErrMissingHTML
	}
	return doc, nil
}

func selectHTML(part *mimePart) []byte {
	if part == nil || part.attachment {
		return nil
	}
	if part.mediaType == "text/html" {
		return part.html
	}
	if part.mediaType == "multipart/alternative" {
		var selected []byte
		for _, child := range part.children {
			if html := selectHTML(child); html != nil {
				selected = html
			}
		}
		return selected
	}
	for _, child := range part.children {
		if html := selectHTML(child); html != nil {
			return html
		}
	}
	return nil
}

func normalizeCID(v string) string {
	return strings.ToLower(strings.TrimSpace(strings.Trim(strings.TrimSpace(v), "<>")))
}
