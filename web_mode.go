// Curl-able like https://github.com/fortio/h2life
package main

import (
	"bufio"
	"flag"
	"net/http"
	"strings"
	"time"

	"fortio.org/fortio/fhttp"
	"fortio.org/log"
	"fortio.org/progressbar"
	"fortio.org/scli"
	"fortio.org/terminal/ansipixels"
)

var (
	portFlag = flag.String("http", "", "Port to listen on, eg :3000, empty for normal CLI interactive mode")
	delay    time.Duration
	maxIter  int64
)

func HttpMode(fpsLimit float64) int {
	mux, _ := fhttp.HTTPServer("fps", *portFlag)
	delay = time.Duration(float64(time.Second) / fpsLimit)
	if delay <= 0 {
		return log.FErrf("http mode: FPS limit must be > 0, got %f -> %v", fpsLimit, delay)
	}
	maxIter = *exactlyFlag
	if maxIter <= 1 {
		return log.FErrf("http mode: -n must be set for  limit must be > 0, got %d", maxIter)
	}
	log.Infof("http mode: input fps %.1f -> delay %v; num frames: %d", fpsLimit, delay, maxIter)
	mux.HandleFunc("GET /fire", log.LogAndCall("fire",
		fhttp.Gzip(http.HandlerFunc(fpsHandler)).ServeHTTP))
	scli.UntilInterrupted()
	return 0
}

func isRealBrowser(userAgent string) bool {
	return strings.Contains(userAgent, "Mozilla")
}

func fpsHandler(w http.ResponseWriter, r *http.Request) {
	if isRealBrowser(r.UserAgent()) {
		http.Redirect(w, r, "https://github.com/fortio/fps", http.StatusFound)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	// chunked is implied by multiple writes/flushes and no content-length
	w.WriteHeader(http.StatusOK)
	ww := bufio.NewWriter(w)
	ap := &ansipixels.AnsiPixels{Out: ww}
	ap.W = 80
	ap.H = 24
	ap.Color = true
	ap.TrueColor = true
	// check color query param
	if r.URL.Query().Get("colors") == "256" {
		ap.TrueColor = false
	}
	ap.Margin = 1
	ap.ClearScreen()
	drawBox(ap, false)
	cfg := progressbar.DefaultConfig()
	cfg.ScreenWriter = ww
	cfg.UpdateInterval = 0
	cfg.Prefix = ansipixels.SquareBottomLeft + ansipixels.Horizontal
	cfg.Width = 60 // smaller so there is room for the life game on last line too.
	pbar := cfg.NewBar()
	for i := range maxIter {
		// Add go up 1 line + \n
		// curl needs to see a \n to flush without --no-buffer
		ap.StartSyncMode()
		AnimateFire(ap, i)
		ap.MoveCursor(3, ap.H-1)
		pbar.Progress(float64(100*i) / float64(maxIter))
		ww.Flush()
		ap.EndSyncMode()
		_, _ = w.Write([]byte("\x1b[1A\n"))
		flusher.Flush()
		select {
		case <-r.Context().Done():
			log.LogVf("Client disconnected")
			return
		case <-time.After(delay):
			// fmt.Fprintln(ww, i)
			log.LogVf("Iteration %d", i)
		}
	}
	pbar.Progress(100.0)
	ww.Flush()
	_, _ = w.Write([]byte("\r\n\n"))
}
