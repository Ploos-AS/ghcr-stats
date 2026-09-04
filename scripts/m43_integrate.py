from pathlib import Path

main = Path("main.go")
s = main.read_text()
old = 'mux.HandleFunc("/api/v1/org/history", a.handleOrgHistory)\n'
new = old + '\tmux.HandleFunc("/api/v1/org/export", a.handleOrgExport)\n'
if 'mux.HandleFunc("/api/v1/org/export", a.handleOrgExport)' not in s:
    if old not in s:
        raise SystemExit("org route anchor not found")
    s = s.replace(old, new, 1)
main.write_text(s)

m3 = Path("m3.go")
s = m3.read_text()
old = '''\tif len(parts) == 2 && parts[1] == "history" {
\t\ta.handlePackageHistory(w, r, parts[0])
\t\treturn
\t}
'''
new = old + '''\tif len(parts) == 2 && parts[1] == "export" {
\t\ta.handlePackageExport(w, r, parts[0])
\t\treturn
\t}
'''
if 'parts[1] == "export"' not in s:
    if old not in s:
        raise SystemExit("package route anchor not found")
    s = s.replace(old, new, 1)
m3.write_text(s)

readme = Path("README.md")
s = readme.read_text()
marker = "## API\n"
addition = """\n### History export (M4.3)\n\nHistory can be exported as JSON or CSV with the same period semantics as the analytics API (`24h`, `7d`, `30d`, `90d`, `all`):\n\n```text\nGET /api/v1/packages/{package}/export?format=json&period=30d\nGET /api/v1/packages/{package}/export?format=csv&period=30d\nGET /api/v1/org/export?format=json&period=30d\nGET /api/v1/org/export?format=csv&period=30d\n```\n\nCSV responses use the columns `owner,package,period,timestamp,downloads,delta` and are returned as downloadable attachments. Export endpoints are read-only (`GET` only).\n\n"""
if "### History export (M4.3)" not in s:
    if marker in s:
        pos = s.index(marker) + len(marker)
        s = s[:pos] + addition + s[pos:]
    else:
        s += "\n" + addition
readme.write_text(s)
