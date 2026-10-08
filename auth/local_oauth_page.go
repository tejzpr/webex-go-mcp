package auth

import (
	"html/template"
	"net/http"
)

// localAuthPageTmpl is the small page shown in the browser at the end of the
// STDIO-mode Webex sign-in. html/template escapes all values.
var localAuthPageTmpl = template.Must(template.New("local-auth").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} · Webex MCP</title>
<style>
  :root { --bg:#f5f6f8; --card:#fff; --fg:#1c1e21; --muted:#5b6170; --ok:#1d8a4e; --err:#c4314b; --border:#e3e5ea; }
  @media (prefers-color-scheme: dark) {
    :root { --bg:#14161a; --card:#1e2127; --fg:#eceef2; --muted:#a3a9b6; --ok:#4cc38a; --err:#ff6b81; --border:#2c3038; }
  }
  * { box-sizing: border-box; }
  body { margin:0; min-height:100vh; display:flex; align-items:center; justify-content:center;
         background:var(--bg); color:var(--fg); font:16px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif; padding:16px; }
  .card { background:var(--card); border:1px solid var(--border); border-radius:14px; padding:32px 28px; max-width:440px; width:100%; text-align:center; }
  .icon { width:56px; height:56px; border-radius:50%; margin:0 auto 16px; display:flex; align-items:center; justify-content:center; font-size:28px; color:#fff; }
  .ok { background:var(--ok); } .err { background:var(--err); }
  h1 { font-size:20px; margin:0 0 8px; }
  p { margin:0; color:var(--muted); }
  .brand { margin-top:24px; font-size:13px; color:var(--muted); }
</style>
</head>
<body>
  <main class="card">
    <div class="icon {{if .OK}}ok{{else}}err{{end}}" aria-hidden="true">{{if .OK}}&#10003;{{else}}!{{end}}</div>
    <h1>{{.Title}}</h1>
    <p>{{.Message}}</p>
    <div class="brand">Webex MCP Server</div>
  </main>
</body>
</html>`))

func renderLocalAuthPage(w http.ResponseWriter, status int, title, message string, ok bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.WriteHeader(status)
	_ = localAuthPageTmpl.Execute(w, struct {
		Title   string
		Message string
		OK      bool
	}{title, message, ok})
}
