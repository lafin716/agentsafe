package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"unicode"

	"github.com/agentsafe/agentsafe/internal/applog"
)

// bridgeServer is a loopback HTTP server that lets detached "popout" windows
// reach the same Go backend as the main Wails window. Wails v2 is single-window
// and does not inject the `window.go` bindings or `window.runtime` event bus
// into windows opened with window.open, so a popout (served from this server's
// origin) talks to the backend over:
//   - POST /rpc/{Method}  : JSON-array body of args, reflection-dispatched to App
//   - GET  /events        : Server-Sent Events stream mirroring every Wails event
//   - GET  /              : the embedded SPA assets (frontend/dist)
//
// All RPC/event access is guarded by a per-launch random token and a loopback
// Host check (DNS-rebinding mitigation). Static assets are served without a
// token (they are just the public app bundle); the sensitive surface is the
// token-guarded RPC/event endpoints.
type bridgeServer struct {
	app   *App
	token string
	addr  string // 127.0.0.1:<port>

	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// newBridge binds a loopback listener, generates a session token, and starts
// serving in a background goroutine. assets is the frontend/dist sub-filesystem.
func newBridge(app *App, assets fs.FS) (*bridgeServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		_ = ln.Close()
		return nil, err
	}
	b := &bridgeServer{
		app:   app,
		token: hex.EncodeToString(raw),
		addr:  ln.Addr().String(),
		subs:  map[chan []byte]struct{}{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc/", b.handleRPC)
	mux.HandleFunc("/events", b.handleEvents)
	mux.Handle("/", http.FileServer(http.FS(assets)))

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			applog.Warn("popout bridge server stopped", "err", err)
		}
	}()
	applog.Info("popout bridge listening", "addr", b.addr)
	return b, nil
}

// popoutURL builds the loopback URL that opens the given serialized view in a
// detached window. viewJSON is the JSON of the frontend View object.
func (b *bridgeServer) popoutURL(viewJSON string) string {
	return fmt.Sprintf(
		"http://%s/?popout=%s&token=%s",
		b.addr, url.QueryEscape(viewJSON), url.QueryEscape(b.token),
	)
}

// broadcast pushes a Wails event to every connected popout. It never blocks the
// caller: slow subscribers drop the message rather than stalling event emission.
func (b *bridgeServer) broadcast(name string, data any) {
	if b == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{"event": name, "data": data})
	if err != nil {
		return
	}
	b.mu.Lock()
	for ch := range b.subs {
		select {
		case ch <- payload:
		default:
		}
	}
	b.mu.Unlock()
}

// authorized enforces the loopback Host check and the session token. Returns
// false (and writes the error response) when the request must be rejected.
func (b *bridgeServer) authorized(w http.ResponseWriter, r *http.Request) bool {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host != "127.0.0.1" && host != "localhost" {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return false
	}
	tok := r.URL.Query().Get("token")
	if tok == "" {
		tok = r.Header.Get("X-Bridge-Token")
	}
	if subtle.ConstantTimeCompare([]byte(tok), []byte(b.token)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

// handleRPC dispatches POST /rpc/{Method} to the matching exported App method
// by reflection, mirroring how Wails binds App. The body is a JSON array whose
// elements decode into the method's parameters; the response is the method's
// non-error return value (or null), and a trailing error return maps to HTTP 400.
func (b *bridgeServer) handleRPC(w http.ResponseWriter, r *http.Request) {
	if !b.authorized(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/rpc/")
	if name == "" || !isExportedName(name) {
		http.Error(w, "unknown method", http.StatusNotFound)
		return
	}
	method := reflect.ValueOf(b.app).MethodByName(name)
	if !method.IsValid() {
		http.Error(w, "unknown method", http.StatusNotFound)
		return
	}

	var rawArgs []json.RawMessage
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&rawArgs); err != nil && err != io.EOF {
			http.Error(w, "bad args: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	mt := method.Type()
	if len(rawArgs) != mt.NumIn() {
		http.Error(w, fmt.Sprintf("method %s expects %d args, got %d", name, mt.NumIn(), len(rawArgs)), http.StatusBadRequest)
		return
	}
	in := make([]reflect.Value, mt.NumIn())
	for i := 0; i < mt.NumIn(); i++ {
		argPtr := reflect.New(mt.In(i))
		if err := json.Unmarshal(rawArgs[i], argPtr.Interface()); err != nil {
			http.Error(w, fmt.Sprintf("bad arg %d for %s: %v", i, name, err), http.StatusBadRequest)
			return
		}
		in[i] = argPtr.Elem()
	}

	out := method.Call(in)
	var result any
	var callErr error
	for _, ov := range out {
		if ov.Type() == errorType {
			if !ov.IsNil() {
				callErr = ov.Interface().(error)
			}
			continue
		}
		result = ov.Interface()
	}
	if callErr != nil {
		http.Error(w, callErr.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if result == nil {
		_, _ = w.Write([]byte("null"))
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

// handleEvents streams every Wails event to the popout as Server-Sent Events.
// The popout's window.runtime.EventsOn reads this stream; it only consumes
// events (input/resize go back through /rpc), so one-way SSE is sufficient.
func (b *bridgeServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !b.authorized(w, r) {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan []byte, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}()

	flusher.Flush()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// isExportedName reports whether name starts with an upper-case letter, so only
// the exported App methods (the same surface Wails binds) are RPC-callable.
func isExportedName(name string) bool {
	for _, r := range name {
		return unicode.IsUpper(r)
	}
	return false
}
