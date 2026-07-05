package server

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"strings"
)

//go:embed bridge.js
var bridgeJSSource string

// wrapperTmpl renders the sandboxed iframe that hosts untrusted user HTML.
// Using html/template ensures the srcdoc attribute value is correctly escaped
// (including </script> sequences inside inline scripts).
var wrapperTmpl = template.Must(template.New("wrapper").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>aidocs render</title><style>html,body{margin:0;height:100%;overflow:hidden}#aidocs-doc{border:0;width:100%;height:100vh}</style><script>try{var u=new URL(location.href);if(u.searchParams.has("token")){u.searchParams.delete("token");history.replaceState(null,"",u.pathname+(u.search||"")+u.hash)}}catch(e){}</script></head><body><iframe id="aidocs-doc" sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox" srcdoc="{{.SrcDoc}}"></iframe><script>window.__AIDOCS_APP_ORIGIN__={{.AppOriginJS}};({{.BridgeJS}})();</script></body></html>`))

type wrapperData struct {
	// SrcDoc is the raw user HTML. html/template escapes it for the
	// double-quoted srcdoc attribute context (including </script> inside
	// inline scripts), so no manual escaping is needed.
	SrcDoc      string
	AppOriginJS template.JS
	BridgeJS    template.JS
}

const bridgePrelude = `<script id="aidocs-bootstrap">(function(){if(window.aidocs&&window.aidocs.kv)return;var q=[];var handlers={kvchange:[]};function enqueue(op,args){return new Promise(function(resolve,reject){q.push({op:op,args:Array.prototype.slice.call(args),resolve:resolve,reject:reject});});}function off(event,handler){if(event!=="kvchange")return;handlers.kvchange=handlers.kvchange.filter(function(h){return h!==handler;});}function on(event,handler){if(event==="kvchange"&&typeof handler==="function"){handlers.kvchange.push(handler);return function(){off(event,handler);};}}window.__aidocsKVQueue=q;window.__aidocsKVHandlers=handlers;window.aidocs=Object.assign(window.aidocs||{},{kv:{get:function(){return enqueue("get",arguments);},all:function(){return enqueue("all",arguments);},set:function(){return enqueue("set",arguments);},delete:function(){return enqueue("delete",arguments);},patch:function(){return enqueue("patch",arguments);}},on:on,off:off});})();</script>`

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

// renderWrapperHTML returns the full wrapper page that embeds userHTML inside
// a sandboxed iframe. appOrigin is used for the Content-Security-Policy and
// for the bridge JS postMessage target origin.
func renderWrapperHTML(userHTML []byte, appOrigin string) []byte {
	originJSON, _ := json.Marshal(appOrigin)
	data := wrapperData{
		SrcDoc:      userHTMLWithBridgePrelude(userHTML),
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
