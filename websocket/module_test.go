package websocket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Scardice/quickjs_nodejs/eventloop"
	quickjs "github.com/buke/quickjs-go"
	gorilla "github.com/gorilla/websocket"
)

func TestWebSocketLifecycleWithExplicitDialer(t *testing.T) {
	upgrader := gorilla.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if err := conn.WriteMessage(gorilla.TextMessage, []byte("hello")); err != nil {
			return
		}
		_, message, err := conn.ReadMessage()
		if err != nil || string(message) != "ping" {
			return
		}
		_ = conn.WriteControl(gorilla.CloseMessage, gorilla.FormatCloseMessage(gorilla.CloseNormalClosure, ""), time.Now().Add(time.Second))
	}))
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}

	result := make(chan string, 4)
	setupDone := make(chan error, 1)
	if !loop.Schedule(func(ctx *quickjs.Context) error {
		if err := InstallGlobal(ctx, WithDialer(&gorillaDialer{dialer: gorilla.Dialer{}})); err != nil {
			setupDone <- err
			return err
		}
		report := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			if len(args) > 0 {
				result <- args[0].ToString()
			}
			return ctx.NewUndefined()
		})
		ctx.Globals().Set("report", report)
		value := ctx.Eval(`
			const ws = new WebSocket(` + strconv.Quote(wsURL) + `);
			ws.onopen = () => { report("open"); ws.send("ping"); };
			ws.onmessage = event => report("message:" + event.data);
			ws.onclose = event => report("close:" + event.code);
			ws.onerror = event => report("error:" + event.message);
		`)
		if value == nil {
			err := errors.New("websocket evaluation returned nil")
			setupDone <- err
			return err
		}
		if value.IsException() {
			err := ctx.Exception()
			setupDone <- err
			return err
		}
		value.Free()
		setupDone <- nil
		return nil
	}) {
		t.Fatal("failed to schedule websocket setup")
	}
	if err := <-setupDone; err != nil {
		t.Fatalf("websocket setup failed: %v", err)
	}

	want := []string{"open", "message:hello", "close:1000"}
	for _, expected := range want {
		select {
		case got := <-result:
			if got != expected {
				t.Fatalf("event order mismatch: got %q, want %q", got, expected)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %q", expected)
		}
	}
}

func TestWebSocketWithoutDialerEmitsErrorAndClose(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}

	result := make(chan string, 2)
	setupDone := make(chan error, 1)
	if !loop.Schedule(func(ctx *quickjs.Context) error {
		if err := InstallGlobal(ctx); err != nil {
			setupDone <- err
			return err
		}
		report := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			if len(args) > 0 {
				result <- args[0].ToString()
			}
			return ctx.NewUndefined()
		})
		ctx.Globals().Set("report", report)
		value := ctx.Eval(`
			const ws = new WebSocket("ws://disabled.invalid");
			ws.onerror = event => report("error:" + event.message);
			ws.onclose = event => report("close:" + event.code);
		`)
		if value == nil {
			err := errors.New("websocket evaluation returned nil")
			setupDone <- err
			return err
		}
		if value.IsException() {
			err := ctx.Exception()
			setupDone <- err
			return err
		}
		value.Free()
		setupDone <- nil
		return nil
	}) {
		t.Fatal("failed to schedule websocket setup")
	}
	if err := <-setupDone; err != nil {
		t.Fatalf("websocket setup failed: %v", err)
	}

	select {
	case got := <-result:
		if got != "error:websocket: no dialer configured" {
			t.Fatalf("unexpected error event %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket error")
	}
	select {
	case got := <-result:
		if got != "close:1006" {
			t.Fatalf("unexpected close event %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket close")
	}
}

type gorillaDialer struct {
	dialer gorilla.Dialer
}

func (d *gorillaDialer) DialContext(ctx context.Context, url string, header http.Header) (Conn, *http.Response, error) {
	conn, response, err := d.dialer.DialContext(ctx, url, header)
	if err != nil {
		return nil, response, err
	}
	return conn, response, nil
}
