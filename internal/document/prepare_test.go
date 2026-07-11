package document

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"mail2receipt/internal/message"
)

func TestPrepareInlinesCIDReferences(t *testing.T) {
	doc := message.Document{
		HTML: []byte(`<html><head></head><body background="CID:Background@ID"><img src="cid:Logo@ID" srcset="CID:Small@ID 1x, https://example.com/large.png 2x"><div style="background-image: url('cId:Tile@ID')"></div></body></html>`),
		CID: map[string]message.Asset{
			"background@id": {MediaType: "image/gif", Data: []byte{1}},
			"logo@id":       {MediaType: "image/png", Data: []byte{1, 2, 3}},
			"small@id":      {MediaType: "image/jpeg", Data: []byte{2}},
			"tile@id":       {MediaType: "image/webp", Data: []byte{3}},
		},
	}

	got, err := Prepare(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`data:image/gif;base64,AQ==`,
		`data:image/png;base64,AQID`,
		`data:image/jpeg;base64,Ag== 1x`,
		`data:image/webp;base64,Aw==`,
		`https://example.com/large.png 2x`,
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("prepared HTML does not contain %q", want)
		}
	}
}

func TestPrepareLooksUpCIDAssetsCaseInsensitively(t *testing.T) {
	doc := message.Document{
		HTML: []byte(`<img src="cid:logo@example">`),
		CID: map[string]message.Asset{
			"Logo@Example": {MediaType: "image/png", Data: []byte{1, 2, 3}},
		},
	}

	got, err := Prepare(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`data:image/png;base64,AQID`)) {
		t.Fatalf("CID was not replaced: %s", got)
	}
}

func TestPreparePreservesCommaBearingSrcsetURLs(t *testing.T) {
	const remote = `https://example.com/image,cid:not-a-candidate.png?crop=1,2 1x`
	doc := message.Document{
		HTML: []byte(`<img srcset="` + remote + `, cid:Logo@ID 2x">`),
		CID: map[string]message.Asset{
			"logo@id": {MediaType: "image/png", Data: []byte{1, 2, 3}},
		},
	}

	got, err := Prepare(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{remote, `data:image/png;base64,AQID 2x`} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("prepared HTML does not contain %q: %s", want, got)
		}
	}
}

func TestPrepareRejectsInvalidCIDAssets(t *testing.T) {
	tests := []struct {
		name      string
		doc       message.Document
		want      error
		sensitive string
	}{
		{
			name:      "missing",
			doc:       message.Document{HTML: []byte(`<img src="cid:private-account-id">`)},
			want:      ErrMissingCID,
			sensitive: "private-account-id",
		},
		{
			name: "not image",
			doc: message.Document{
				HTML: []byte(`<div style="background:url(cid:private-file-id)"></div>`),
				CID:  map[string]message.Asset{"private-file-id": {MediaType: "text/plain", Data: []byte("secret-content")}},
			},
			want:      ErrNonImageCID,
			sensitive: "private-file-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Prepare(tt.doc)
			if err == nil {
				t.Fatal("Prepare returned nil error")
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want errors.Is(_, %v)", err, tt.want)
			}
			for _, sensitive := range []string{tt.sensitive, "secret-content"} {
				if strings.Contains(err.Error(), sensitive) {
					t.Fatalf("error leaks sensitive value %q: %v", sensitive, err)
				}
			}
		})
	}
}

func TestPreparePreservesRemoteResources(t *testing.T) {
	const input = `<html><head><script src="https://example.com/app.js"></script></head><body><img src="HTTP://example.com/a.png" srcset="https://example.com/a.png 1x, http://example.com/b.png 2x"><a href="https://example.com/">link</a><div style="background:url(https://example.com/bg.png)"></div></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`https://example.com/app.js`, `HTTP://example.com/a.png`,
		`https://example.com/a.png 1x`, `http://example.com/b.png 2x`,
		`https://example.com/`, `https://example.com/bg.png`,
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("prepared HTML does not contain %q", want)
		}
	}
}

func TestPrepareInjectsOnePrintStyleAsFinalHeadChild(t *testing.T) {
	got, err := Prepare(message.Document{HTML: []byte(`<html><head><title>Receipt</title></head><body></body></html>`)})
	if err != nil {
		t.Fatal(err)
	}

	const css = `@page { size: A5 portrait; margin: 8mm; }
html, body { margin: 0; padding: 0; }`
	if count := bytes.Count(got, []byte(css)); count != 1 {
		t.Fatalf("print style count = %d", count)
	}
	headEnd := strings.Index(string(got), "</head>")
	styleEnd := strings.Index(string(got), "</style>")
	if headEnd < 0 || styleEnd < 0 || strings.TrimSpace(string(got[styleEnd+len("</style>"):headEnd])) != "" {
		t.Fatalf("print style is not the final head child: %s", got)
	}
}

func TestPrepareLeavesMalformedCIDTextUnchanged(t *testing.T) {
	const input = `<html><head></head><body><img src="cid:"><div style="background:url('cid:bad id')"></div></body></html>`
	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`src="cid:"`)) || !bytes.Contains(got, []byte(`cid:bad id`)) {
		t.Fatalf("malformed CID text was changed: %s", got)
	}
}
