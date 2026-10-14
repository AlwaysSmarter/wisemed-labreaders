package htmlprintformatter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"wisemed-labreaders/readersv3/shared/apibridge"
)

func TestBrowserIntegration(t *testing.T) {
	s, _ := ReadSettings(nil)
	if s.ChromePath == "" {
		for _, p := range []string{"chromium", "chromium-browser", "google-chrome", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"} {
			if path, err := exec.LookPath(p); err == nil {
				s.ChromePath = path
				break
			}
		}
	}
	if s.ChromePath == "" {
		if os.Getenv("REQUIRE_CHROME_TESTS") == "1" {
			t.Fatal("Chromium required")
		}
		t.Skip("install Chromium or set CHROME_PATH; REQUIRE_CHROME_TESTS=1 makes this mandatory")
	}
	var logs bytes.Buffer
	var logMu sync.Mutex
	t.Cleanup(func() {
		if t.Failed() {
			logMu.Lock()
			defer logMu.Unlock()
			t.Log(logs.String())
		}
	})
	r, err := NewRenderer(context.Background(), s, func(f string, a ...interface{}) { logMu.Lock(); defer logMu.Unlock(); fmt.Fprintf(&logs, f+"\n", a...) })
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	jquery, err := os.ReadFile("../../apps/html-print-formatter/deployments/help/jquery-3.7.1.min.js")
	if err != nil {
		t.Fatal(err)
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/jquery.js":
			w.Header().Set("Content-Type", "application/javascript")
			w.Write(jquery)
		case "/layout.css":
			w.Header().Set("Content-Type", "text/css")
			io.WriteString(w, `@media screen { #report { width: 321px; } }`)
		case "/static":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, `<html><body>STATIC URL PDF</body></html>`)
		case "/linked":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, `<!DOCTYPE html><html><head><script src="jquery.js"></script></head><body><script>$(function(){document.body.id='linked-ready';window.printReady=true})</script></body></html>`)
		case "/redirect":
			http.Redirect(w, req, "/linked", 302)
		default:
			http.NotFound(w, req)
		}
	}))
	defer fixture.Close()
	render := func(req Request) (string, error) {
		if req.TimeoutMS == 0 {
			req.TimeoutMS = 5000
		}
		return r.Render(context.Background(), req, "integration-test")
	}
	t.Run("jquery-ready-layout-and-doctype", func(t *testing.T) {
		source := `<!DOCTYPE html><html><head><link rel="stylesheet" href="layout.css"><script src="jquery.js"></script></head><body><section id="report"></section><p id="move">WiseMED</p><script>$(document).ready(function(){ $('#move').appendTo('#report').css('color','red'); $('#report').attr('data-width', $('#report').width()).attr('data-viewport',window.innerWidth); $('<b>generated</b>').appendTo('#report'); console.log('ready-message'); window.printReady=true; });</script></body></html>`
		out, err := render(Request{HTML: source, BaseURL: fixture.URL + "/", ViewportWidth: 1111})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"<!DOCTYPE html>", `data-width="321"`, `data-viewport="1111"`, `<p id="move" style="color: red;">WiseMED</p><b>generated</b></section>`, `<script src="jquery.js">`} {
			if !strings.Contains(out, want) {
				t.Errorf("missing %s in %s", want, out)
			}
		}
		logMu.Lock()
		defer logMu.Unlock()
		if !strings.Contains(logs.String(), "request_id=integration-test console=log") {
			t.Fatal("console not correlated")
		}
	})
	t.Run("existing-base", func(t *testing.T) {
		out, err := render(Request{HTML: `<!DOCTYPE html><html><head><base href="` + fixture.URL + `/"><script src="jquery.js"></script></head><body><script>$(function(){window.printReady=true})</script></body></html>`, BaseURL: "https://unused.invalid/"})
		if err != nil || strings.Contains(out, "unused.invalid") || strings.Count(out, "<base ") != 1 {
			t.Fatalf("%v %s", err, out)
		}
	})
	t.Run("link-redirect-and-base", func(t *testing.T) {
		for _, base := range []string{"", fixture.URL + "/"} {
			out, err := render(Request{HTMLLink: fixture.URL + "/redirect", BaseURL: base})
			if err != nil || !strings.Contains(out, `id="linked-ready"`) {
				t.Fatalf("%v %s", err, out)
			}
		}
	})
	t.Run("source-http-error", func(t *testing.T) {
		_, err := render(Request{HTMLLink: fixture.URL + "/missing"})
		requireCode(t, err, "source_load_failed")
	})
	t.Run("mandatory-exact-ready", func(t *testing.T) {
		for _, js := range []string{"", `window.printReady='true'`} {
			_, err := render(Request{HTML: `<html><body><script>` + js + `</script></body></html>`, TimeoutMS: 400})
			requireCode(t, err, "render_timeout")
		}
	})
	t.Run("page-error-priority", func(t *testing.T) {
		_, err := render(Request{HTML: `<html><script>window.printReady=true;window.printError='pagination failed'</script></html>`})
		requireCode(t, err, "page_error")
		if !strings.Contains(err.Error(), "pagination failed") {
			t.Fatal(err)
		}
	})
	t.Run("javascript-errors-logged", func(t *testing.T) {
		_, err := render(Request{HTML: `<html><script>throw new Error('uncaught-fixture')</script><script>window.printReady=true</script></html>`})
		if err != nil {
			t.Fatal(err)
		}
		logMu.Lock()
		defer logMu.Unlock()
		if !strings.Contains(logs.String(), "javascript_error=") || !strings.Contains(logs.String(), "uncaught-fixture") {
			t.Fatal("missing JS error")
		}
	})
	t.Run("storage-isolation-and-concurrency", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, err := render(Request{HTML: `<html><body><script>document.body.id=localStorage.getItem('leak')||'clean';localStorage.setItem('leak','dirty');document.cookie='leak=yes';window.printReady=true</script></body></html>`})
				if err != nil || !strings.Contains(out, `id="clean"`) {
					t.Errorf("%v %s", err, out)
				}
			}()
		}
		wg.Wait()
	})
	t.Run("images-settle-cancel-and-late-error", func(t *testing.T) {
		entered := make(chan struct{}, 10)
		slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { entered <- struct{}{}; <-req.Context().Done() }))
		defer slow.Close()
		source := `<html><body><img src="` + slow.URL + `/image"><script>window.printReady=true;</script></body></html>`
		_, err := render(Request{HTML: source, TimeoutMS: 500})
		requireCode(t, err, "render_timeout")
		_, err = render(Request{HTML: strings.Replace(source, "window.printReady=true;", "window.printReady=true;setTimeout(()=>window.printError='late failure',80);", 1)})
		requireCode(t, err, "page_error")
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { _, e := r.Render(ctx, Request{HTML: source}, "cancel-test"); done <- e }()
		// Wait for this request's image, discarding notifications from the first two.
		for i := 0; i < 3; i++ {
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("image not requested")
			}
		}
		cancel()
		select {
		case e := <-done:
			requireCode(t, e, "render_canceled")
		case <-time.After(3 * time.Second):
			t.Fatal("cancel did not close tab")
		}
	})
	t.Run("http-and-wss-parity", func(t *testing.T) {
		m := &Module{settings: s, renderer: r}
		mux := http.NewServeMux()
		mux.HandleFunc("/render", m.handleRender)
		mux.HandleFunc("/api/html-print-formatter/render", m.handleRender)
		body := json.RawMessage(`{"html":"<!DOCTYPE html><html><body><script>document.body.id='done';window.printReady=true</script></body></html>"}`)
		req := httptest.NewRequest("POST", "/render", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 200 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		out, e := apibridge.Invoke(context.Background(), mux, apibridge.Principal{Subject: "test", TenantID: "wisemed"}, apibridge.Request{Method: "POST", Path: "/api/html-print-formatter/render", Body: body})
		if e != nil || out.Status != 200 {
			t.Fatalf("%v %+v", e, out)
		}
		if out.Body.(map[string]interface{})["html"] != w.Body.String() {
			t.Fatal("HTTP/WS returned different DOM")
		}
	})
	t.Run("queued-request-deadline", func(t *testing.T) {
		for i := 0; i < cap(r.sem); i++ {
			r.sem <- struct{}{}
		}
		defer func() {
			for i := 0; i < cap(r.sem); i++ {
				<-r.sem
			}
		}()
		_, err := render(Request{HTML: `<html><script>window.printReady=true</script></html>`, TimeoutMS: 50})
		requireCode(t, err, "render_timeout")
		if len(r.sem) != cap(r.sem) {
			t.Fatal("queued timeout released another request's slot")
		}
	})
	t.Run("settled-assets-and-two-frames", func(t *testing.T) {
		assets := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			select {
			case <-req.Context().Done():
				return
			case <-time.After(80 * time.Millisecond):
			}
			w.Header().Set("Access-Control-Allow-Origin", "*")
			http.NotFound(w, req)
		}))
		defer assets.Close()
		out, err := render(Request{HTML: `<html><body><img loading="lazy" src="` + assets.URL + `/img" onerror="document.body.dataset.image='settled'"><script>
const font=new FontFace('Fixture','url(` + assets.URL + `/font)');document.fonts.add(font);font.load().catch(()=>{document.body.dataset.font='settled'});
requestAnimationFrame(()=>requestAnimationFrame(()=>document.body.dataset.frames='two'));window.printReady=true;
</script></body></html>`})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`data-image="settled"`, `data-font="settled"`, `data-frames="two"`, `loading="lazy"`} {
			if !strings.Contains(out, want) {
				t.Fatalf("missing %s in %s", want, out)
			}
		}
	})
	t.Run("deadline-during-context-creation", func(t *testing.T) {
		for _, ms := range []int{1, 2, 4, 8, 16} {
			_, err := render(Request{HTML: `<html><body>pending</body></html>`, TimeoutMS: ms})
			requireCode(t, err, "render_timeout")
		}
	})
	t.Run("pdf-live-dom-and-css-pages", func(t *testing.T) {
		source := `<!DOCTYPE html><html><head><style>@page { size: 100mm 150mm; margin: 10mm; } body {font:16px sans-serif} .second {break-before:page} .print-only {display:none} @media print {.screen-only {display:none} .print-only {display:block}}</style></head><body><h1 id="title">NOT PROCESSED</h1><section class="second">SECOND PAGE</section><p class="screen-only">SCREEN ONLY CONTENT</p><p class="print-only">PRINT ONLY CONTENT</p><script>document.body.dataset.initialMedia=matchMedia("screen").matches?"screen":"other"; matchMedia("print").addEventListener("change",e=>document.body.dataset.printMedia=String(e.matches));document.getElementById('title').textContent='PROCESSED DOM'; document.body.dataset.runs=String((Number(document.body.dataset.runs)||0)+1); window.printReady=true;</script></body></html>`
		html, pdf, err := r.RenderPDF(context.Background(), Request{HTML: source, TimeoutMS: 5000, WaitForPrintReady: true}, "pdf-live")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(html, `data-runs="1"`) || !strings.Contains(html, `data-initial-media="screen"`) || !strings.Contains(html, `data-print-media="true"`) || !strings.Contains(html, `>PROCESSED DOM</h1>`) {
			t.Fatalf("wrong DOM: %s", html)
		}
		if !bytes.HasPrefix(pdf, []byte("%PDF-")) || !bytes.Contains(pdf, []byte("%%EOF")) {
			t.Fatal("invalid PDF")
		}
		path := filepath.Join(t.TempDir(), "rendered.pdf")
		if err := os.WriteFile(path, pdf, 0600); err != nil {
			t.Fatal(err)
		}
		if tool, err := exec.LookPath("pdftotext"); err == nil {
			text, err := exec.Command(tool, path, "-").CombinedOutput()
			if err != nil || !bytes.Contains(text, []byte("PROCESSED DOM")) || !bytes.Contains(text, []byte("SECOND PAGE")) || bytes.Contains(text, []byte("NOT PROCESSED")) || !bytes.Contains(text, []byte("PRINT ONLY CONTENT")) || bytes.Contains(text, []byte("SCREEN ONLY CONTENT")) {
				t.Fatalf("PDF text: %v %s", err, text)
			}
		}
		if tool, err := exec.LookPath("pdfinfo"); err == nil {
			info, err := exec.Command(tool, path).CombinedOutput()
			var width, height float64
			for _, line := range strings.Split(string(info), "\n") {
				if strings.HasPrefix(line, "Page size:") {
					fmt.Sscanf(strings.TrimPrefix(line, "Page size:"), "%f x %f", &width, &height)
				}
			}
			if err != nil || math.Abs(width-100*72/25.4) > 2 || math.Abs(height-150*72/25.4) > 2 {
				t.Fatalf("CSS page size not preserved: %v %s", err, info)
			}
		}
	})
	t.Run("pdf-http-and-wss", func(t *testing.T) {
		m := &Module{settings: s, renderer: r}
		mux := http.NewServeMux()
		mux.HandleFunc("/pdf", m.handleRender)
		mux.HandleFunc("/api/html-print-formatter/pdf", m.handleRender)
		for _, in := range []Request{{HTML: `<!DOCTYPE html><html><body><h1>STATIC PDF</h1></body></html>`, TimeoutMS: 5000}, {HTMLLink: fixture.URL + "/static", TimeoutMS: 5000}} {
			body, _ := json.Marshal(in)
			req := httptest.NewRequest("POST", "/pdf", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != 200 || w.Header().Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-")) || w.Header().Get("Content-Disposition") != `attachment; filename="rendered.pdf"` {
				t.Fatalf("HTTP PDF: %d %s", w.Code, w.Body.String())
			}
			out, err := apibridge.Invoke(context.Background(), mux, apibridge.Principal{Subject: "test", TenantID: "wisemed"}, apibridge.Request{Method: "POST", Path: "/api/html-print-formatter/pdf", Body: body})
			if err != nil || out.Status != 200 {
				t.Fatalf("WSS PDF: %v %+v", err, out)
			}
			payload := out.Body.(map[string]interface{})
			pdf, err := base64.StdEncoding.DecodeString(payload["pdf_base64"].(string))
			if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) || payload["html"] == "" {
				t.Fatal("missing PDF/DOM")
			}
		}
	})
	t.Run("pdf-errors-and-timeout", func(t *testing.T) {
		_, _, err := r.RenderPDF(context.Background(), Request{HTML: `<html><body>No signal</body></html>`, WaitForPrintReady: true, TimeoutMS: 400}, "pdf-timeout")
		requireCode(t, err, "render_timeout")
		_, _, err = r.RenderPDF(context.Background(), Request{HTML: `<html><script>window.printError='PDF page failed'</script></html>`, TimeoutMS: 5000}, "pdf-error")
		requireCode(t, err, "page_error")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err = r.RenderPDF(ctx, Request{HTML: `<html></html>`}, "pdf-cancel")
		requireCode(t, err, "render_canceled")
	})
	t.Run("helper-browser-download", func(t *testing.T) {
		m := &Module{settings: s, renderer: r}
		mux := http.NewServeMux()
		mux.HandleFunc("/api/html-print-formatter/pdf", m.handleRender)
		mux.Handle("/help/", http.StripPrefix("/help/", http.FileServer(http.Dir("../../apps/html-print-formatter/deployments/help"))))
		server := httptest.NewServer(mux)
		defer server.Close()
		browser, closeTab := chromedp.NewContext(r.browser)
		defer closeTab()
		ctx, cancel := context.WithTimeout(browser, 15*time.Second)
		defer cancel()
		for _, button := range []string{"pdf", "render"} {
			js := `document.getElementById('request').value='<html><body><h1>BROWSER PDF</h1>`
			if button == "render" {
				js += `<script>window.printReady=true</script>`
			}
			js += `</body></html>';document.getElementById('` + button + `').click();`
			if err := chromedp.Run(ctx, chromedp.Navigate(server.URL+"/help/"), chromedp.Evaluate(js, nil), chromedp.Poll(`document.getElementById('download').hidden === false`, nil)); err != nil {
				t.Fatal(err)
			}
			var prefix string
			if err := chromedp.Run(ctx, chromedp.Evaluate(`fetch(document.getElementById('download').href).then(r=>r.text()).then(t=>t.slice(0,5))`, &prefix, func(p *cdpruntime.EvaluateParams) *cdpruntime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil || prefix != "%PDF-" {
				t.Fatalf("download: %s %v", prefix, err)
			}
		}
	})
	// Cleanup must have disposed every per-request browser context, even errors.
	ids, err := target.GetBrowserContexts().Do(cdp.WithExecutor(r.browser, chromedp.FromContext(r.browser).Browser))
	if err != nil || len(ids) != 0 {
		t.Fatalf("leaked browser contexts: %v %v", ids, err)
	}
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	if e, ok := err.(*RenderError); !ok || e.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}
