package server

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed bridge.js
var bridgeJSSource string

// wrapperTmpl renders the sandboxed iframe that hosts untrusted user HTML.
// Using html/template ensures the srcdoc attribute value is correctly escaped
// (including </script> sequences inside inline scripts).
var wrapperTmpl = template.Must(template.New("wrapper").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>aidocs render</title><style>html,body{margin:0;height:100%;overflow:hidden}#aidocs-doc{border:0;width:100%;height:100vh}</style></head><body><iframe id="aidocs-doc" sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox" srcdoc="{{.SrcDoc}}"></iframe><script>window.__AIDOCS_APP_ORIGIN__={{.AppOriginJS}};({{.BridgeJS}})();</script></body></html>`))

type wrapperData struct {
	// SrcDoc is the raw user HTML. html/template escapes it for the
	// double-quoted srcdoc attribute context (including </script> inside
	// inline scripts), so no manual escaping is needed.
	SrcDoc      string
	AppOriginJS template.JS
	BridgeJS    template.JS
}

// renderWrapperHTML returns the full wrapper page that embeds userHTML inside
// a sandboxed iframe. appOrigin is used for the Content-Security-Policy and
// for the bridge JS postMessage target origin.
func renderWrapperHTML(userHTML []byte, appOrigin string) []byte {
	originJSON, _ := json.Marshal(appOrigin)
	data := wrapperData{
		SrcDoc:      string(userHTML),
		AppOriginJS: template.JS(originJSON),
		BridgeJS:    template.JS(bridgeJSSource),
	}
	var sb strings.Builder
	if err := wrapperTmpl.Execute(&sb, data); err != nil {
		// Fallback: should never happen with a valid template.
		return []byte("<!doctype html><html><body>render error</body></html>")
	}
	return []byte(sb.String())
}

// renderSecurityHeaders sets the sandbox CSP + nosniff headers shared by the
// native (renderVersion) and Drive (driveRender) render paths, and returns the
// resolved app origin (defaulting to 'self' when unset) for use as the
// wrapper's frame-ancestors / postMessage target. Keeping a single source for
// the CSP avoids the two paths silently diverging.
func renderSecurityHeaders(c *gin.Context, appOrigin string) string {
	if appOrigin == "" {
		appOrigin = "'self'"
	}
	c.Header("Content-Security-Policy", "default-src 'none'; img-src * data: blob:; media-src * data: blob:; font-src * data: blob:; style-src * 'unsafe-inline'; script-src * 'unsafe-inline' 'unsafe-eval'; connect-src *; frame-src *; worker-src * blob:; object-src 'none'; frame-ancestors "+appOrigin)
	c.Header("X-Content-Type-Options", "nosniff")
	return appOrigin
}
