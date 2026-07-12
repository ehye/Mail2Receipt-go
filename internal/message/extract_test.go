package message

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"
)

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestExtractPrefersDecodedHTML(t *testing.T) {
	raw := "MIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=x\r\n\r\n" +
		"--x\r\nContent-Type: text/plain\r\n\r\nplain\r\n" +
		"--x\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\nPGI+cmVjZWlwdDwvYj4=\r\n--x--\r\n"

	got, err := Extract(strings.NewReader(raw), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.HTML) != "<b>receipt</b>" {
		t.Fatalf("HTML = %q", got.HTML)
	}
}

func TestExtractDecodesQuotedPrintableNestedHTML(t *testing.T) {
	raw := "MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=outer\r\n\r\n" +
		"--outer\r\nContent-Type: multipart/alternative; boundary=inner\r\n\r\n" +
		"--inner\r\nContent-Type: text/plain\r\n\r\nplain\r\n" +
		"--inner\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n<p>total =3D $5</p>\r\n" +
		"--inner--\r\n--outer--\r\n"

	got, err := Extract(strings.NewReader(raw), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.HTML) != "<p>total = $5</p>" {
		t.Fatalf("HTML = %q", got.HTML)
	}
}

func TestExtractConvertsDeclaredCharsetToUTF8(t *testing.T) {
	raw := append([]byte("MIME-Version: 1.0\r\nContent-Type: text/html; charset=iso-8859-1\r\n\r\n<p>caf"), 0xe9)
	raw = append(raw, []byte("</p>")...)

	got, err := Extract(bytes.NewReader(raw), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.HTML) != "<p>caf\u00e9</p>" {
		t.Fatalf("HTML = %q", got.HTML)
	}
}

func TestExtractIndexesImagesByNormalizedContentID(t *testing.T) {
	raw := "MIME-Version: 1.0\r\nContent-Type: multipart/related; boundary=x\r\n\r\n" +
		"--x\r\nContent-Type: text/html\r\n\r\n<img src=\"cid:Logo@Example\">\r\n" +
		"--x\r\nContent-Type: image/png\r\nContent-ID: < Logo@Example >\r\nContent-Transfer-Encoding: base64\r\n\r\nAQID\r\n--x--\r\n"

	got, err := Extract(strings.NewReader(raw), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	asset, ok := got.CID["logo@example"]
	if !ok {
		t.Fatalf("CID keys = %v", got.CID)
	}
	if asset.MediaType != "image/png" || !bytes.Equal(asset.Data, []byte{1, 2, 3}) {
		t.Fatalf("asset = %#v", asset)
	}
}

func TestExtractMixedSelectsFirstHTMLChild(t *testing.T) {
	raw := "MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=x\r\n\r\n" +
		"--x\r\nContent-Type: text/html\r\n\r\nfirst\r\n" +
		"--x\r\nContent-Type: text/html\r\n\r\nfinal\r\n--x--\r\n"

	got, err := Extract(strings.NewReader(raw), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.HTML) != "first" {
		t.Fatalf("HTML = %q", got.HTML)
	}
}

func TestExtractIgnoresHTMLAttachmentAfterBody(t *testing.T) {
	raw := "MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=x\r\n\r\n" +
		"--x\r\nContent-Type: text/html\r\n\r\nbody\r\n" +
		"--x\r\nContent-Type: text/html\r\nContent-Disposition: attachment; filename=receipt.html\r\n\r\nattachment\r\n--x--\r\n"

	got, err := Extract(strings.NewReader(raw), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.HTML) != "body" {
		t.Fatalf("HTML = %q", got.HTML)
	}
}

func TestExtractIgnoresHTMLInUnrelatedLaterBranch(t *testing.T) {
	raw := "MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=outer\r\n\r\n" +
		"--outer\r\nContent-Type: multipart/related; boundary=body\r\n\r\n" +
		"--body\r\nContent-Type: text/html\r\n\r\nselected\r\n--body--\r\n" +
		"--outer\r\nContent-Type: multipart/related; boundary=other\r\n\r\n" +
		"--other\r\nContent-Type: text/html\r\n\r\nunrelated\r\n--other--\r\n--outer--\r\n"

	got, err := Extract(strings.NewReader(raw), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.HTML) != "selected" {
		t.Fatalf("HTML = %q", got.HTML)
	}
}

func TestExtractNestedAlternativeSelectsItsHTMLRepresentation(t *testing.T) {
	raw := "MIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=outer\r\n\r\n" +
		"--outer\r\nContent-Type: text/html\r\n\r\nouter-html\r\n" +
		"--outer\r\nContent-Type: multipart/alternative; boundary=inner\r\n\r\n" +
		"--inner\r\nContent-Type: text/plain\r\n\r\nplain\r\n" +
		"--inner\r\nContent-Type: text/html\r\n\r\ninner-html\r\n--inner--\r\n--outer--\r\n"

	got, err := Extract(strings.NewReader(raw), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.HTML) != "inner-html" {
		t.Fatalf("HTML = %q", got.HTML)
	}
}

func TestExtractMixedSkipsChildrenWithoutHTML(t *testing.T) {
	raw := "MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=x\r\n\r\n" +
		"--x\r\nContent-Type: text/plain\r\n\r\nplain\r\n" +
		"--x\r\nContent-Type: text/html\r\n\r\nbody\r\n" +
		"--x\r\nContent-Type: text/html\r\n\r\nlater\r\n--x--\r\n"

	got, err := Extract(strings.NewReader(raw), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.HTML) != "body" {
		t.Fatalf("HTML = %q", got.HTML)
	}
}

func TestExtractRejectsMissingHTML(t *testing.T) {
	_, err := Extract(strings.NewReader("Content-Type: text/plain\r\n\r\nplain"), 1<<20)
	if !errors.Is(err, ErrMissingHTML) {
		t.Fatalf("error = %v", err)
	}
}

func TestExtractRejectsMalformedMIME(t *testing.T) {
	_, err := Extract(strings.NewReader("Bad Header\r\n\r\nbody"), 1<<20)
	if !errors.Is(err, ErrMalformedMIME) {
		t.Fatalf("error = %v", err)
	}
}

func TestExtractRejectsMessageLargerThanLimit(t *testing.T) {
	raw := "Content-Type: text/html\r\n\r\n" + strings.Repeat("x", 25<<20)
	_, err := Extract(strings.NewReader(raw), 25<<20)
	if !errors.Is(err, ErrMessageTooLarge) || !strings.Contains(err.Error(), "message too large") {
		t.Fatalf("error = %v", err)
	}
}

func TestExtractRejectsCIDImageLargerThanLimit(t *testing.T) {
	raw := "MIME-Version: 1.0\r\nContent-Type: multipart/related; boundary=x\r\n\r\n" +
		"--x\r\nContent-Type: text/html\r\n\r\nreceipt\r\n" +
		"--x\r\nContent-Type: image/png\r\nContent-ID: <large>\r\n\r\n" +
		strings.Repeat("x", (10<<20)+1) + "\r\n--x--\r\n"

	_, err := Extract(strings.NewReader(raw), 25<<20)
	if !errors.Is(err, ErrCIDTooLarge) {
		t.Fatalf("error = %v", err)
	}
}

func TestExtractSanitizesSourceReadError(t *testing.T) {
	const secret = "account-secret-123"
	_, err := Extract(errorReader{err: errors.New(secret)}, 1<<20)
	if !errors.Is(err, ErrReadMessage) {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error disclosed input-derived text: %q", err)
	}
}

func TestExtractSanitizesCIDDecodeError(t *testing.T) {
	const secret = "account-secret-123"
	raw := "MIME-Version: 1.0\r\nContent-Type: multipart/related; boundary=x\r\n\r\n" +
		"--x\r\nContent-Type: text/html\r\n\r\nreceipt\r\n" +
		"--x\r\nContent-Type: image/png\r\nContent-ID: <bad>\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		secret + "%%%\r\n--x--\r\n"

	_, err := Extract(strings.NewReader(raw), 1<<20)
	if !errors.Is(err, ErrMalformedMIME) {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error disclosed input-derived text: %q", err)
	}
}

func TestExtractAcceptsExactlyAggregateCIDLimit(t *testing.T) {
	raw := multipartWithCIDAssets(t, []int{10 << 20, 10 << 20, 10 << 20, 10 << 20, 10 << 20})
	doc, err := Extract(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var total int
	for _, asset := range doc.CID {
		total += len(asset.Data)
	}
	if total != 50<<20 {
		t.Fatalf("aggregate CID bytes = %d", total)
	}
}

func TestExtractRejectsByteBeyondAggregateCIDLimit(t *testing.T) {
	raw := multipartWithCIDAssets(t, []int{10 << 20, 10 << 20, 10 << 20, 10 << 20, 10 << 20, 1})
	_, err := Extract(bytes.NewReader(raw), int64(len(raw)))
	if !errors.Is(err, ErrCIDTotalTooLarge) {
		t.Fatalf("error = %v", err)
	}
}

func TestExtractRejectsInvalidMaxBytes(t *testing.T) {
	for _, maxBytes := range []int64{-1, math.MaxInt64} {
		t.Run(fmt.Sprint(maxBytes), func(t *testing.T) {
			_, err := Extract(strings.NewReader("Content-Type: text/html\r\n\r\nreceipt"), maxBytes)
			if !errors.Is(err, ErrInvalidLimit) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func multipartWithCIDAssets(t *testing.T, sizes []int) []byte {
	t.Helper()
	var raw bytes.Buffer
	io.WriteString(&raw, "MIME-Version: 1.0\r\nContent-Type: multipart/related; boundary=x\r\n\r\n")
	io.WriteString(&raw, "--x\r\nContent-Type: text/html\r\n\r\nreceipt\r\n")
	for i, size := range sizes {
		fmt.Fprintf(&raw, "--x\r\nContent-Type: image/png\r\nContent-ID: <%d>\r\n\r\n", i)
		io.CopyN(&raw, strings.NewReader(strings.Repeat("x", size)), int64(size))
		io.WriteString(&raw, "\r\n")
	}
	io.WriteString(&raw, "--x--\r\n")
	return raw.Bytes()
}
