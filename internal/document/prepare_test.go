package document

import (
	"bytes"
	"encoding/base64"
	"errors"
	stdhtml "html"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	nethtml "golang.org/x/net/html"

	"mail2receipt/internal/message"
)

func tinyImage(t *testing.T, mediaType string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff})
	var data bytes.Buffer
	var err error
	switch mediaType {
	case "image/png":
		err = png.Encode(&data, img)
	case "image/jpeg":
		err = jpeg.Encode(&data, img, nil)
	case "image/gif":
		err = gif.Encode(&data, img, nil)
	default:
		t.Fatalf("unsupported test media type %q", mediaType)
	}
	if err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func imageDataURL(mediaType string, data []byte) string {
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func TestPrepareRemovesDeclarativeShadowRootAttributes(t *testing.T) {
	input := `<html><head></head><body>
<div id="open"><template SHADOWROOTMODE="open" shadowrootdelegatesfocus shadowrootclonable shadowrootserializable><span>open template content</span></template></div>
<div id="closed"><template shadowrootmode="closed" ShadowRootFutureOption="yes"><span>closed template content</span></template></div>
<div shadowrootmode="open" shadowrootcustom="value">ordinary element</div>
</body></html>`

	prepared, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := nethtml.Parse(bytes.NewReader(prepared))
	if err != nil {
		t.Fatal(err)
	}
	templates := 0
	var content strings.Builder
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node.Type == nethtml.ElementNode {
			if node.Data == "template" {
				templates++
			}
			for _, attr := range node.Attr {
				if strings.HasPrefix(strings.ToLower(attr.Key), "shadowroot") {
					t.Errorf("prepared element retained declarative shadow attribute %q", attr.Key)
				}
			}
		}
		if node.Type == nethtml.TextNode {
			content.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(reparsed)
	if templates != 2 {
		t.Fatalf("prepared template count = %d, want 2", templates)
	}
	for _, want := range []string{"open template content", "closed template content", "ordinary element"} {
		if !strings.Contains(content.String(), want) {
			t.Errorf("prepared HTML lost inert content %q", want)
		}
	}
}

func TestPrepareInlinesCIDReferences(t *testing.T) {
	gifData := tinyImage(t, "image/gif")
	pngData := tinyImage(t, "image/png")
	jpegData := tinyImage(t, "image/jpeg")
	doc := message.Document{
		HTML: []byte(`<html><head></head><body background="CID:Background@ID"><img src="cid:Logo@ID" srcset="CID:Small@ID 1x, https://example.com/large.png 2x"><div style="background-image: url('cId:Tile@ID')"></div></body></html>`),
		CID: map[string]message.Asset{
			"background@id": {MediaType: "image/gif", Data: gifData},
			"logo@id":       {MediaType: "image/png", Data: pngData},
			"small@id":      {MediaType: "image/jpeg", Data: jpegData},
			"tile@id":       {MediaType: "image/png", Data: pngData},
		},
	}

	got, err := Prepare(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		imageDataURL("image/gif", gifData),
		imageDataURL("image/png", pngData),
		imageDataURL("image/jpeg", jpegData) + ` 1x`,
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
	pngData := tinyImage(t, "image/png")
	doc := message.Document{
		HTML: []byte(`<img src="cid:logo@example">`),
		CID: map[string]message.Asset{
			"Logo@Example": {MediaType: "image/png", Data: pngData},
		},
	}

	got, err := Prepare(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(imageDataURL("image/png", pngData))) {
		t.Fatalf("CID was not replaced: %s", got)
	}
}

func TestPreparePreservesCommaBearingSrcsetURLs(t *testing.T) {
	pngData := tinyImage(t, "image/png")
	const remote = `https://example.com/image,cid:not-a-candidate.png?crop=1,2 1x`
	doc := message.Document{
		HTML: []byte(`<img srcset="` + remote + `, cid:Logo@ID 2x">`),
		CID: map[string]message.Asset{
			"logo@id": {MediaType: "image/png", Data: pngData},
		},
	}

	got, err := Prepare(doc)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte(remote)) {
		t.Errorf("prepared HTML retained remote srcset URL: %s", got)
	}
	if !bytes.Contains(got, []byte(imageDataURL("image/png", pngData)+` 2x`)) {
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
	if !strings.Contains(lower, "border-bottom: 1px solid #ededed") {
		t.Fatalf("gray separator was removed")
	}
	if !strings.Contains(lower, "color: #ededed") {
		t.Fatalf("text color was removed")
	}
	if !strings.Contains(lower, "color: #123456") {
		t.Fatalf("unrelated text color was removed")
	}
}

func TestPrepareRemovesExactDecorativeImageElementsAndComments(t *testing.T) {
	const input = `<html><head><style>.keep{background:url(email_top.png.bak)}</style></head><body>
<!-- decorative https://assets.example/EMAIL_TOP.PNG?version=1 -->
<!-- keep email_top.png.bak -->
<div id="top" style="background-image:url('https://assets.example/a/email_top.png#x')"><span>remove top</span></div>
<picture id="mid-wrapper"><source id="mid" srcset="https://assets.example/EMAIL_MID.PNG?x=1 1x"><span>keep narrow wrapper</span></picture>
<object id="bottom" data="../email_bottom.png?download=1"><span>remove bottom</span></object>
<div id="similar"><img src="email_top.png.bak"><span>keep similar</span></div>
<a id="navigation" href="https://example.com/email_top.png">keep navigation</a>
</body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, unwanted := range []string{`id="top"`, `id="mid"`, `id="bottom"`, "remove top", "remove bottom", "decorative https://assets.example/EMAIL_TOP.PNG"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("prepared HTML retained decorative content %q", unwanted)
		}
	}
	for _, want := range []string{`id="mid-wrapper"`, "keep narrow wrapper", `id="similar"`, "keep similar", "email_top.png.bak", `id="navigation"`, "keep navigation"} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML removed unrelated content %q", want)
		}
	}
}

func TestPrepareRemovesStyleElementsReferencingDecorativeImages(t *testing.T) {
	const input = `<html><head>
<style id="top">.legacy{background:url("https://assets.example/EMAIL_TOP.PNG?version=1")}.lost{color:red}</style>
<style id="mid">.legacy{background-image:image-set(url(../email_mid.png#x) 1x)}</style>
<style id="bottom">.legacy{mask-image:cross-fade(url(email_bottom.png), black)}</style>
<style id="similar">.keep{background:url(email_top.png.bak);color:green}</style>
<style id="text">.keep::before{content:"url(email_mid.png)";color:blue}</style>
</head><body></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, unwanted := range []string{`id="top"`, `id="mid"`, `id="bottom"`, ".lost"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("prepared HTML retained decorative style element content %q", unwanted)
		}
	}
	for _, want := range []string{`id="similar"`, "color:green", `id="text"`, "url(email_mid.png)"} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML removed unrelated style content %q", want)
		}
	}
}

func TestPrepareRemovesCommentsReferencingDecorativeURLBasenames(t *testing.T) {
	const input = `<html><head></head><body>
<!-- background: url(email_top.png) -->
<!-- background: URL("../EMAIL_MID.PNG?version=1") -->
<!-- asset=https://assets.example/path/email_bottom.png#footer -->
<!-- keep url(email_top.png.bak) -->
<!-- keep prefixemail_mid.png -->
<!-- keep email_bottom.png.extra -->
</body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, unwanted := range []string{"background: url(email_top.png)", `URL("../EMAIL_MID.PNG`, "path/email_bottom.png#footer"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("prepared HTML retained decorative comment %q", unwanted)
		}
	}
	for _, want := range []string{"url(email_top.png.bak)", "prefixemail_mid.png", "email_bottom.png.extra"} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML removed unrelated comment %q", want)
		}
	}
}

func TestPrepareRemovesDecorativeReferencesOnlyFromImageBearingElements(t *testing.T) {
	const input = `<html><head></head><body>
<img id="src" src="email_top.png"><img id="srcset" srcset="email_mid.png 1x">
<table><tr><td id="cell" background="email_bottom.png">remove cell</td></tr></table>
<video id="video" poster="email_top.png">remove video</video>
<object id="object" data="email_mid.png">remove object</object>
<svg><image id="svg-image" href="email_bottom.png"></image></svg>
<div id="style" style="background:url(email_top.png)">remove style</div>
</body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, unwanted := range []string{`id="src"`, `id="srcset"`, `id="cell"`, `id="video"`, `id="object"`, `id="svg-image"`, `id="style"`, "remove cell", "remove video", "remove object", "remove style"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("prepared HTML retained decorative image-bearing element %q", unwanted)
		}
	}
}

func TestPrepareRemovesBodyWithDecorativeBackground(t *testing.T) {
	const input = `<html><head></head><body id="body" background="email_top.png"><p>remove body content</p></body></html>`
	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, unwanted := range []string{`id="body"`, "remove body content"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("prepared HTML retained decorative body content %q", unwanted)
		}
	}
}

func TestPreparePreservesNonImageElementsWithDecorativeNamedAttributes(t *testing.T) {
	const input = `<html><head></head><body>
<script id="script" src="email_top.png">keep script</script>
<iframe id="iframe" src="email_mid.png">keep iframe</iframe>
<div id="poster" poster="email_bottom.png">keep poster element</div>
<div id="srcset" srcset="email_top.png 1x">keep srcset element</div>
<div id="background" background="email_mid.png">keep background element</div>
<div id="data" data="email_bottom.png">keep data element</div>
</body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{`id="script"`, "keep script", `id="iframe"`, "keep iframe", `id="poster"`, "keep poster element", `id="srcset"`, "keep srcset element", `id="background"`, "keep background element", `id="data"`, "keep data element"} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML removed non-image element content %q", want)
		}
	}
	for _, removedAttribute := range []string{`src="email_top.png"`, `src="email_mid.png"`, `poster="email_bottom.png"`, `srcset="email_top.png 1x"`, `background="email_mid.png"`} {
		if strings.Contains(text, removedAttribute) {
			t.Errorf("prepared HTML retained normalized remote attribute %q", removedAttribute)
		}
	}
}

func TestPreparePreservesEDEDEDBordersAndTextOnly(t *testing.T) {
	const input = `<html><head><style>.x{border:1px solid #EDEDED;border-inline-start-color:#ededed;color:#ededed;background:#ededed;outline-color:#ededed;fill:#ededed}</style></head><body style="border-top-color:#EDEDED;background-color:#ededed;color:#EDEDED"></body></html>`
	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(got))
	for _, want := range []string{"border:1px solid #ededed", "border-inline-start-color:#ededed", "border-top-color:#ededed", "color:#ededed"} {
		if !strings.Contains(lower, want) {
			t.Errorf("prepared HTML removed approved declaration %q", want)
		}
	}
	for _, unwanted := range []string{"background:#ededed", "background-color:#ededed", "outline-color:#ededed", "fill:#ededed"} {
		if strings.Contains(lower, unwanted) {
			t.Errorf("prepared HTML retained non-text declaration %q", unwanted)
		}
	}
}

func TestKeepDeclarationRecognizesOnlyActualEDEDEDBorderProperties(t *testing.T) {
	for _, property := range []string{
		"border", "border-color",
		"border-top", "border-right", "border-bottom", "border-left",
		"border-top-color", "border-right-color", "border-bottom-color", "border-left-color",
		"border-block", "border-inline", "border-block-color", "border-inline-color",
		"border-block-start", "border-block-end", "border-inline-start", "border-inline-end",
		"border-block-start-color", "border-block-end-color", "border-inline-start-color", "border-inline-end-color",
	} {
		if !keepDeclaration(property, "1px solid #EDEDED") {
			t.Errorf("keepDeclaration(%q) = false, want true", property)
		}
	}

	for _, property := range []string{
		"background", "outline-color", "border-topography", "border-inlinefoo", "border-blocked",
		"border-top-colorized", "border-inline-started", "my-border", "border-image",
	} {
		if keepDeclaration(property, "#EDEDED") {
			t.Errorf("keepDeclaration(%q) = true, want false", property)
		}
	}
}

func TestPrepareRemovesAllNonEmbeddedResources(t *testing.T) {
	gifData := tinyImage(t, "image/gif")
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
		CID:  map[string]message.Asset{"safe@id": {MediaType: "image/gif", Data: gifData}},
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
		imageDataURL("image/gif", gifData) + " 2x",
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
.source-data { background:url(data:image/png;base64,AQID); color: navy }
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
	if strings.Contains(text, "data:image/png;base64,AQID") {
		t.Fatal("prepared HTML retained source-authored data URL")
	}
	for _, safe := range []string{"color: navy", "color:green"} {
		if !strings.Contains(text, safe) {
			t.Errorf("prepared HTML removed safe CSS %q", safe)
		}
	}
}

func TestPrepareFiltersImageSetStringResources(t *testing.T) {
	tests := []struct {
		name  string
		value string
		keep  bool
	}{
		{name: "source data strings", value: `image-set("data:image/png;base64,AQID" type("image/png") 1x, 'data:image/gif;base64,BAUG' 2x)`, keep: false},
		{name: "source data URL entries", value: `image-set(url("data:image/png;base64,AQID") 1x, url(data:image/gif;base64,BAUG) 2x)`, keep: false},
		{name: "source data vendor function", value: `-webkit-image-set("data:image/png;base64,AQID" 1x)`, keep: false},
		{name: "unrelated function suffix", value: `my-image-set("safe string")`, keep: true},
		{name: "file", value: `image-set("file:///C:/private/image.png" 1x)`, keep: false},
		{name: "UNC", value: `image-set("\\server\share\image.png" 1x)`, keep: false},
		{name: "relative", value: `image-set("../private/image.png" 1x)`, keep: false},
		{name: "protocol relative", value: `image-set("//images.example/image.png" 1x)`, keep: false},
		{name: "HTTP", value: `image-set("https://images.example/image.png" 1x)`, keep: false},
		{name: "unsafe URL entry", value: `image-set(url("file:///C:/private/image.png") 1x)`, keep: false},
		{name: "mixed", value: `image-set("data:image/png;base64,AQID" 1x, "local.png" 2x)`, keep: false},
		{name: "escaped function", value: `image-s\65t("local.png" 1x)`, keep: false},
		{name: "escaped vendor function and scheme", value: `-webkit-image-s\65t("f\69le:///C:/private/image.png" 1x)`, keep: false},
		{name: "escaped HTTP scheme", value: `image-set("h\74tps://images.example/image.png" 1x)`, keep: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `<html><head><style>.receipt { background-image:` + tt.value + `; color: navy }</style></head>` +
				`<body style="background-image:` + tt.value + `;color:green"><p style="content:'safe string';font-family:'Receipt Sans'">text</p></body></html>`
			got, err := Prepare(message.Document{HTML: []byte(input)})
			if err != nil {
				t.Fatal(err)
			}
			text := string(got)
			if retained := strings.Contains(text, "background-image"); retained != tt.keep {
				t.Errorf("background-image retained = %v, want %v", retained, tt.keep)
			}
			for _, safe := range []string{"color: navy", "color:green", "safe string", "Receipt Sans"} {
				if !strings.Contains(text, safe) {
					t.Errorf("safe non-resource CSS %q was removed", safe)
				}
			}
		})
	}
}

func TestPrepareNormalizesOfflineImageReferences(t *testing.T) {
	gifData := tinyImage(t, "image/gif")
	doc := message.Document{
		HTML: []byte(`<html><head><style>
@media print { .cid { background: url("cid:tile@id"); } }
.mixed { background: red; background-image: url(https://images.example/style.png); color: blue; }
</style></head><body background="https://images.example/body.png">
<img srcset="https://images.example/remote.png 1x, ./google-play-crm-logo-transparent-w192px-h192px-2x.png 2x, cid:tile@id 3x">
<div style="background-image: url(https://images.example/inline.png); content: 'a;b:c'; color: red"></div>
</body></html>`),
		CID: map[string]message.Asset{"tile@id": {MediaType: "image/gif", Data: gifData}},
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
		imageDataURL("image/gif", gifData) + " 3x",
		`url("` + imageDataURL("image/gif", gifData) + `")`,
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
	for _, unwanted := range []string{"assets.example"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(unwanted)) {
			t.Errorf("prepared HTML retained unsafe unclosed-block CSS %q: %s", unwanted, got)
		}
	}
	for _, want := range []string{"color: purple", "border-color: #EDEDED", "font-weight: bold"} {
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

	const css = `@page { size: A5 portrait; margin: 0; }
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

func TestPrepareIncreasesSimpleTopLevelFontSizes(t *testing.T) {
	const input = `<html><head><style>.a{font-size:10px}.b{font-size:2.5em !important}.c{font-size:0}.cz{font-size:0PX}.u{font-size:10PX !IMPORTANT}.d{font-size:large}.e{font-size:calc(10px + 1vw)}.f{font-size:var(--size)}.g{font-size:12wat}</style></head><body><p style="font-size: 8pt !important">Text</p></body></html>`
	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{"font-size:10.5px", "font-size:2.625em !important", "font-size:0", "font-size:0PX", "font-size:10.5PX !IMPORTANT", "font-size: 8.4pt !important", "font-size:large", "font-size:calc(10px + 1vw)", "font-size:var(--size)", "font-size:12wat"} {
		if !strings.Contains(text, want) {
			t.Errorf("prepared HTML missing expected CSS %q", want)
		}
	}
}

func TestPrepareStripsSourceAuthoredEmphasisMarkers(t *testing.T) {
	const input = `<html><head></head><body><div id="forged" data-mail2receipt-emphasis="forged">Ordinary text</div><div id="legitimate">See your details<span id="child" data-mail2receipt-emphasis>continued</span></div></body></html>`
	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	root, err := nethtml.Parse(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	marked := map[string]bool{}
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		id, marker := "", false
		for _, attr := range node.Attr {
			if attr.Key == "id" {
				id = attr.Val
			}
			if attr.Key == "data-mail2receipt-emphasis" {
				marker = true
			}
		}
		if marker {
			marked[id] = true
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if marked["forged"] || marked["child"] {
		t.Error("source-authored emphasis marker was retained")
	}
	if !marked["legitimate"] {
		t.Error("forged descendant marker suppressed legitimate ancestor marking")
	}
}

func TestPrepareScalesDeclarationsAroundNestedCSSRules(t *testing.T) {
	const input = `<html><head><style>@media print { font-size:10px; .child { font-size:20px } font-size:30px; }</style></head><body></body></html>`
	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{"font-size:10.5px", ".child { font-size:21px }", "font-size:31.5px"} {
		if !strings.Contains(text, want) {
			t.Errorf("nested stylesheet missing %q", want)
		}
	}
}

func TestPrepareMarksApprovedEmphasisPrefixes(t *testing.T) {
	const input = `<html><head></head><body><div id="first">  BY subscribing, <a>you authorize us to continue</a><p>Questions about this?</p></div><section id="second"><span>See</span>   your details</section><div id="punctuation">See your: details</div><div id="substring">Intro: See your details</div><div id="yourself">See yourself</div><div id="authorize-today">By subscribing, you authorize us today</div><div id="other">Unrelated</div></body></html>`
	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	root, err := nethtml.Parse(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	marked := map[string]bool{}
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node.Type == nethtml.ElementNode {
			id, marker := "", false
			for _, attr := range node.Attr {
				if attr.Key == "id" {
					id = attr.Val
				}
				if attr.Key == "data-mail2receipt-emphasis" {
					marker = true
				}
			}
			if marker {
				marked[id] = true
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if !marked["first"] || !marked["second"] || !marked["punctuation"] {
		t.Errorf("approved prefix containers were not all marked")
	}
	if marked["substring"] || marked["yourself"] || marked["authorize-today"] || marked["other"] {
		t.Errorf("non-prefix container was marked")
	}
	css := string(got)
	for _, want := range []string{`[data-mail2receipt-emphasis]`, `[data-mail2receipt-emphasis] *`, `font-size: 12px !important`, `line-height: 18px !important`} {
		if !strings.Contains(css, want) {
			t.Errorf("injected CSS missing %q", want)
		}
	}
}

func TestPrepareForcesEmphasisInlineOnContainerAndDescendants(t *testing.T) {
	const input = `<html><head><style>#target, #child { font-size:40px !important; line-height:2 !important }</style></head><body><div id="target" style="color:red;font-size:30px !important;line-height:3!important">See your details <span id="child" style="font-size:20px!important;line-height:4 !important;font-weight:bold">now</span></div></body></html>`
	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	root, err := nethtml.Parse(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	styles := map[string]string{}
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		id, style := "", ""
		for _, attr := range node.Attr {
			if attr.Key == "id" {
				id = attr.Val
			}
			if attr.Key == "style" {
				style = attr.Val
			}
		}
		if id != "" {
			styles[id] = style
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	for _, id := range []string{"target", "child"} {
		style := styles[id]
		if strings.Count(strings.ToLower(style), "font-size:") != 1 || strings.Count(strings.ToLower(style), "line-height:") != 1 {
			t.Errorf("%s retained competing typography declarations: %q", id, style)
		}
		if !strings.HasSuffix(style, "font-size:12px !important;line-height:18px !important") {
			t.Errorf("%s does not end with forced typography: %q", id, style)
		}
	}
}

func TestPrepareDoesNotApplyTypographyInsideSrcdoc(t *testing.T) {
	nested := `<html><head><style>.x{font-size:10px}</style></head><body><div style="font-size:8pt">See your details</div></body></html>`
	input := `<html><head></head><body><iframe srcdoc="` + stdhtml.EscapeString(nested) + `"></iframe></body></html>`
	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, unwanted := range []string{"10.5px", "8.4pt"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("nested srcdoc received top-level typography transformation %q", unwanted)
		}
	}
	if count := strings.Count(text, "data-mail2receipt-emphasis"); count != 2 {
		t.Errorf("emphasis marker count = %d, want only the two top-level CSS selectors", count)
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

func TestPrepareStripsSourceAuthoredDataImagesFromLoadBearingLocations(t *testing.T) {
	const data = `data:image/png;base64,AQID`
	const svg = `data:image/svg+xml,%3Csvg%3E%3Cimage%20href%3D%22https%3A%2F%2Fprivate.example%2Fnested.png%22%2F%3E%3C%2Fsvg%3E`
	input := `<html><head><style>
.url { background-image:url(` + data + `); color:navy }
.set { background-image:image-set("` + data + `" 1x, url("cid:trusted") 2x); color:green }
.escaped { background-image:u\72l(d\61ta:image/png;base64,AQID); color:purple }
</style></head><body background="` + data + `">
<img src="` + data + `" srcset="` + data + ` 1x, cid:trusted 2x">
<object data="` + svg + `"></object><video poster="` + data + `"></video>
<svg><image href="` + svg + `"></image></svg>
<div style="background:url('` + data + `');background-image:-webkit-image-set('` + data + `' 1x, url(cid:trusted) 2x);color:red"></div>
</body></html>`
	pngData := tinyImage(t, "image/png")

	got, err := Prepare(message.Document{
		HTML: []byte(input),
		CID:  map[string]message.Asset{"trusted": {MediaType: "image/png", Data: pngData}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, unsafe := range []string{data, "data:image/svg+xml", "private.example", `d\61ta`, `u\72l`} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(unsafe)) {
			t.Errorf("prepared HTML retained source-authored image marker %q", unsafe)
		}
	}
	trusted := imageDataURL("image/png", pngData)
	if count := strings.Count(text, trusted); count != 1 {
		t.Errorf("trusted CID occurrence count = %d, want 1", count)
	}
	for _, safe := range []string{"color:navy", "color:green", "color:purple", "color:red"} {
		if !strings.Contains(text, safe) {
			t.Errorf("prepared HTML removed safe declaration %q", safe)
		}
	}
}

func TestPrepareValidatesReferencedCIDImageData(t *testing.T) {
	valid := map[string][]byte{
		"image/png":  tinyImage(t, "image/png"),
		"image/jpeg": tinyImage(t, "image/jpeg"),
		"image/gif":  tinyImage(t, "image/gif"),
	}
	for mediaType, data := range valid {
		t.Run(mediaType, func(t *testing.T) {
			got, err := Prepare(message.Document{
				HTML: []byte(`<img src="cid:asset">`),
				CID:  map[string]message.Asset{"asset": {MediaType: mediaType, Data: data}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(got, []byte(imageDataURL(mediaType, data))) {
				t.Fatal("validated CID was not embedded")
			}
		})
	}

	tests := []struct {
		name      string
		mediaType string
		data      []byte
	}{
		{name: "declared and decoded mismatch", mediaType: "image/jpeg", data: valid["image/png"]},
		{name: "corrupt supported image", mediaType: "image/png", data: []byte("private-corrupt-image")},
		{name: "declared SVG", mediaType: "image/svg+xml", data: []byte(`<svg><image href="https://private.example/nested"/></svg>`)},
		{name: "SVG declared as PNG", mediaType: "image/png", data: []byte(`<svg><image href="https://private.example/nested"/></svg>`)},
		{name: "unsupported decoded format", mediaType: "image/webp", data: []byte("private-webp-image")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Prepare(message.Document{
				HTML: []byte(`<img src="cid:private-cid">`),
				CID:  map[string]message.Asset{"private-cid": {MediaType: tt.mediaType, Data: tt.data}},
			})
			if err == nil {
				t.Fatal("Prepare returned nil error")
			}
			for _, sensitive := range []string{"private-cid", "private-corrupt-image", "private.example", "private-webp-image"} {
				if strings.Contains(err.Error(), sensitive) {
					t.Fatalf("error leaks sensitive input %q: %v", sensitive, err)
				}
			}
		})
	}
}

func TestPrepareDoesNotValidateUnreferencedCIDAssets(t *testing.T) {
	got, err := Prepare(message.Document{
		HTML: []byte(`<html><head></head><body><p>receipt</p></body></html>`),
		CID: map[string]message.Asset{
			"unused-corrupt": {MediaType: "image/png", Data: []byte("not an image")},
			"unused-svg":     {MediaType: "image/svg+xml", Data: []byte(`<svg/>`)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("receipt")) {
		t.Fatal("prepared HTML lost document content")
	}
}

func TestPrepareValidatesCIDReferencedBesideSourceData(t *testing.T) {
	tests := []string{
		`background-image:image-set("data:image/png;base64,AQID" 1x, url(cid:bad) 2x)`,
		`background-image:image-set("data:image/png;base64,AQID" 1x, "cid:bad" 2x)`,
	}
	for _, style := range tests {
		t.Run(style, func(t *testing.T) {
			_, err := Prepare(message.Document{
				HTML: []byte(`<div style='` + style + `'></div>`),
				CID:  map[string]message.Asset{"bad": {MediaType: "image/png", Data: []byte("corrupt")}},
			})
			if !errors.Is(err, ErrNonImageCID) {
				t.Fatalf("error = %v, want ErrNonImageCID", err)
			}
		})
	}
}

func TestPrepareRejectsImagesWithValidHeadersAndCorruptPixelData(t *testing.T) {
	for _, mediaType := range []string{"image/png", "image/jpeg", "image/gif"} {
		t.Run(mediaType, func(t *testing.T) {
			valid := tinyImage(t, mediaType)
			var truncated []byte
			for end := 1; end < len(valid); end++ {
				candidate := valid[:end]
				if _, _, configErr := image.DecodeConfig(bytes.NewReader(candidate)); configErr == nil {
					if _, _, decodeErr := image.Decode(bytes.NewReader(candidate)); decodeErr != nil {
						truncated = candidate
						break
					}
				}
			}
			if truncated == nil {
				t.Fatal("test image has no header-valid truncation")
			}
			_, err := Prepare(message.Document{
				HTML: []byte(`<img src="cid:asset">`),
				CID:  map[string]message.Asset{"asset": {MediaType: mediaType, Data: truncated}},
			})
			if !errors.Is(err, ErrNonImageCID) {
				t.Fatalf("error = %v, want ErrNonImageCID", err)
			}
		})
	}
}

func TestPrepareIgnoresResourceLikeTextOutsideCSSResourceContexts(t *testing.T) {
	const input = `<html><head><style>
.ordinary { content:"url(cid:missing) data:image/png;base64,AQID image-set('cid:missing')"; font-family:'url(cid:missing)' }
.comment { /* url(cid:missing) image-set("data:image/png;base64,AQID") */ color:navy }
</style></head><body style="content:'cid:missing url(data:image/png;base64,AQID)';/* url(cid:missing) */color:green"></body></html>`

	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{"url(cid:missing) data:image/png", "font-family", "color:navy", "cid:missing url(data:image/png", "color:green"} {
		if !strings.Contains(text, want) {
			t.Errorf("ordinary CSS text %q was removed: %s", want, got)
		}
	}
}

func TestPrepareRecognizesCommentSeparatedCSSResourceFunctions(t *testing.T) {
	pngData := tinyImage(t, "image/png")
	tests := []struct {
		name       string
		value      string
		assets     map[string]message.Asset
		wantCID    bool
		wantMarker string
	}{
		{name: "source data URL", value: `url/**/(data:image/png;base64,AQID)`, wantMarker: "data:image"},
		{name: "remote URL", value: `url/**/(https://private.example/image.png)`, wantMarker: "private.example"},
		{name: "local URL", value: `u\72l/**/(../private/image.png)`, wantMarker: "private/image.png"},
		{name: "CID URL", value: `u\72l/**/(cid:trusted)`, assets: map[string]message.Asset{"trusted": {MediaType: "image/png", Data: pngData}}, wantCID: true},
		{name: "source data image set", value: `image-set/**/("data:image/png;base64,AQID" 1x)`, wantMarker: "data:image"},
		{name: "remote image set", value: `-webkit-image-set/**/("https://private.example/image.png" 1x)`, wantMarker: "private.example"},
		{name: "CID image set", value: `image-s\65t/**/(url/**/(cid:trusted) 1x)`, assets: map[string]message.Asset{"trusted": {MediaType: "image/png", Data: pngData}}, wantCID: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Prepare(message.Document{
				HTML: []byte(`<div style='background-image:` + tt.value + `;color:teal'></div>`),
				CID:  tt.assets,
			})
			if err != nil {
				t.Fatal(err)
			}
			text := string(got)
			if tt.wantMarker != "" && strings.Contains(text, tt.wantMarker) {
				t.Errorf("prepared HTML retained resource marker %q: %s", tt.wantMarker, got)
			}
			if tt.wantCID && !strings.Contains(text, imageDataURL("image/png", pngData)) {
				t.Errorf("prepared HTML did not embed comment-separated CID: %s", got)
			}
			if !strings.Contains(text, "color:teal") {
				t.Errorf("prepared HTML removed safe sibling declaration: %s", got)
			}
		})
	}
}

func TestPrepareValidatesCommentSeparatedCSSCID(t *testing.T) {
	for _, value := range []string{
		`url/**/(cid:missing)`,
		`image-set/**/("cid:missing" 1x)`,
		`-webkit-image-set/**/(url/**/(cid:missing) 1x)`,
	} {
		t.Run(value, func(t *testing.T) {
			_, err := Prepare(message.Document{HTML: []byte(`<div style='background:` + value + `'></div>`)})
			if !errors.Is(err, ErrMissingCID) {
				t.Fatalf("error = %v, want ErrMissingCID", err)
			}
		})
	}
}

func TestPrepareScansResourcesNestedInOtherCSSFunctions(t *testing.T) {
	const input = `<style>.nested { background:cross-fade(url/**/(https://private.example/image.png), red); color:navy }</style>`
	got, err := Prepare(message.Document{HTML: []byte(input)})
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "private.example") || strings.Contains(text, "background:") {
		t.Fatalf("prepared HTML retained nested remote resource: %s", got)
	}
	if !strings.Contains(text, "color:navy") {
		t.Fatalf("prepared HTML removed safe sibling declaration: %s", got)
	}
}
