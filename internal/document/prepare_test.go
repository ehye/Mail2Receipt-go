package document

import (
	"bytes"
	"errors"
	stdhtml "html"
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

func TestPrepareRemovesAllNonEmbeddedResources(t *testing.T) {
	const input = `<html><head>
<base href="C:\private\receipts\">
<link rel="stylesheet" href="//files.example/receipt.css">
<link rel="icon" href="file:///C:/private/icon.png">
<style>@import "local.css"; .local { background:url(../private/tile.png) }</style>
</head><body background="\\server\share\body.png">
<img src="C:\private\logo.png" srcset="/root/image.png 1x, cid:safe@id 2x">
<object data="../private/object.bin"></object><video poster="//files.example/poster.png"></video>
<svg><image href="file:///C:/private/vector.png"></image><use href="sprite.svg#icon"></use></svg>
<div style="background:url(file:///C:/private/tile.png)"></div>
<a href="file:///C:/navigation">link</a><area href="//example.com/map"><form action="relative-submit"></form>
<img src="https://example.com/google-play-crm-logo-transparent-w192px-h192px-2x.png">
</body></html>`

	got, err := Prepare(message.Document{
		HTML: []byte(input),
		CID:  map[string]message.Asset{"safe@id": {MediaType: "image/gif", Data: []byte{1}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, unsafe := range []string{"<base", "files.example", "private", "local.css", "sprite.svg", "root/image.png"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(unsafe)) {
			t.Errorf("prepared HTML retained non-embedded resource marker %q", unsafe)
		}
	}
	for _, safe := range []string{
		"data:image/gif;base64,AQ== 2x",
		"data:image/png;base64,",
		`href="file:///C:/navigation"`,
		`href="//example.com/map"`,
		`action="relative-submit"`,
	} {
		if !strings.Contains(text, safe) {
			t.Errorf("prepared HTML does not contain safe value %q", safe)
		}
	}
}

func TestPrepareDecodesCSSResourceEscapes(t *testing.T) {
	const input = `<html><head><style>
.escaped-identifier { background: u\72l(local.png) }
.escaped-scheme { background: url(h\74tps://assets.example/image.png) }
@import u\72l(\\server\share\receipt.css);
.safe { background:url(data:image/png;base64,AQID); color: navy }
</style></head><body style="background:u\72l(file:///C:/private/tile.png);color:green"></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, unsafe := range []string{`u\72l`, `h\74tps`, "assets.example", "local.png", "server", "private"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(unsafe)) {
			t.Errorf("prepared HTML retained escaped resource marker %q", unsafe)
		}
	}
	for _, safe := range []string{"data:image/png;base64,AQID", "color: navy", "color:green"} {
		if !strings.Contains(text, safe) {
			t.Errorf("prepared HTML removed safe CSS %q", safe)
		}
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

func TestPrepareRemovesRemoteLinkResourcesButPreservesHyperlinks(t *testing.T) {
	const input = `<html><head>
<link rel="stylesheet" href="https://assets.example/receipt.css">
<link rel="icon" href="HTTP://assets.example/icon.png">
<link rel="preload" href="https://assets.example/font.woff2">
</head><body><a href="https://example.com/order">order</a><area href="https://example.com/map"></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "assets.example") {
		t.Fatalf("prepared HTML retained remote link resource: %s", got)
	}
	for _, want := range []string{`href="https://example.com/order"`, `href="https://example.com/map"`} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML removed hyperlink %q: %s", want, got)
		}
	}
}

func TestPrepareRemovesCSSImports(t *testing.T) {
	const input = `<html><head><style>
/* lead comment */ @import "https://assets.example/quoted.css" screen;
@import url('HTTP://assets.example/url.css');
@import "local.css";
.receipt { color: green; }
</style></head><body></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "assets.example") {
		t.Fatalf("prepared HTML retained remote CSS import: %s", got)
	}
	for _, want := range []string{`color: green`} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML removed nonremote CSS %q: %s", want, got)
		}
	}
}

func TestPrepareScansStylesheetBracesOutsideQuotesAndComments(t *testing.T) {
	const input = `<html><head><style>
/* misleading } { braces */
.receipt { content: "literal } and { braces"; background: url(https://assets.example/bypass.png); color: navy; }
</style></head><body></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "assets.example") {
		t.Fatalf("prepared HTML retained remote CSS after misleading braces: %s", got)
	}
	for _, want := range []string{`literal } and { braces`, `color: navy`} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML removed CSS %q: %s", want, got)
		}
	}
}

func TestPrepareUsesBackslashParityForCSSQuotes(t *testing.T) {
	const input = `<html><head></head><body><div style='content: "even\\"; background: url(https://assets.example/bypass.png); color: maroon'></div><div style='content: "odd\";still quoted"; color: teal'></div></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "assets.example") {
		t.Fatalf("prepared HTML retained remote CSS after even backslashes: %s", got)
	}
	for _, want := range []string{`even\\`, `color: maroon`, `odd\&#34;;still quoted`, `color: teal`} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML removed CSS %q: %s", want, got)
		}
	}
}

func TestPrepareNormalizesUnclosedStylesheetBlock(t *testing.T) {
	const input = `<html><head><style>.receipt { color: purple; border-color: #EDEDED; background: url(https://assets.example/unclosed.png); font-weight: bold` + `</style></head><body></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, unwanted := range []string{"assets.example", "border-color"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(unwanted)) {
			t.Errorf("prepared HTML retained unsafe unclosed-block CSS %q: %s", unwanted, got)
		}
	}
	for _, want := range []string{"color: purple", "font-weight: bold"} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML removed safe unclosed-block CSS %q: %s", want, got)
		}
	}
}

func TestPrepareRemovesAllBaseElements(t *testing.T) {
	const input = `<html><head><base id="remote" href="https://assets.example/receipts/"><base id="local" href="/receipts/"></head><body><img src="relative.png"></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "assets.example") {
		t.Fatalf("prepared HTML retained remote base URL: %s", got)
	}
	if strings.Contains(text, `<base`) {
		t.Fatalf("prepared HTML retained a base element: %s", got)
	}
}

func TestPrepareRemovesRemoteLoadBearingAttributesAndPreservesNavigation(t *testing.T) {
	const input = `<html><head></head><body>
<object data="https://assets.example/object.bin"></object>
<video poster="https://assets.example/poster.png"></video>
<input poster="https://assets.example/input-poster.png">
<svg><image href="https://assets.example/vector.png"></image><use href="https://assets.example/sprite.svg#icon"></use></svg>
<form action="https://example.com/submit"></form><a href="https://example.com/order">order</a><area href="https://example.com/map">
</body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "assets.example") {
		t.Fatalf("prepared HTML retained a remote load-bearing attribute: %s", got)
	}
	for _, want := range []string{
		`action="https://example.com/submit"`,
		`href="https://example.com/order"`,
		`href="https://example.com/map"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML removed navigation target %q: %s", want, got)
		}
	}
}

func TestPrepareRemovesRemoteCSSImportsAfterComments(t *testing.T) {
	const input = `<html><head><style>
@import /* quoted */ "https://assets.example/commented.css";
@import /* function */ url(https://assets.example/commented-url.css);
.receipt { color: olive; }
</style></head><body></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "assets.example") {
		t.Fatalf("prepared HTML retained remote import after comment: %s", got)
	}
	if !strings.Contains(text, "color: olive") {
		t.Fatalf("prepared HTML removed safe CSS: %s", got)
	}
}

func TestPrepareNeutralizesRemoteMetaRefresh(t *testing.T) {
	const input = `<html><head>
<meta HTTP-EQUIV=" Refresh " content="0; URL=https://assets.example/plain">
<meta http-equiv="refresh" content=" 5 ; url = 'HTTP://assets.example/quoted' ">
<meta name="description" content="safe receipt">
<meta http-equiv="refresh" content="10; url=/local/receipt">
</head><body></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "assets.example") {
		t.Fatalf("prepared HTML retained remote meta refresh: %s", got)
	}
	for _, want := range []string{`name="description" content="safe receipt"`} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML removed harmless meta content %q: %s", want, got)
		}
	}
}

func TestPrepareNeutralizesDirectRemoteMetaRefreshTargets(t *testing.T) {
	const input = `<html><head>
<meta http-equiv="refresh" content="0; https://assets.example/direct">
<meta http-equiv="refresh" content="1; 'HTTP://assets.example/quoted'">
<meta http-equiv="refresh" content='2; "https://assets.example/double-quoted"'>
<meta http-equiv="refresh" content="3; /local/receipt">
<meta http-equiv="refresh" content="invalid refresh value">
<meta http-equiv="refresh" content="4; url">
</head><body></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "assets.example") {
		t.Fatalf("prepared HTML retained direct remote meta refresh: %s", got)
	}
	for _, want := range []string{
		`content="invalid refresh value"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML removed nonremote or invalid refresh %q: %s", want, got)
		}
	}
}

func TestPrepareNormalizesIframeSrcdocOffline(t *testing.T) {
	nested := `<html><head><base href="https://assets.example/"><meta http-equiv="refresh" content="0;url=https://assets.example/next"><style>.remote { background:url(https://assets.example/bg.png) } .safe { color: navy }</style></head><body><img src="https://assets.example/remote.png"><img src="./google-play-crm-logo-transparent-w192px-h192px-2x.png"><p>safe nested text</p></body></html>`
	input := `<html><head></head><body><iframe srcdoc="` + stdhtml.EscapeString(nested) + `"></iframe></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "assets.example") {
		t.Fatalf("prepared HTML retained remote srcdoc resource: %s", got)
	}
	for _, want := range []string{"data:image/png;base64,", "color: navy", "safe nested text"} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML removed safe nested content %q: %s", want, got)
		}
	}
}

func TestPrepareLimitsIframeSrcdocNesting(t *testing.T) {
	const wantDepth = 4
	nested := `<p>safe outer text</p>`
	for i := 0; i < wantDepth+2; i++ {
		nested = `<iframe srcdoc="` + stdhtml.EscapeString(nested) + `"></iframe>`
	}
	input := `<html><head></head><body>` + nested + `</body></html>`

	first, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatalf("first Prepare error = %v", err)
	}
	second, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatalf("second Prepare error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("over-limit srcdoc normalization was not deterministic")
	}
	if count := bytes.Count(first, []byte("srcdoc=")); count != wantDepth {
		t.Fatalf("srcdoc attribute count = %d, want %d: %s", count, wantDepth, first)
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

func TestPrepareRemovesMalformedCIDResources(t *testing.T) {
	const input = `<html><head></head><body><img src="cid:"><div style="background:url('cid:bad id')"></div></body></html>`
	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte(`cid:`)) {
		t.Fatalf("malformed CID resource was retained: %s", got)
	}
}
