package serialconsole

import (
	"context"
	"encoding/json"
	"github.com/coder/websocket"
	"net/http"
	"strings"
	"testing"
	"time"
)

func bridgeFixture(t *testing.T) (*Bridge, *int) {
	t.Helper()
	opened := 0
	b := NewBridge([]string{"wails://localhost"}, false)
	b.open = func(context.Context, string, string, string) (ConsoleStream, error) {
		opened++
		f := &fakeStream{input: []byte("Rocky login:"), eof: true}
		s, err := newSession(f, func() error { return nil })
		if err == nil {
			f.fire(1)
		}
		return s, err
	}
	t.Cleanup(func() { b.Close() })
	return b, &opened
}
func dialTicket(t *testing.T, c Credentials, origin string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, c.URL, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{origin}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.CloseNow() })
	return ws
}
func authenticate(t *testing.T, ws *websocket.Conn, c Credentials) {
	t.Helper()
	data, _ := json.Marshal(map[string]string{"type": "auth", "sessionID": c.ID, "token": c.Token})
	if err := ws.Write(context.Background(), websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}
}

func TestBridgeAuthenticatesThenDeliversRawBytes(t *testing.T) {
	b, opened := bridgeFixture(t)
	c, err := b.Issue("window1", "vm", "key", "revision")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.URL, c.Token) || strings.Contains(c.URL, c.ID) {
		t.Fatal("credentials leaked in URL")
	}
	ws := dialTicket(t, c, "wails://localhost")
	authenticate(t, ws, c)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	kind, data, err := ws.Read(ctx)
	if err != nil || kind != websocket.MessageText || !strings.Contains(string(data), "connected") {
		t.Fatalf("no connection status: %s %v", data, err)
	}
	kind, data, err = ws.Read(ctx)
	if err != nil || kind != websocket.MessageBinary || string(data) != "Rocky login:" {
		t.Fatalf("lost terminal bytes: %q %v", data, err)
	}
	if *opened != 1 {
		t.Fatal("unexpected console attach count")
	}
}

func TestBridgeRejectsOriginsBeforeOpeningGuest(t *testing.T) {
	b, opened := bridgeFixture(t)
	c, err := b.Issue("owner", "vm", "key", "rev")
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"https://evil.example", "null", ""} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		ws, response, err := websocket.Dial(ctx, c.URL, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{origin}}})
		cancel()
		if ws != nil {
			ws.CloseNow()
		}
		if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
			t.Fatalf("origin accepted %q: %v", origin, err)
		}
	}
	if *opened != 0 {
		t.Fatal("opened before authentication")
	}
}

func TestBridgeRejectsBadExpiredAndReusedTokens(t *testing.T) {
	for _, mode := range []string{"wrong", "expired", "reused"} {
		t.Run(mode, func(t *testing.T) {
			b, _ := bridgeFixture(t)
			c, err := b.Issue("owner", "vm", "key", "rev")
			if err != nil {
				t.Fatal(err)
			}
			if mode == "expired" {
				b.mu.Lock()
				b.pending[c.ID].expires = time.Now().Add(-time.Second)
				b.mu.Unlock()
			}
			if mode == "reused" {
				if _, err = b.claim(c.ID, c.Token); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "wrong" {
				c.Token = "incorrect"
			}
			ws := dialTicket(t, c, "wails://localhost")
			authenticate(t, ws, c)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, _, err = ws.Read(ctx)
			if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
				t.Fatalf("bad token was not rejected: %v", err)
			}
		})
	}
}

func TestBridgeEnforcesOwnershipAndReleasesPendingSlot(t *testing.T) {
	b, _ := bridgeFixture(t)
	c, err := b.Issue("owner", "vm", "key", "rev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.Issue("owner", "vm", "key", "rev"); err == nil {
		t.Fatal("duplicate session accepted")
	}
	if err = b.Disconnect("other", c.ID); err == nil {
		t.Fatal("other window disconnected session")
	}
	if err = b.Disconnect("owner", c.ID); err != nil {
		t.Fatal(err)
	}
	if err = b.Disconnect("owner", c.ID); err != nil {
		t.Fatal("disconnect must be idempotent")
	}
	if _, err = b.Issue("owner", "vm", "key", "rev"); err != nil {
		t.Fatal("slot not released")
	}
}

func TestBridgeRejectsMissingTokenAndOversizedFrames(t *testing.T) {
	for _, mode := range []string{"missing-token", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			b, _ := bridgeFixture(t)
			b.open = func(context.Context, string, string, string) (ConsoleStream, error) {
				return newSession(&fakeStream{}, func() error { return nil })
			}
			c, err := b.Issue("owner", "vm", "key", "rev")
			if err != nil {
				t.Fatal(err)
			}
			ws := dialTicket(t, c, "wails://localhost")
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if mode == "missing-token" {
				c.Token = ""
				authenticate(t, ws, c)
			} else {
				authenticate(t, ws, c)
				if _, _, err = ws.Read(ctx); err != nil {
					t.Fatal(err)
				}
				_ = ws.Write(ctx, websocket.MessageBinary, make([]byte, 64*1024+1))
			}
			_, _, err = ws.Read(ctx)
			if err == nil {
				t.Fatal("invalid client accepted")
			}
			if mode == "missing-token" && websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
				t.Fatalf("unexpected auth rejection: %v", err)
			}
		})
	}
}

func TestBridgeAllowsDistinctVMsAndRejectsForgedAcknowledgement(t *testing.T) {
	b, _ := bridgeFixture(t)
	b.open = func(context.Context, string, string, string) (ConsoleStream, error) {
		return newSession(&fakeStream{}, func() error { return nil })
	}
	c, err := b.Issue("one", "vm1", "key", "rev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.Issue("two", "vm2", "key", "rev"); err != nil {
		t.Fatal("independent VM rejected")
	}
	ws := dialTicket(t, c, "wails://localhost")
	authenticate(t, ws, c)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, err = ws.Read(ctx); err != nil {
		t.Fatal(err)
	}
	if err = ws.Write(ctx, websocket.MessageText, []byte(`{"type":"ack","bytes":1}`)); err != nil {
		t.Fatal(err)
	}
	_, data, err := ws.Read(ctx)
	if err != nil || !strings.Contains(string(data), "Invalid console output acknowledgement") {
		t.Fatalf("bad credit not rejected: %s %v", data, err)
	}
}
