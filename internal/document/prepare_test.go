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
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("prepared HTML does not contain %q", want)
		}
	}
	if bytes.Contains(got, []byte(`https://example.com/large.png`)) {
		t.Fatalf("prepared HTML retained remote srcset candidate: %s", got)
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
	if bytes.Contains(got, []byte(remote)) {
		t.Errorf("prepared HTML retained remote srcset URL: %s", got)
	}
	if !bytes.Contains(got, []byte(`data:image/png;base64,AQID 2x`)) {
		t.Errorf("prepared HTML does not contain CID candidate: %s", got)
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

func TestPrepareEmbedsKnownLogosAndRemovesRemoteImages(t *testing.T) {
	const input = `<html><head><style>
      .remote { background-image: url("https://images.example/bg.png"); }
      .gray { border-bottom: 1px solid #ededed; color: #EDEDED; }
    </style></head><body background="https://images.example/body.png">
      <a href="https://example.com/order?id=private">order</a>
      <img id="lockup" src="https://images.example/google-play-crm-lockup-ic-h-transparent-w688px-h140px-2x.png">
      <img id="logo" src="./google-play-crm-logo-transparent-w192px-h192px-2x.png">
      <img id="tracking" src="https://images.example/tracking.png">
      <div style="background: #EDEDED url('https://images.example/tile.png'); color: #123456">text</div>
    </body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if count := strings.Count(text, "data:image/png;base64,"); count != 2 {
		t.Fatalf("embedded PNG count = %d", count)
	}
	if strings.Contains(text, "images.example") {
		t.Fatalf("prepared HTML retained remote image host")
	}
	if !strings.Contains(text, `href="https://example.com/order?id=private"`) {
		t.Fatalf("ordinary hyperlink was removed")
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "border-bottom: 1px solid #ededed") {
		t.Fatalf("gray separator was retained")
	}
	if !strings.Contains(lower, "color: #ededed") {
		t.Fatalf("text color was removed")
	}
	if !strings.Contains(lower, "color: #123456") {
		t.Fatalf("unrelated text color was removed")
	}
}

func TestPrepareNormalizesOfflineImageReferences(t *testing.T) {
	doc := message.Document{
		HTML: []byte(`<html><head><style>
@media print { .cid { background: url("cid:tile@id"); } }
.mixed { background: red; background-image: url(https://images.example/style.png); color: blue; }
</style></head><body background="https://images.example/body.png">
<img srcset="https://images.example/remote.png 1x, ./google-play-crm-logo-transparent-w192px-h192px-2x.png 2x, cid:tile@id 3x">
<div style="background-image: url(https://images.example/inline.png); content: 'a;b:c'; color: red"></div>
</body></html>`),
		CID: map[string]message.Asset{"tile@id": {MediaType: "image/gif", Data: []byte{1}}},
	}

	got, err := Prepare(doc)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "images.example") {
		t.Fatalf("prepared HTML retained remote image reference: %s", got)
	}
	for _, want := range []string{
		"data:image/png;base64,",
		"data:image/gif;base64,AQ== 3x",
		`url("data:image/gif;base64,AQ==")`,
		`content: &#39;a;b:c&#39;`,
		`color: red`,
		`color: blue`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML does not contain %q: %s", want, got)
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
