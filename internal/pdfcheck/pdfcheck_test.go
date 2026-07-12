package pdfcheck

import (
	"fmt"
	"strings"
	"testing"
)

func TestVerifyRejectsEmptyInput(t *testing.T) {
	assertVerifyErrorContains(t, nil, "invalid PDF")
}

func TestVerifyRejectsInvalidPDF(t *testing.T) {
	assertVerifyErrorContains(t, []byte("not a PDF"), "invalid PDF")
}

func TestVerifyRejectsZeroPages(t *testing.T) {
	assertVerifyErrorContains(t, makePDF(t, nil), "page count")
}

func TestVerifyRejectsMultiplePages(t *testing.T) {
	boxes := [][4]float64{{0, 0, 419.528, 595.276}, {0, 0, 419.528, 595.276}}
	assertVerifyErrorContains(t, makePDF(t, boxes), "page count")
}

func TestVerifyRejectsWrongMediaBox(t *testing.T) {
	assertVerifyErrorContains(t, makePDF(t, [][4]float64{{0, 0, 612, 792}}), "paper dimensions")
}

func TestVerifyAcceptsOneA5PortraitPage(t *testing.T) {
	if err := Verify(makePDF(t, [][4]float64{{0, 0, 419.528, 595.276}})); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func assertVerifyErrorContains(t *testing.T, data []byte, want string) {
	t.Helper()
	err := Verify(data)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Verify() error = %v, want error containing %q", err, want)
	}
}

func makePDF(t *testing.T, boxes [][4]float64) []byte {
	t.Helper()

	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", pageReferences(len(boxes)), len(boxes)),
	}
	for i, box := range boxes {
		objects = append(objects, fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [%g %g %g %g] /Resources << >> /Contents %d 0 R >>",
			box[0], box[1], box[2], box[3], 3+len(boxes)+i,
		))
	}
	for range boxes {
		objects = append(objects, "<< /Length 0 >>\nstream\n\nendstream")
	}

	var pdf strings.Builder
	pdf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		offsets[i+1] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return []byte(pdf.String())
}

func pageReferences(count int) string {
	refs := make([]string, count)
	for i := range refs {
		refs[i] = fmt.Sprintf("%d 0 R", i+3)
	}
	return strings.Join(refs, " ")
}
