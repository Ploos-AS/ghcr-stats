package main

import (
	"html/template"
	"strings"
	"time"
)

var dashboardTemplateM32 = template.Must(template.New("dashboard-m32").Funcs(template.FuncMap{"compact": compact}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ghcr-stats</title>
<style>
:root{color-scheme:dark}*{box-sizing:border-box}body{font-family:system-ui,sans-serif;margin:0;background:#0d1117;color:#e6edf3}main{max-width:1120px;margin:auto;padding:28px}.top{display:flex;gap:16px;align-items:flex-end;justify-content:space-between;flex-wrap:wrap}.periods{display:flex;gap:6px;flex-wrap:wrap}.periods a{padding:7px 10px;border:1px solid #30363d;border-radius:7px;background:#161b22}.periods a.active{border-color:#58a6ff;background:#1f2937}.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:12px;margin-top:18px}.card,.panel,table{background:#161b22;border:1px solid #30363d;border-radius:8px}.card{padding:16px}.big{font-size:1.7rem;font-weight:700}.panel{padding:14px;margin-top:20px}.chart-wrap{position:relative;min-height:290px}canvas{width:100%;height:270px}.empty{display:none;position:absolute;inset:0;place-items:center;color:#8b949e}.table-wrap{overflow-x:auto;margin-top:20px}table{width:100%;border-collapse:collapse;min-width:620px}th,td{text-align:left;padding:11px;border-bottom:1px solid #30363d}a{color:#58a6ff;text-decoration:none}.ok{color:#3fb950}.bad{color:#f85149}.muted{color:#8b949e}.notice{padding:10px 12px;border:1px solid #9e6a03;background:#2b2300;border-radius:7px;margin-top:16px}@media(max-width:640px){main{padding:18px}.big{font-size:1.45rem}.card{padding:13px}h1{font-size:1.6rem}.hide-mobile{display:none}}
</style></head><body><main>
<div class="top"><div><h1>{{.Owner}} GHCR stats</h1><div class="muted">Historical analytics from periodic GHCR snapshots.</div></div><nav class="periods" aria-label="Period">{{range .Periods}}<a href="/?period={{.Value}}" class="{{if .Active}}active{{end}}">{{.Label}}</a>{{end}}</nav></div>
{{if .OrgDegraded}}<div class="notice">Some package data is stale or currently unavailable. Last known good snapshots are still shown.</div>{{end}}
<div class="cards"><div class="card"><div class="muted">Total</div><div class="big">{{compact .Org.Downloads}}</div></div><div class="card"><div class="muted">24h</div><div class="big">+{{compact .Org.Downloads24h}}</div></div><div class="card"><div class="muted">7d</div><div class="big">+{{compact .Org.Downloads7d}}</div></div><div class="card"><div class="muted">30d</div><div class="big">+{{compact .Org.Downloads30d}}</div></div><div class="card"><div class="muted">90d</div><div class="big">+{{compact .Org.Downloads90d}}</div></div></div>
<section class="panel"><div class="top"><strong>Organization history · {{.Period}}</strong><span class="muted" id="chart-meta">loading…</span></div><div class="chart-wrap"><canvas id="org-chart" width="1050" height="270"></canvas><div class="empty" id="org-empty">Not enough history yet.</div></div></section>
<div class="table-wrap"><table><thead><tr><th>#</th><th>Package</th><th>{{.Period}}</th><th>Total</th><th>Health</th></tr></thead><tbody>{{range .Rankings}}<tr><td>{{.Rank}}</td><td><a href="/package/{{.Package}}?period={{$.Period}}">{{.Package}}</a></td><td>+{{compact .Delta}}</td><td>{{compact .Downloads}}</td><td>{{if .CollectorUp}}{{if .Stale}}<span class="bad">stale</span>{{else}}<span class="ok">healthy</span>{{end}}{{else}}<span class="bad">collector error</span>{{end}}</td></tr>{{else}}<tr><td colspan="5" class="muted">No package snapshots have been collected yet.</td></tr>{{end}}</tbody></table></div>
<script>
const period={{printf "%q" .Period}};
function drawChart(canvas,points,empty,meta){if(!points||points.length<2){empty.style.display='grid';meta.textContent=(points&&points.length===1)?'1 snapshot':'no snapshots';return}const ctx=canvas.getContext('2d'),vals=points.map(p=>p.downloads),min=Math.min(...vals),max=Math.max(...vals),span=Math.max(1,max-min),pad=22;ctx.clearRect(0,0,canvas.width,canvas.height);ctx.strokeStyle='#30363d';ctx.beginPath();ctx.moveTo(pad,canvas.height-pad);ctx.lineTo(canvas.width-pad,canvas.height-pad);ctx.stroke();ctx.strokeStyle='#58a6ff';ctx.lineWidth=2;ctx.beginPath();points.forEach((p,i)=>{const x=pad+i*(canvas.width-2*pad)/(points.length-1),y=canvas.height-pad-(p.downloads-min)*(canvas.height-2*pad)/span;i?ctx.lineTo(x,y):ctx.moveTo(x,y)});ctx.stroke();meta.textContent=points.length+' points · '+min.toLocaleString()+' → '+max.toLocaleString()}
fetch('/api/v1/org/history?period='+encodeURIComponent(period)).then(r=>r.ok?r.json():Promise.reject(r)).then(d=>drawChart(document.getElementById('org-chart'),d.points,document.getElementById('org-empty'),document.getElementById('chart-meta'))).catch(()=>{document.getElementById('org-empty').style.display='grid';document.getElementById('chart-meta').textContent='history unavailable'});
</script></main></body></html>`))

var packageTemplateM32 = template.Must(template.New("package-m32").Funcs(template.FuncMap{"compact": compact}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Summary.Package}} · ghcr-stats</title><style>:root{color-scheme:dark}*{box-sizing:border-box}body{font-family:system-ui,sans-serif;max-width:980px;margin:auto;padding:28px;background:#0d1117;color:#e6edf3}a{color:#58a6ff;text-decoration:none}.top{display:flex;gap:12px;justify-content:space-between;align-items:center;flex-wrap:wrap}.periods{display:flex;gap:6px;flex-wrap:wrap}.periods a{padding:7px 10px;border:1px solid #30363d;border-radius:7px;background:#161b22}.periods a.active{border-color:#58a6ff}.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(130px,1fr));gap:12px}.card{padding:14px;border:1px solid #30363d;background:#161b22;border-radius:8px}.big{font-size:1.5rem;font-weight:700}.panel{margin-top:20px;padding:14px;border:1px solid #30363d;background:#161b22;border-radius:8px}.chart-wrap{position:relative;min-height:290px}canvas{width:100%;height:270px}.empty{display:none;position:absolute;inset:0;place-items:center;color:#8b949e}.ok{color:#3fb950}.bad{color:#f85149}.muted{color:#8b949e}@media(max-width:640px){body{padding:18px}.big{font-size:1.35rem}}</style></head><body><div class="top"><a href="/?period={{.Period}}">← overview</a><nav class="periods">{{range .Periods}}<a href="/package/{{$.Summary.Package}}?period={{.Value}}" class="{{if .Active}}active{{end}}">{{.Label}}</a>{{end}}</nav></div><h1>{{.Summary.Package}}</h1><div class="cards"><div class="card"><div>Total</div><div class="big">{{compact .Summary.Downloads}}</div></div><div class="card"><div>24h</div><div class="big">+{{compact .Summary.Downloads24h}}</div></div><div class="card"><div>7d</div><div class="big">+{{compact .Summary.Downloads7d}}</div></div><div class="card"><div>30d</div><div class="big">+{{compact .Summary.Downloads30d}}</div></div><div class="card"><div>90d</div><div class="big">+{{compact .Summary.Downloads90d}}</div></div></div><p>Collector: {{if .Health.Up}}<span class="ok">healthy</span>{{else}}<span class="bad">error</span>{{end}} · {{if .Health.Stale}}<span class="bad">stale</span>{{else}}fresh{{end}} · last success: {{.Health.LastSuccess}}</p><section class="panel"><div class="top"><strong>History · {{.Period}}</strong><span class="muted" id="chart-meta">loading…</span></div><div class="chart-wrap"><canvas id="chart" width="930" height="270"></canvas><div class="empty" id="empty">Not enough history yet.</div></div></section><script>const period={{printf "%q" .Period}};fetch('/api/v1/packages/{{.Summary.Package}}/history?period='+encodeURIComponent(period)).then(r=>r.ok?r.json():Promise.reject(r)).then(d=>{const c=document.getElementById('chart'),e=document.getElementById('empty'),m=document.getElementById('chart-meta'),p=d.points||[];if(p.length<2){e.style.display='grid';m.textContent=p.length?p.length+' snapshot':'no snapshots';return}const x=c.getContext('2d'),v=p.map(q=>q.downloads),min=Math.min(...v),max=Math.max(...v),span=Math.max(1,max-min),pad=22;x.strokeStyle='#58a6ff';x.lineWidth=2;x.beginPath();p.forEach((q,i)=>{const px=pad+i*(c.width-2*pad)/(p.length-1),py=c.height-pad-(q.downloads-min)*(c.height-2*pad)/span;i?x.lineTo(px,py):x.moveTo(px,py)});x.stroke();m.textContent=p.length+' points · '+min.toLocaleString()+' → '+max.toLocaleString()}).catch(()=>{document.getElementById('empty').style.display='grid';document.getElementById('chart-meta').textContent='history unavailable'});</script></body></html>`))

type dashboardPeriod struct {
	Value  string
	Label  string
	Active bool
}

func normalizeDashboardPeriod(raw string) string {
	_, p, err := parsePeriod(raw)
	if err != nil {
		return "30d"
	}
	return p
}

func dashboardPeriods(active string) []dashboardPeriod {
	values := []dashboardPeriod{{Value: "24h", Label: "24h"}, {Value: "7d", Label: "7d"}, {Value: "30d", Label: "30d"}, {Value: "90d", Label: "90d"}, {Value: "all", Label: "All"}}
	for i := range values {
		values[i].Active = values[i].Value == active
	}
	return values
}

func orgDashboardDegraded(a *App) bool {
	now := time.Now().UTC()
	for _, pkg := range a.packageNames() {
		h := a.collectorHealth(pkg, now)
		if !h.Up || h.Stale {
			return true
		}
	}
	return false
}

func validPackagePath(pkg string) bool {
	return pkg != "" && !strings.Contains(pkg, "/")
}
