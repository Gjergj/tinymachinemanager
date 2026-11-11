package serialconsole

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// ConsoleStream is the byte-stream boundary shared by the native adapter and
// authenticated transport. The transport is independently testable without VMs.
type ConsoleStream interface {
	Read(context.Context) ([]byte, error)
	Write([]byte) error
	Ack(int) error
	Close() error
}
type Credentials struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Token string `json:"token"`
}
type pendingSession struct {
	owner, uuid, key, revision string
	hash                       [32]byte
	expires                    time.Time
	claimed                    bool
	stream                     ConsoleStream
	cancel                     context.CancelFunc
	timer                      *time.Timer
}
type Bridge struct {
	mu          sync.Mutex
	pending     map[string]*pendingSession
	origins     map[string]bool
	allowOpaque bool
	server      *http.Server
	listener    net.Listener
	closed      bool
	ctx         context.Context
	cancel      context.CancelFunc
	open        func(context.Context, string, string, string) (ConsoleStream, error)
}

func NewBridge(origins []string, allowOpaque bool) *Bridge {
	ctx, cancel := context.WithCancel(context.Background())
	b := &Bridge{pending: make(map[string]*pendingSession), origins: make(map[string]bool), allowOpaque: allowOpaque, ctx: ctx, cancel: cancel}
	for _, origin := range origins {
		b.origins[origin] = true
	}
	b.open = func(ctx context.Context, uuid, key, revision string) (ConsoleStream, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return Open("", uuid, key, revision)
	}
	return b
}
func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Issue reserves a VM slot; no guest connection is opened until the token is
// claimed. Credentials must travel through the owning window's binding only.
func (b *Bridge) Issue(owner, uuid, key, revision string) (Credentials, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return Credentials{}, errors.New("Console service has stopped.")
	}
	if owner == "" || uuid == "" || key == "" || revision == "" {
		return Credentials{}, errors.New("Incomplete console session request.")
	}
	for _, p := range b.pending {
		if p.owner == owner || p.uuid == uuid {
			return Credentials{}, errors.New("A console session already exists. Disconnect it before reconnecting.")
		}
	}
	if b.listener == nil {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return Credentials{}, err
		}
		b.listener = listener
		b.server = &http.Server{Handler: b, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
		go b.server.Serve(listener)
	}
	id, err := randomSecret()
	if err != nil {
		return Credentials{}, err
	}
	token, err := randomSecret()
	if err != nil {
		return Credentials{}, err
	}
	p := &pendingSession{owner: owner, uuid: uuid, key: key, revision: revision, hash: sha256.Sum256([]byte(token)), expires: time.Now().Add(30 * time.Second)}
	b.pending[id] = p
	p.timer = time.AfterFunc(30*time.Second, func() {
		b.mu.Lock()
		if current := b.pending[id]; current == p && !p.claimed {
			delete(b.pending, id)
		}
		b.mu.Unlock()
	})
	return Credentials{ID: id, URL: "ws://" + b.listener.Addr().String() + "/serial", Token: token}, nil
}

func (b *Bridge) claim(id, token string) (*pendingSession, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	p := b.pending[id]
	hash := sha256.Sum256([]byte(token))
	if p == nil || p.claimed || !time.Now().Before(p.expires) || subtle.ConstantTimeCompare(hash[:], p.hash[:]) != 1 {
		return nil, errors.New("Unauthorized console session.")
	}
	p.claimed = true
	p.hash = [32]byte{}
	p.timer.Stop()
	return p, nil
}

func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	host := ""
	if b.listener != nil {
		host = b.listener.Addr().String()
	}
	closed := b.closed
	b.mu.Unlock()
	origin := r.Header.Get("Origin")
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if closed || err != nil || !net.ParseIP(remote).IsLoopback() || r.Host != host || origin == "" || (!b.origins[origin] && !(b.allowOpaque && origin == "null")) {
		http.Error(w, "Console origin denied.", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet || r.URL.Path != "/serial" || r.URL.RawQuery != "" {
		http.Error(w, "Invalid console endpoint.", http.StatusBadRequest)
		return
	}
	// Origin validation above includes the native custom scheme and opaque
	// packaged origin, which the library's generic hostname matcher cannot express.
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer ws.CloseNow()
	ws.SetReadLimit(64 * 1024)
	authCtx, cancelAuth := context.WithTimeout(b.ctx, 5*time.Second)
	kind, data, err := ws.Read(authCtx)
	cancelAuth()
	var auth struct {
		Type      string `json:"type"`
		SessionID string `json:"sessionID"`
		Token     string `json:"token"`
	}
	if err != nil || kind != websocket.MessageText || json.Unmarshal(data, &auth) != nil || auth.Type != "auth" {
		ws.Close(websocket.StatusPolicyViolation, "Console authentication required.")
		return
	}
	p, err := b.claim(auth.SessionID, auth.Token)
	auth.Token = ""
	data = nil
	if err != nil {
		ws.Close(websocket.StatusPolicyViolation, "Console credentials are invalid or expired.")
		return
	}
	defer b.finish(auth.SessionID)
	ctx, cancel := context.WithCancel(b.ctx)
	defer cancel()
	b.mu.Lock()
	if b.pending[auth.SessionID] != p {
		b.mu.Unlock()
		return
	}
	p.cancel = cancel
	b.mu.Unlock()
	stream, err := b.open(ctx, p.uuid, p.key, p.revision)
	if err != nil {
		writeStatus(ws, "error", err.Error())
		return
	}
	b.mu.Lock()
	if b.pending[auth.SessionID] != p || ctx.Err() != nil {
		b.mu.Unlock()
		stream.Close()
		return
	}
	p.stream = stream
	b.mu.Unlock()
	if err = writeStatus(ws, "connected", ""); err != nil {
		return
	}
	b.relay(ctx, cancel, ws, stream)
}

func writeStatus(ws *websocket.Conn, kind, message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	data, _ := json.Marshal(struct {
		Type    string `json:"type"`
		Message string `json:"message,omitempty"`
	}{kind, message})
	return ws.Write(ctx, websocket.MessageText, data)
}

func (b *Bridge) relay(ctx context.Context, cancel context.CancelFunc, ws *websocket.Conn, stream ConsoleStream) {
	readerDone := make(chan struct{})
	readError := make(chan error, 1)
	go func() {
		defer close(readerDone)
		defer cancel()
		for {
			kind, data, err := ws.Read(ctx)
			if err != nil {
				return
			}
			if kind == websocket.MessageBinary {
				err = stream.Write(data)
			} else {
				var ack struct {
					Type  string `json:"type"`
					Bytes int    `json:"bytes"`
				}
				if json.Unmarshal(data, &ack) != nil || ack.Type != "ack" {
					err = errors.New("Invalid console control message.")
				} else {
					err = stream.Ack(ack.Bytes)
				}
			}
			if err != nil {
				readError <- err
				return
			}
		}
	}()
	for {
		data, err := stream.Read(ctx)
		if err != nil {
			select {
			case inputErr := <-readError:
				writeStatus(ws, "error", inputErr.Error())
			default:
				if errors.Is(err, io.EOF) {
					writeStatus(ws, "disconnected", "The serial console disconnected.")
				} else if ctx.Err() == nil {
					writeStatus(ws, "error", err.Error())
				}
			}
			break
		}
		writeCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		err = ws.Write(writeCtx, websocket.MessageBinary, data)
		stop()
		if err != nil {
			break
		}
	}
	cancel()
	ws.CloseNow()
	<-readerDone
}

func (b *Bridge) finish(id string) {
	b.mu.Lock()
	p := b.pending[id]
	delete(b.pending, id)
	b.mu.Unlock()
	if p != nil {
		p.timer.Stop()
		if p.cancel != nil {
			p.cancel()
		}
		if p.stream != nil {
			p.stream.Close()
		}
	}
}
func (b *Bridge) Disconnect(owner, id string) error {
	b.mu.Lock()
	p := b.pending[id]
	if p != nil && p.owner != owner {
		b.mu.Unlock()
		return errors.New("This console belongs to another window.")
	}
	b.mu.Unlock()
	b.finish(id)
	return nil
}
func (b *Bridge) CloseOwner(owner string) {
	b.mu.Lock()
	ids := []string{}
	for id, p := range b.pending {
		if p.owner == owner {
			ids = append(ids, id)
		}
	}
	b.mu.Unlock()
	for _, id := range ids {
		b.finish(id)
	}
}
func (b *Bridge) Close() error {
	b.mu.Lock()
	b.closed = true
	ids := []string{}
	for id := range b.pending {
		ids = append(ids, id)
	}
	server := b.server
	b.mu.Unlock()
	b.cancel()
	for _, id := range ids {
		b.finish(id)
	}
	if server != nil {
		return server.Close()
	}
	return nil
}
