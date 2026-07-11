package message

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

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

func TestExtractKeepsFinalHTMLPart(t *testing.T) {
	raw := "MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=x\r\n\r\n" +
		"--x\r\nContent-Type: text/html\r\n\r\nfirst\r\n" +
		"--x\r\nContent-Type: text/html\r\n\r\nfinal\r\n--x--\r\n"

	got, err := Extract(strings.NewReader(raw), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.HTML) != "final" {
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
