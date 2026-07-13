package server

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"strings"
)

// ErrUnsafeRenderOrigin marks the one render-origin validation failure that
// the explicit local-development escape hatch may override. Invalid origins
// must remain fatal even when that escape hatch is set.
var ErrUnsafeRenderOrigin = errors.New("app and render origins are not isolated by hostname")

// ValidateRenderOrigin refuses an app/render origin pair that shares a
// hostname. Uploaded HTML is untrusted; scheme and port do not isolate
// host-scoped cookies, so rendering it on the app host lets a
// malicious document script against the app itself (session cookies, API
// calls). Startup code may downgrade this error to a loud warning when
// AIDOCS_ALLOW_UNSAFE_RENDER_ORIGIN=true opts into the risk (dev/demo only).
func ValidateRenderOrigin(appOrigin, renderOrigin string) error {
	appCanonical, appOK := canonicalHTTPOrigin(appOrigin)
	if !appOK {
		return fmt.Errorf("AIDOCS_APP_ORIGIN (%q) is not a valid HTTP origin", appOrigin)
	}
	renderCanonical, renderOK := canonicalHTTPOrigin(renderOrigin)
	if !renderOK {
		return fmt.Errorf("AIDOCS_RENDER_ORIGIN (%q) is not a valid HTTP origin", renderOrigin)
	}
	appURL, _ := url.Parse(appCanonical)
	renderURL, _ := url.Parse(renderCanonical)
	appHostname := strings.TrimSuffix(appURL.Hostname(), ".")
	renderHostname := strings.TrimSuffix(renderURL.Hostname(), ".")
	if appHostname != renderHostname {
		return nil
	}
	return fmt.Errorf("%w: AIDOCS_RENDER_ORIGIN (%q) must use a different hostname from AIDOCS_APP_ORIGIN: the server routes renders by Host, so a scheme or port difference alone cannot isolate untrusted document HTML. Serve renders from a separate hostname, or set AIDOCS_ALLOW_UNSAFE_RENDER_ORIGIN=true to accept this risk in non-production setups", ErrUnsafeRenderOrigin, renderOrigin)
}

// bridgeFS embeds a tracked placeholder so plain Go builds remain valid before
// the frontend toolchain generates bridge_dist/bridge.js.
//
//go:embed all:bridge_dist
var bridgeFS embed.FS

var bridgeJSSource = func() string {
	source, err := bridgeFS.ReadFile("bridge_dist/bridge.js")
	if err != nil {
		return ""
	}
	return string(source)
}()

// wrapperTmpl renders the sandboxed iframe that hosts untrusted user HTML.
// Using html/template ensures the srcdoc attribute value is correctly escaped
// (including </script> sequences inside inline scripts).
var wrapperTmpl = template.Must(template.New("wrapper").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>aidocs render</title><style>html,body{margin:0;height:100%;overflow:hidden}#aidocs-doc{border:0;width:100%;height:100vh}</style><script>try{var u=new URL(location.href);if(u.searchParams.has("token")){u.searchParams.delete("token");history.replaceState(null,"",u.pathname+(u.search||"")+u.hash)}}catch(e){}</script></head><body><iframe id="aidocs-doc" sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox" srcdoc="{{.SrcDoc}}"></iframe><script>window.__AIDOCS_APP_ORIGIN__={{.AppOriginJS}};{{.BridgeJS}}</script></body></html>`))

type wrapperData struct {
	// SrcDoc is the raw user HTML. html/template escapes it for the
	// double-quoted srcdoc attribute context (including </script> inside
	// inline scripts), so no manual escaping is needed.
	SrcDoc      string
	AppOriginJS template.JS
	BridgeJS    template.JS
}

const bridgePrelude = `<script id="aidocs-bootstrap">(function(){if(window.aidocs&&window.aidocs.kv)return;var q=[];var handlers={kvchange:[]};function enqueue(op,args){return new Promise(function(resolve,reject){q.push({op:op,args:Array.prototype.slice.call(args),resolve:resolve,reject:reject});});}function off(event,handler){if(event!=="kvchange")return;handlers.kvchange=handlers.kvchange.filter(function(h){return h!==handler;});}function on(event,handler){if(event==="kvchange"&&typeof handler==="function"){handlers.kvchange.push(handler);return function(){off(event,handler);};}}window.__aidocsKVQueue=q;window.__aidocsKVHandlers=handlers;window.aidocs=Object.assign(window.aidocs||{},{kv:{get:function(){return enqueue("get",arguments);},all:function(){return enqueue("all",arguments);},set:function(){return enqueue("set",arguments);},delete:function(){return enqueue("delete",arguments);},patch:function(){return enqueue("patch",arguments);},undo:function(){return enqueue("undo",arguments);},redo:function(){return enqueue("redo",arguments);}},on:on,off:off});})();</script>`

func userHTMLWithBridgePrelude(userHTML []byte) string {
	s := string(userHTML)
	lower := strings.ToLower(s)
	for _, tag := range []string{"head", "html"} {
		if end := findStartTagEnd(lower, tag); end >= 0 {
			return s[:end+1] + bridgePrelude + s[end+1:]
		}
	}
	if idx := strings.Index(lower, "<!doctype"); idx >= 0 {
		if end := strings.IndexByte(lower[idx:], '>'); end >= 0 {
			insertAt := idx + end + 1
			return s[:insertAt] + bridgePrelude + s[insertAt:]
		}
	}
	return bridgePrelude + s
}

func findStartTagEnd(lowerHTML, tag string) int {
	needle := "<" + tag
	for offset := 0; offset < len(lowerHTML); {
		idx := strings.Index(lowerHTML[offset:], needle)
		if idx < 0 {
			return -1
		}
		idx += offset
		next := idx + len(needle)
		if next < len(lowerHTML) {
			switch lowerHTML[next] {
			case '>', '/', ' ', '\t', '\n', '\r', '\f':
			default:
				offset = next
				continue
			}
		}
		end := strings.IndexByte(lowerHTML[next:], '>')
		if end < 0 {
			return -1
		}
		return next + end
	}
	return -1
}

func appOriginScriptValue(appOrigin string) template.JS {
	originJSON, err := json.Marshal(appOrigin)
	if err != nil {
		panic(err) // encoding a string as JSON cannot fail
	}
	//nolint:gosec // G203: json.Marshal produces a quoted JS string literal with script-breaking characters escaped.
	return template.JS(originJSON)
}

func embeddedBridgeScript() template.JS {
	//nolint:gosec // G203: bridgeJSSource is a build-time embedded repository asset, never user-controlled input.
	return template.JS(bridgeJSSource)
}

// renderWrapperHTML returns the full wrapper page that embeds userHTML inside
// a sandboxed iframe. appOrigin is used for the Content-Security-Policy and
// for the bridge JS postMessage target origin.
func renderWrapperHTML(userHTML []byte, appOrigin string) []byte {
	data := wrapperData{
		SrcDoc:      userHTMLWithBridgePrelude(userHTML),
		AppOriginJS: appOriginScriptValue(appOrigin),
		BridgeJS:    embeddedBridgeScript(),
	}
	var sb strings.Builder
	if err := wrapperTmpl.Execute(&sb, data); err != nil {
		// Fallback: should never happen with a valid template.
		return []byte("<!doctype html><html><body>render error</body></html>")
	}
	return []byte(sb.String())
}
