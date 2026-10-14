#!/usr/bin/env python3
"""Regenerate bundled help and the curl fixture from the app's documentation."""
import html
import json
import re
from pathlib import Path

root = Path(__file__).resolve().parent
help_dir = root / "deployments/help"


def inline(text):
    text = html.escape(text)
    text = re.sub(r"`([^`]+)`", r"<code>\1</code>", text)
    return re.sub(r"\*\*([^*]+)\*\*", r"<strong>\1</strong>", text)


parts, paragraph, code, table = [], [], None, False


def flush():
    if paragraph:
        parts.append("<p>" + inline(" ".join(paragraph)) + "</p>")
        paragraph.clear()


for line in (root / "docs/API.md").read_text().splitlines():
    if line.startswith("```"):
        flush()
        if code is None:
            code = []
        else:
            parts.append("<pre><code>" + html.escape("\n".join(code)) + "</code></pre>")
            code = None
        continue
    if code is not None:
        code.append(line)
        continue
    if table and not line.startswith("|"):
        parts.append("</tbody></table></div>")
        table = False
    if line.startswith("|"):
        flush()
        if re.fullmatch(r"[| :\-]+", line):
            continue
        tag = "td" if table else "th"
        if not table:
            parts.append('<div class="table"><table><tbody>')
            table = True
        cells = line.strip("|").split("|")
        parts.append("<tr>" + "".join(f"<{tag}>{inline(c.strip())}</{tag}>" for c in cells) + "</tr>")
    elif line.startswith("#"):
        flush()
        depth = len(line) - len(line.lstrip("#"))
        parts.append(f"<h{depth}>{inline(line[depth:].strip())}</h{depth}>")
    elif not line.strip():
        flush()
    else:
        paragraph.append(line)
flush()

page = '''<!DOCTYPE html><html lang="ro"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>WiseMED • HTML Print Formatter • Ajutor</title>
<style>
body{font:16px/1.65 system-ui,sans-serif;color:#173448;background:#f1f5f9;margin:0}
main{max-width:1000px;margin:auto;padding:32px;background:white}
h1,h2{line-height:1.25;color:#075985}h2{margin-top:2.3em}
code,pre,textarea{font:14px/1.6 ui-monospace,monospace}pre{background:#edf4f8;padding:18px;overflow:auto;border-radius:8px}
td,th{border-bottom:1px solid #d6e3ec;text-align:left;padding:10px;vertical-align:top}table{border-collapse:collapse;width:100%}.table{overflow:auto}
a{color:#075985}nav{display:flex;gap:20px;flex-wrap:wrap}textarea{width:100%;box-sizing:border-box;min-height:160px;padding:12px}
button{font:inherit;background:#075985;color:white;border:0;padding:9px 16px;border-radius:6px;margin:12px 12px 12px 0;cursor:pointer}button:disabled{opacity:.5}
details{padding:18px;border:1px solid #d6e3ec;border-radius:8px;margin-top:24px}#status{white-space:pre-wrap}
</style></head><body><main><nav><a href="/">Administrare WiseMED</a><a href="/health">Health</a><a href="example.html" download>Exemplu jQuery</a><a href="request.json" download>request.json pentru curl</a></nav>
<details><summary>Test local HTML și PDF</summary><p>Completați HTML sau Link HTML. Randează așteaptă window.printReady; PDF direct nu cere acest semnal.</p>
<label>HTML<textarea id="request" aria-label="HTML" spellcheck="false"></textarea></label>
<label>Link HTML<input id="html-link" type="url" placeholder="https://exemplu.ro/raport.html"></label>
<label>Timeout (ms)<input id="timeout" type="number" min="1" max="600000" value="120000"></label>
<button id="example">Încarcă exemplul</button><button id="render">Randează → PDF</button><button id="pdf">HTML → PDF direct</button>
<p id="status" role="status"></p><textarea id="result" readonly aria-label="DOM rezultat"></textarea><a id="download" hidden download="rendered.pdf">Descarcă PDF</a></details>
'''
page += "\n".join(parts)
page += '''<script>
let downloadURL;
const input=document.getElementById('request'),output=document.getElementById('result'),status=document.getElementById('status'),button=document.getElementById('render'),direct=document.getElementById('pdf'),download=document.getElementById('download'),link=document.getElementById('html-link'),timeout=document.getElementById('timeout');
document.getElementById('example').onclick=async()=>{try{const response=await fetch('example.html');if(!response.ok)throw Error('Exemplu indisponibil');input.value=(await response.text()).replace('<head>','<head><base href="'+location.origin+'/help/">');link.value=''}catch(error){status.textContent=error.message}};
async function renderPDF(waitForPrintReady){
  button.disabled=true;direct.disabled=true;download.hidden=true;output.value='';status.textContent='Generare PDF în curs…';
  if(downloadURL){URL.revokeObjectURL(downloadURL);downloadURL=null;download.removeAttribute('href')}
  try{
    const html=input.value,html_link=link.value.trim(),timeout_ms=Number(timeout.value);
    if(!!html.trim()===!!html_link)throw Error('Completați exact una dintre surse: HTML sau Link HTML.');
    if(!Number.isInteger(timeout_ms)||timeout_ms<1||timeout_ms>600000)throw Error('Timeout: număr întreg între 1 și 600000 ms.');
    const body=JSON.stringify({...html_link?{html_link}:{html},timeout_ms,wait_for_print_ready:waitForPrintReady});
    const response=await fetch('/api/html-print-formatter/pdf',{method:'POST',headers:{'Content-Type':'application/json'},body});
    const result=await response.json();if(!response.ok)throw Error(result.error?.message||'Generarea PDF a eșuat');
    output.value=result.html;const bytes=Uint8Array.from(atob(result.pdf_base64),c=>c.charCodeAt(0));
    downloadURL=URL.createObjectURL(new Blob([bytes],{type:'application/pdf'}));download.href=downloadURL;download.hidden=false;
    status.textContent='PDF generat • request_id: '+result.request_id;
  }catch(error){status.textContent=error.message}finally{button.disabled=false;direct.disabled=false}
}
button.onclick=()=>renderPDF(true);direct.onclick=()=>renderPDF(false);
window.addEventListener('pagehide',()=>{if(downloadURL)URL.revokeObjectURL(downloadURL)});
</script></main></body></html>'''
(help_dir / "index.html").write_text(page)
(help_dir / "request.json").write_text(json.dumps({
    "html": (help_dir / "example.html").read_text(),
    "base_url": "http://127.0.0.1:19113/help/",
    "timeout_ms": 120000,
    "viewport_width": 1280,
    "viewport_height": 900,
}, ensure_ascii=False, indent=2) + "\n")
