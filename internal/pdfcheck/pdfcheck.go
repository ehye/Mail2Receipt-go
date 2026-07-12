package pdfcheck

import (
	"bytes"
	"errors"
	"fmt"
	"math"

	"rsc.io/pdf"
)

const (
	a5Width   = 419.528
	a5Height  = 595.276
	tolerance = 1.0
)

// Verify requires a readable, single-page A5 portrait PDF.
func Verify(data []byte) error {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return errors.New("invalid PDF")
	}
	if pages := reader.NumPage(); pages != 1 {
		return fmt.Errorf("invalid page count: got %d, want 1", pages)
	}

	mediaBox := reader.Page(1).V.Key("MediaBox")
	if mediaBox.Len() != 4 {
		return fmt.Errorf("invalid paper dimensions: missing media box")
	}
	width := mediaBox.Index(2).Float64() - mediaBox.Index(0).Float64()
	height := mediaBox.Index(3).Float64() - mediaBox.Index(1).Float64()
	if math.Abs(width-a5Width) > tolerance || math.Abs(height-a5Height) > tolerance {
		return fmt.Errorf("invalid paper dimensions: got %.3f x %.3f points, want A5 portrait", width, height)
	}
	return nil
}
