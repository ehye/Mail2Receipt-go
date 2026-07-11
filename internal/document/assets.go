package document

import (
	"embed"
	"encoding/base64"
	"net/url"
	"path"
	"strings"
)

const (
	lockupFilename = "google-play-crm-lockup-ic-h-transparent-w688px-h140px-2x.png"
	logoFilename   = "google-play-crm-logo-transparent-w192px-h192px-2x.png"
)

//go:embed assets/*.png
var logoFiles embed.FS

var embeddedLogos = map[string]string{
	lockupFilename: pngDataURL("assets/" + lockupFilename),
	logoFilename:   pngDataURL("assets/" + logoFilename),
}

func pngDataURL(name string) string {
	data, err := logoFiles.ReadFile(name)
	if err != nil {
		panic("embedded receipt logo missing")
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

func embeddedLogo(source string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil {
		return "", false
	}
	dataURL, ok := embeddedLogos[path.Base(parsed.Path)]
	return dataURL, ok
}
