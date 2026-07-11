package message

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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

func Extract(r io.Reader, maxBytes int64) (Document, error) {
	var doc Document

	raw, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return doc, fmt.Errorf("read message: %w", err)
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
	err = entity.Walk(func(_ []int, part *gomessage.Entity, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("%w: walk entity", ErrMalformedMIME)
		}

		mediaType, _, err := part.Header.ContentType()
		if err != nil {
			return fmt.Errorf("%w: content type", ErrMalformedMIME)
		}
		mediaType = strings.ToLower(mediaType)
		if mediaType == "text/html" {
			body, err := io.ReadAll(part.Body)
			if err != nil {
				return fmt.Errorf("%w: read HTML part", ErrMalformedMIME)
			}
			doc.HTML = body
			return nil
		}

		cid := normalizeCID(part.Header.Get("Content-ID"))
		if cid == "" || !strings.HasPrefix(mediaType, "image/") {
			return nil
		}
		data, err := io.ReadAll(io.LimitReader(part.Body, maxCIDBytes+1))
		if err != nil {
			return fmt.Errorf("read CID image: %w", err)
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
	if doc.HTML == nil {
		return Document{}, ErrMissingHTML
	}
	return doc, nil
}

func normalizeCID(v string) string {
	return strings.ToLower(strings.TrimSpace(strings.Trim(strings.TrimSpace(v), "<>")))
}
