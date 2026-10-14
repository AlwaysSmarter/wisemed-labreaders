package htmlprintformatter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	cdplog "github.com/chromedp/cdproto/log"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
)

type Renderer struct {
	settings       Settings
	browser        context.Context
	closeBrowser   context.CancelFunc
	closeAllocator context.CancelFunc
	sem            chan struct{}
	logf           func(string, ...interface{})
	documents      sync.Map
	documentServer *http.Server
	documentURL    string
}

func NewRenderer(ctx context.Context, s Settings, logf func(string, ...interface{})) (*Renderer, error) {
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	// chromedp otherwise adds --no-sandbox automatically when running as root.
	opts = append(opts, chromedp.Flag("no-sandbox", false))
	if s.ChromePath != "" {
		opts = append(opts, chromedp.ExecPath(s.ChromePath))
	}
	a, ac := chromedp.NewExecAllocator(ctx, opts...)
	b, bc := chromedp.NewContext(a, chromedp.WithErrorf(func(f string, args ...interface{}) { logf("chromium: "+f, args...) }))
	startupTimer := time.AfterFunc(30*time.Second, ac)
	defer startupTimer.Stop()
	if err := chromedp.Run(b); err != nil {
		bc()
		ac()
		return nil, fmt.Errorf("start Chromium (install Chrome/Chromium or set CHROME_PATH): %w", err)
	}
	r := &Renderer{settings: s, browser: b, closeBrowser: bc, closeAllocator: ac, sem: make(chan struct{}, s.MaxConcurrent), logf: logf}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		bc()
		ac()
		return nil, err
	}
	r.documentURL = "http://" + listener.Addr().String()
	r.documentServer = &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != "GET" {
			http.NotFound(w, req)
			return
		}
		doc, ok := r.documents.Load(req.URL.Path)
		if !ok {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, doc.(string))
	})}
	go func() {
		if e := r.documentServer.Serve(listener); e != nil && !errors.Is(e, http.ErrServerClosed) {
			logf("html-print-formatter document server: %v", e)
		}
	}()
	return r, nil
}

func (r *Renderer) Close() { r.closeBrowser(); r.closeAllocator(); _ = r.documentServer.Close() }
func (r *Renderer) Healthy() bool {
	return r.browser.Err() == nil && !channelClosed(chromedp.FromContext(r.browser).Browser.LostConnection)
}
func channelClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func (r *Renderer) Render(parent context.Context, req Request, id string) (string, error) {
	return r.render(parent, req, id, nil)
}

// RenderPDF prints the settled live DOM in the same tab, without loading the
// serialized HTML into a second document (which would execute its scripts again).
func (r *Renderer) RenderPDF(parent context.Context, req Request, id string) (string, []byte, error) {
	var pdf []byte
	html, err := r.render(parent, req, id, &pdf)
	return html, pdf, err
}

func (r *Renderer) render(parent context.Context, req Request, id string, pdf *[]byte) (result string, err error) {
	if err = req.validate(r.settings); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(req.TimeoutMS)*time.Millisecond)
	defer cancel()
	stopShutdown := context.AfterFunc(r.browser, cancel)
	defer stopShutdown()
	defer func() {
		if err == nil {
			return
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = failure("render_timeout", "Render deadline exceeded while queued, loading, waiting for readiness, settling assets or printing PDF", 504)
		} else if ctx.Err() != nil {
			err = failure("render_canceled", "Request canceled or service shutting down", 408)
		} else {
			var re *RenderError
			if !errors.As(err, &re) {
				err = failure("render_failed", err.Error(), 502)
			}
		}
		r.logf("html-print-formatter request_id=%s error=%q", id, err)
	}()
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if !r.Healthy() {
		return "", failure("browser_unavailable", "Chromium is unavailable; restart the service", 503)
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	// Own the BrowserContext explicitly so it is also disposed when target
	// creation/attachment fails (chromedp's automatic cleanup requires a Target).
	browserExecutor := cdp.WithExecutor(ctx, chromedp.FromContext(r.browser).Browser)
	// Do not lose the new context ID if the client cancels after Chrome creates
	// it but before its reply arrives. Allocation has a bounded cleanup budget;
	// no document is loaded after the caller's deadline has expired.
	allocation, endAllocation := context.WithTimeout(r.browser, 5*time.Second)
	bcID, err := target.CreateBrowserContext().WithDisposeOnDetach(true).Do(cdp.WithExecutor(allocation, chromedp.FromContext(r.browser).Browser))
	endAllocation()
	if err != nil {
		// An interrupted allocation may have created an unaddressable context.
		r.closeBrowser()
		return "", err
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		if e := target.DisposeBrowserContext(bcID).Do(cdp.WithExecutor(cleanup, chromedp.FromContext(r.browser).Browser)); e != nil && r.Healthy() {
			r.logf("html-print-formatter request_id=%s context_cleanup_error=%q", id, e)
			r.closeBrowser()
			if err == nil {
				err = e
			}
		}
	}()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	// Omit newWindow, rather than sending false: Chrome's unified headless mode
	// rejects an explicit newWindow on an empty incognito context (Chromium 454825274).
	var created struct {
		TargetID target.ID `json:"targetId"`
	}
	err = cdp.Execute(browserExecutor, target.CommandCreateTarget, struct {
		URL              string               `json:"url"`
		BrowserContextID cdp.BrowserContextID `json:"browserContextId"`
	}{"about:blank", bcID}, &created)
	if err != nil {
		return "", err
	}
	tab, closeTab := chromedp.NewContext(r.browser, chromedp.WithTargetID(created.TargetID))
	defer closeTab()
	work, stop := context.WithCancel(tab)
	defer stop()
	stopClient := context.AfterFunc(ctx, stop)
	defer stopClient()
	if err = chromedp.Run(work); err != nil {
		return "", err
	}
	var mainFrame cdp.FrameID
	err = chromedp.Run(work, chromedp.ActionFunc(func(ctx context.Context) error {
		tree, e := page.GetFrameTree().Do(ctx)
		if e == nil {
			mainFrame = tree.Frame.ID
		}
		return e
	}), emulation.SetDeviceMetricsOverride(int64(req.ViewportWidth), int64(req.ViewportHeight), 1, false), emulation.SetEmulatedMedia().WithMedia("screen"))
	if err != nil {
		return "", err
	}
	url := req.HTMLLink
	inline := url == ""
	if inline {
		path := "/documents/" + uuid.NewString()
		r.documents.Store(path, withBase(req.HTML, req.BaseURL))
		defer r.documents.Delete(path)
		url = r.documentURL + path
	}
	var interceptMu sync.Mutex
	var interceptErr error
	setInterceptError := func(e error) {
		interceptMu.Lock()
		if interceptErr == nil {
			interceptErr = e
		}
		interceptMu.Unlock()
		stop()
	}
	chromedp.ListenTarget(work, func(ev interface{}) {
		switch e := ev.(type) {
		case *cdplog.EventEntryAdded:
			b, _ := json.Marshal(e.Entry)
			r.logf("html-print-formatter request_id=%s browser_log=%s", id, b)
		case *cdpruntime.EventConsoleAPICalled:
			b, _ := json.Marshal(e.Args)
			r.logf("html-print-formatter request_id=%s console=%s args=%s", id, e.Type, b)
		case *cdpruntime.EventExceptionThrown:
			b, _ := json.Marshal(e.ExceptionDetails)
			r.logf("html-print-formatter request_id=%s javascript_error=%s", id, b)
		case *fetch.EventRequestPaused:
			// CDP listeners must not block the event reader with protocol calls.
			go func() {
				exec := cdp.WithExecutor(work, chromedp.FromContext(work).Target)
				if e.FrameID != mainFrame {
					if x := fetch.ContinueRequest(e.RequestID).Do(exec); x != nil {
						setInterceptError(x)
					}
					return
				}
				if e.ResponseStatusCode >= 300 && e.ResponseStatusCode < 400 {
					if x := fetch.ContinueRequest(e.RequestID).Do(exec); x != nil {
						setInterceptError(x)
					}
					return
				}
				if e.ResponseStatusCode < 200 || e.ResponseStatusCode >= 300 {
					setInterceptError(failure("source_load_failed", fmt.Sprintf("Document HTTP status %d (%s)", e.ResponseStatusCode, e.ResponseErrorReason), 502))
					return
				}
				if req.BaseURL == "" {
					if x := fetch.ContinueRequest(e.RequestID).Do(exec); x != nil {
						setInterceptError(x)
					}
					return
				}
				body, x := fetch.GetResponseBody(e.RequestID).Do(exec)
				if x != nil {
					setInterceptError(x)
					return
				}
				if int64(len(body)) > r.settings.MaxBodyBytes {
					setInterceptError(failure("body_too_large", "Linked HTML exceeds max_body_bytes during base injection", 413))
					return
				}
				var headers []*fetch.HeaderEntry
				for _, h := range e.ResponseHeaders {
					if !strings.EqualFold(h.Name, "content-length") && !strings.EqualFold(h.Name, "content-encoding") {
						headers = append(headers, h)
					}
				}
				status := e.ResponseStatusCode
				body = []byte(withBase(string(body), req.BaseURL))
				if x := fetch.FulfillRequest(e.RequestID, status).WithResponseHeaders(headers).WithBody(base64.StdEncoding.EncodeToString(body)).Do(exec); x != nil {
					setInterceptError(x)
				}
			}()
		}
	})
	if !inline {
		err = chromedp.Run(work, fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "*", ResourceType: network.ResourceTypeDocument, RequestStage: fetch.RequestStageResponse}}))
		if err != nil {
			return "", err
		}
	}
	err = chromedp.Run(work, cdplog.Enable(), chromedp.ActionFunc(func(ctx context.Context) error {
		_, _, message, _, e := page.Navigate(url).Do(ctx)
		if e != nil {
			return e
		}
		if message != "" {
			return errors.New(message)
		}
		return nil
	}))
	if err == nil {
		err = waitSignal(work, pdf == nil || req.WaitForPrintReady)
	}
	if err == nil && pdf != nil {
		// Keep report calculations in screen media, then apply print styles before
		// settling assets and exporting. Allow media-query listeners and layout to
		// react before collecting images and waiting for print-specific fonts.
		err = chromedp.Run(work, emulation.SetEmulatedMedia().WithMedia("print"),
			chromedp.Evaluate(`new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(() => { document.documentElement.getBoundingClientRect(); resolve(true); })))`, nil,
				func(p *cdpruntime.EvaluateParams) *cdpruntime.EvaluateParams { return p.WithAwaitPromise(true) }))
	}
	if err == nil {
		err = chromedp.Run(work, chromedp.Evaluate(settleAndSerialize, &result, func(p *cdpruntime.EvaluateParams) *cdpruntime.EvaluateParams { return p.WithAwaitPromise(true) }))
	}
	interceptMu.Lock()
	sourceErr := interceptErr
	interceptMu.Unlock()
	if sourceErr != nil {
		return "", sourceErr
	}
	if err != nil {
		return "", err
	}
	// JS returns an envelope so a late printError has the same stable error code.
	var out struct {
		HTML  string  `json:"html"`
		Error *string `json:"error"`
	}
	if err = json.Unmarshal([]byte(result), &out); err != nil {
		return "", err
	}
	if out.Error != nil {
		return "", failure("page_error", *out.Error, 422)
	}
	if pdf != nil {
		err = chromedp.Run(work, chromedp.ActionFunc(func(ctx context.Context) error {
			var e error
			*pdf, _, e = page.PrintToPDF().WithPrintBackground(true).
				WithDisplayHeaderFooter(false).WithPreferCSSPageSize(true).
				WithPaperWidth(210.0 / 25.4).WithPaperHeight(297.0 / 25.4).
				WithMarginTop(0).WithMarginBottom(0).WithMarginLeft(0).WithMarginRight(0).Do(ctx)
			return e
		}))
		if err != nil {
			return "", failure("pdf_failed", err.Error(), 502)
		}
		r.logf("html-print-formatter request_id=%s pdf_bytes=%d", id, len(*pdf))
	}
	r.logf("html-print-formatter request_id=%s completed bytes=%d", id, len(out.HTML))
	return out.HTML, nil
}

func waitSignal(ctx context.Context, requirePrintReady bool) error {
	ready := "document.readyState === 'complete'"
	if requirePrintReady {
		ready = "window.printReady === true"
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var signal struct {
			Ready bool    `json:"ready"`
			Error *string `json:"error"`
		}
		err := chromedp.Run(ctx, chromedp.Evaluate(`({ready: location.href !== 'about:blank' && (`+ready+`), error: window.printError == null ? null : String(window.printError)})`, &signal))
		if err == nil {
			if signal.Error != nil {
				return failure("page_error", *signal.Error, 422)
			}
			if signal.Ready {
				return nil
			}
		} else if !strings.Contains(err.Error(), "Cannot find context") && !strings.Contains(err.Error(), "Execution context was destroyed") {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

const settleAndSerialize = `(async () => {
  let timer;
  const pageError = () => window.printError == null ? null : String(window.printError);
  const failed = new Promise(resolve => {
    timer = setInterval(() => { const error = pageError(); if (error !== null) resolve({error}); }, 20);
  });
  const finish = async () => {
    // Lazy images would otherwise remain pending outside the viewport.
    const images = Array.from(document.images);
    const lazy = images.filter(image => image.loading === 'lazy').map(image => [image, image.getAttribute('loading')]);
    for (const [image] of lazy) image.loading = 'eager';
    if (document.fonts) await document.fonts.ready;
    await Promise.all(images.map(image => image.complete ? Promise.resolve() : new Promise(resolve => {
      image.addEventListener('load', resolve, {once:true});
      image.addEventListener('error', resolve, {once:true});
      if (image.complete) resolve();
    })));
    for (const [image, loading] of lazy) image.setAttribute('loading', loading);
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    const error = pageError();
    if (error !== null) return {error};
    const dt = document.doctype;
    const doctype = dt ? new XMLSerializer().serializeToString(dt) + '\n' : '';
    return {html: doctype + document.documentElement.outerHTML};
  };
  try { return JSON.stringify(await Promise.race([failed, finish()])); }
  finally { clearInterval(timer); }
})()`
