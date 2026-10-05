// Package devserver implements `kvist dev`: build a vault folder, serve the
// site, rebuild when notes or theme files change, and reload the browser.
// It is for working on notes and themes locally, never for production.
package devserver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/klppl/kvist/internal/build"
	"github.com/klppl/kvist/internal/config"
	"github.com/klppl/kvist/internal/protocol"
	"github.com/klppl/kvist/internal/serve"
	"github.com/klppl/kvist/internal/source/dir"
)

// Server is a development server for one site.
type Server struct {
	Site      *config.Site
	ThemesDir string
	Vault     string
	Log       *slog.Logger
	// OnBuild, if set, is called after every build (for tests).
	OnBuild func(err error)

	work    string // temp folder holding builds and the current symlink
	n       int
	hist    *build.History // redirects for notes renamed while running
	mu      sync.Mutex
	clients map[chan string]bool
	lastErr string
}

// Run builds once, then serves on listen until ctx ends.
func (s *Server) Run(ctx context.Context, listen string) error {
	h, err := s.Start(ctx)
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: listen, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	s.Log.Info("serving", "url", "http://"+listen+"/", "vault", s.Vault)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Start does the first build, starts watching and returns the handler.
func (s *Server) Start(ctx context.Context) (http.Handler, error) {
	if s.Log == nil {
		s.Log = slog.Default()
	}
	work, err := os.MkdirTemp("", "kvist-dev-*")
	if err != nil {
		return nil, err
	}
	s.work = work
	s.clients = map[chan string]bool{}
	go func() {
		<-ctx.Done()
		_ = os.RemoveAll(work)
	}()
	s.rebuild(ctx)
	go s.watch(ctx)

	static := serve.New([]serve.Site{{PublicDir: filepath.Join(work, "current")}})
	mux := http.NewServeMux()
	mux.HandleFunc("/_kvist/livereload", s.events)
	mux.Handle("/", s.inject(static))
	return mux, nil
}

func (s *Server) watch(ctx context.Context) {
	changes := []<-chan struct{}{dir.New(s.Vault).Changes()}
	for _, d := range s.themeDirs() {
		changes = append(changes, dir.New(d).Changes())
	}
	merged := make(chan struct{}, 1)
	for _, c := range changes {
		go func(c <-chan struct{}) {
			for range c {
				select {
				case merged <- struct{}{}:
				default:
				}
			}
		}(c)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-merged:
			// Let a burst of saves settle.
			time.Sleep(150 * time.Millisecond)
			select {
			case <-merged:
			default:
			}
			s.rebuild(ctx)
		}
	}
}

// themeDirs returns theme folders on disk worth watching.
func (s *Server) themeDirs() []string {
	var dirs []string
	if strings.ContainsAny(s.Site.Theme, `/\`) {
		dirs = append(dirs, s.Site.Theme)
	} else if s.ThemesDir != "" {
		if _, err := os.Stat(filepath.Join(s.ThemesDir, s.Site.Theme, "theme.toml")); err == nil {
			dirs = append(dirs, filepath.Join(s.ThemesDir, s.Site.Theme))
		}
	}
	if s.Site.ThemeDir != "" {
		dirs = append(dirs, s.Site.ThemeDir)
	}
	return dirs
}

func (s *Server) rebuild(ctx context.Context) {
	start := time.Now()
	err := s.build(ctx)
	if s.OnBuild != nil {
		defer s.OnBuild(err)
	}
	if err != nil {
		s.Log.Error("build failed; still serving the last good build", "err", err)
		s.mu.Lock()
		s.lastErr = err.Error()
		s.mu.Unlock()
		s.broadcast("error:" + err.Error())
		return
	}
	s.mu.Lock()
	s.lastErr = ""
	s.mu.Unlock()
	s.Log.Info("built", "in", time.Since(start).Round(time.Millisecond))
	s.broadcast("reload")
}

func (s *Server) build(ctx context.Context) error {
	theme, err := build.LoadTheme(s.ThemesDir, s.Site)
	if err != nil {
		return err
	}
	snap, err := dir.New(s.Vault).Snapshot(ctx)
	if err != nil {
		return err
	}
	s.n++
	out := filepath.Join(s.work, "b"+strconv.Itoa(s.n))
	current := filepath.Join(s.work, "current")
	prev, _ := filepath.EvalSymlinks(current)
	if s.hist == nil {
		s.hist = &build.History{Pages: map[string]build.Page{}, Redirects: map[string]string{}}
	}
	warnings, hist, err := build.WriteSiteIncremental(ctx, s.Site, theme, snap, out, prev, s.hist)
	if err != nil {
		_ = os.RemoveAll(out)
		return err
	}
	s.hist = hist
	for _, w := range warnings {
		logWarning(s.Log, w)
	}
	tmp := current + ".new"
	_ = os.Remove(tmp)
	if err := os.Symlink(out, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, current); err != nil {
		return err
	}
	if prev != "" {
		// Give in-flight requests a moment before removing the old build.
		go func() { time.Sleep(5 * time.Second); _ = os.RemoveAll(prev) }()
	}
	return nil
}

func logWarning(log *slog.Logger, w protocol.Warning) {
	log.Warn(w.Message, "code", w.Code, "path", w.Path)
}

// --- live reload ---

func (s *Server) broadcast(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		select {
		case c <- msg:
		default:
		}
	}
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	c := make(chan string, 4)
	s.mu.Lock()
	s.clients[c] = true
	lastErr := s.lastErr
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
	}()
	fmt.Fprint(w, ": connected\n\n")
	if lastErr != "" {
		writeEvent(w, "error:"+lastErr)
	}
	fl.Flush()
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-c:
			writeEvent(w, msg)
			fl.Flush()
		case <-tick.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func writeEvent(w io.Writer, msg string) {
	for _, line := range strings.Split(msg, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

// reloadScript reloads on "reload" and shows build errors in a banner.
const reloadScript = `<script>(function(){var es=new EventSource("/_kvist/livereload"),bar;
es.onmessage=function(e){if(e.data==="reload"){location.reload();return}
if(e.data.indexOf("error:")===0){if(!bar){bar=document.createElement("pre");
bar.style.cssText="position:fixed;left:0;right:0;bottom:0;z-index:99;margin:0;padding:1rem 1.25rem;max-height:40vh;overflow:auto;background:#7f1d1d;color:#fff;font:13px/1.5 ui-monospace,monospace;white-space:pre-wrap";
document.body.appendChild(bar)}bar.textContent="kvist: build failed (showing the last good build)\n\n"+e.data.slice(6)}};})();</script>`

// inject adds the live reload script to HTML pages. It is added while
// serving, never written into the build.
func (s *Server) inject(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		next.ServeHTTP(rec, r)
		res := rec.Result()
		body := rec.Body.Bytes()
		if strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") {
			if i := bytes.LastIndex(body, []byte("</body>")); i >= 0 {
				body = append(body[:i:i], append([]byte(reloadScript), body[i:]...)...)
			}
		}
		for k, v := range res.Header {
			w.Header()[k] = v
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(res.StatusCode)
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
	})
}
