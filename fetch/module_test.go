package fetch

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Scardice/quickjs_nodejs/abort"
	"github.com/Scardice/quickjs_nodejs/eventloop"
	"github.com/Scardice/quickjs_nodejs/url"
	quickjs "github.com/buke/quickjs-go"
)

func TestFetchUsesExplicitTransportAndBuildsResponse(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != "https://example.test/data" {
			return nil, &testError{"unexpected request"}
		}
		if got := request.Header.Get("X-Test"); got != "ok" {
			return nil, &testError{"missing request header"}
		}
		return &http.Response{
			StatusCode:    http.StatusCreated,
			Status:        "201 Created",
			Header:        http.Header{"Content-Type": []string{"application/json"}},
			Body:          io.NopCloser(strings.NewReader(`{"ok":true}`)),
			ContentLength: 11,
			Request:       request,
		}, nil
	})
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}

	result := make(chan string, 1)
	if !loop.Schedule(func(ctx *quickjs.Context) error {
		if err := InstallGlobal(ctx, WithTransport(transport)); err != nil {
			return err
		}
		report := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			if len(args) > 0 {
				result <- args[0].ToString()
			}
			return ctx.NewUndefined()
		})
		ctx.Globals().Set("reportFetch", report)
		value := ctx.Eval(`fetch("https://example.test/data", {method:"POST", headers:{"X-Test":"ok"}, body:"payload"}).then(async response => [response.status, response.statusText, response.ok, response.headers.get("content-type"), await response.text(), (await response.json()).ok].join("|")).then(reportFetch, error => reportFetch("error:" + error))`)
		if value.IsException() {
			return ctx.Exception()
		}
		value.Free()
		return nil
	}) {
		t.Fatal("fetch task was rejected")
	}
	select {
	case got := <-result:
		if got != "201|Created|true|application/json|{\"ok\":true}|true" {
			t.Fatalf("fetch result = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("fetch result timed out")
	}
}
func TestFetchEncodesURLSearchParamsAndFormData(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		result := request.Header.Get("Content-Type") + "|" + string(body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(result)),
			Request:    request,
		}, nil
	})
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}

	result := make(chan string, 1)
	if !loop.Schedule(func(ctx *quickjs.Context) error {
		if err := url.InstallGlobal(ctx); err != nil {
			return err
		}
		if err := InstallGlobal(ctx, WithTransport(transport)); err != nil {
			return err
		}
		report := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			if len(args) > 0 {
				result <- args[0].ToString()
			}
			return ctx.NewUndefined()
		})
		ctx.Globals().Set("reportFetch", report)
		value := ctx.Eval(`(async () => {
			const params = new URLSearchParams();
			params.append("a", "1");
			params.append("b", "hello world");
			const form = new FormData();
			form.append("name", "seal");
			form.append("file", "file-body", "note.txt");
			const encodedParams = await (await fetch("https://example.test/params", {method:"POST", body:params})).text();
			const encodedForm = await (await fetch("https://example.test/form", {method:"POST", body:form})).text();
			reportFetch(encodedParams + "||" + encodedForm);
		})()`)
		if value == nil {
			return errors.New("body encoding evaluation returned nil")
		}
		if value.IsException() {
			return ctx.Exception()
		}
		value.Free()
		return nil
	}) {
		t.Fatal("body encoding task was rejected")
	}
	select {
	case got := <-result:
		parts := strings.SplitN(got, "||", 2)
		if len(parts) != 2 || parts[0] != "application/x-www-form-urlencoded;charset=UTF-8|a=1&b=hello+world" {
			t.Fatalf("unexpected URLSearchParams encoding %q", got)
		}
		for _, want := range []string{"multipart/form-data; boundary=", `name="name"`, "seal", `name="file"; filename="note.txt"`, "file-body"} {
			if !strings.Contains(parts[1], want) {
				t.Fatalf("multipart body missing %q: %s", want, parts[1])
			}
		}
	case <-time.After(time.Second):
		t.Fatal("body encoding result timed out")
	}
}

func TestFetchWithoutTransportRejects(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}
	result := make(chan string, 1)
	if !loop.Schedule(func(ctx *quickjs.Context) error {
		if err := InstallGlobal(ctx); err != nil {
			return err
		}
		report := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			if len(args) > 0 {
				result <- args[0].ToString()
			}
			return ctx.NewUndefined()
		})
		ctx.Globals().Set("reportFetch", report)
		value := ctx.Eval(`fetch("https://example.test").then(() => reportFetch("unexpected"), error => reportFetch(error.message))`)
		if value.IsException() {
			return ctx.Exception()
		}
		value.Free()
		return nil
	}) {
		t.Fatal("fetch task was rejected")
	}
	select {
	case got := <-result:
		if !strings.Contains(got, "transport") {
			t.Fatalf("unexpected missing transport error %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("missing transport result timed out")
	}
}

func TestFetchPreAbortedSignalRejects(t *testing.T) {
	called := make(chan struct{}, 1)
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		called <- struct{}{}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("unexpected")),
			Request:    request,
		}, nil
	})
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}
	result := make(chan string, 1)
	if !loop.Schedule(func(ctx *quickjs.Context) error {
		if err := abort.InstallGlobal(ctx); err != nil {
			return err
		}
		if err := InstallGlobal(ctx, WithTransport(transport)); err != nil {
			return err
		}
		report := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			if len(args) > 0 {
				result <- args[0].ToString()
			}
			return ctx.NewUndefined()
		})
		ctx.Globals().Set("reportFetch", report)
		value := ctx.Eval(`(() => { const controller = new AbortController(); controller.abort(); return fetch("https://example.test", {signal: controller.signal}).then(() => reportFetch("unexpected"), error => reportFetch(error.message)); })()`)
		if value.IsException() {
			return ctx.Exception()
		}
		value.Free()
		return nil
	}) {
		t.Fatal("fetch task was rejected")
	}
	select {
	case got := <-result:
		if !strings.Contains(got, "aborted") {
			t.Fatalf("unexpected pre-abort error %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("pre-abort result timed out")
	}
	select {
	case <-called:
		t.Fatal("pre-aborted request reached transport")
	default:
	}
}

func TestFetchAbortCancelsInFlightTransport(t *testing.T) {
	started := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-request.Context().Done()
		cancelled <- struct{}{}
		return nil, request.Context().Err()
	})
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}
	result := make(chan string, 1)
	if !loop.Schedule(func(ctx *quickjs.Context) error {
		if err := abort.InstallGlobal(ctx); err != nil {
			return err
		}
		if err := InstallGlobal(ctx, WithTransport(transport)); err != nil {
			return err
		}
		report := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			if len(args) > 0 {
				result <- args[0].ToString()
			}
			return ctx.NewUndefined()
		})
		ctx.Globals().Set("reportFetch", report)
		value := ctx.Eval(`(() => {
			const controller = new AbortController();
			setTimeout(() => controller.abort(), 1);
			return fetch("https://example.test", {signal: controller.signal})
				.then(() => reportFetch("unexpected"), error => reportFetch(error.message));
		})()`)
		if value == nil {
			return &testError{"abort evaluation returned nil"}
		}
		if value.IsException() {
			return ctx.Exception()
		}
		value.Free()
		return nil
	}) {
		t.Fatal("fetch task was rejected")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("transport did not start")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("transport was not cancelled")
	}
	select {
	case got := <-result:
		if !strings.Contains(got, "aborted") {
			t.Fatalf("unexpected abort error %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("abort result timed out")
	}

}

func TestFetchUsesBlobBodyAndReturnsBlob(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/custom"}},
			Body:       io.NopCloser(strings.NewReader(request.Header.Get("Content-Type") + "|" + string(body))),
			Request:    request,
		}, nil
	})
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}

	result := make(chan string, 1)
	if !loop.Schedule(func(ctx *quickjs.Context) error {
		if err := InstallGlobal(ctx, WithTransport(transport)); err != nil {
			return err
		}
		report := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			if len(args) > 0 {
				result <- args[0].ToString()
			}
			return ctx.NewUndefined()
		})
		ctx.Globals().Set("reportFetch", report)
		value := ctx.Eval(`(async () => {
			try {
				const response = await fetch("https://example.test", {method: "POST", body: new Blob(["blob body"], {type: "text/plain"})});
				const body = await response.blob();
				reportFetch([Object.prototype.toString.call(body), body.type, await body.text()].join("|"));
			} catch (error) {
				reportFetch(error.name + ": " + error.message);
			}
		})()`)
		if value == nil {
			return &testError{"Blob fetch evaluation returned nil"}
		}
		if value.IsException() {
			return ctx.Exception()
		}
		value.Free()
		return nil
	}) {
		t.Fatal("fetch task was rejected")
	}
	select {
	case got := <-result:
		if want := "[object Blob]|application/custom|text/plain|blob body"; got != want {
			t.Fatalf("Blob fetch result = %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("Blob fetch result timed out")
	}
}

func TestHeadersPreservesSetCookieRecords(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	var result string
	if err := loop.Run(func(ctx *quickjs.Context) error {
		if err := InstallGlobal(ctx); err != nil {
			return err
		}
		value := ctx.Eval(`(() => {
			const headers = new Headers();
			headers.append("Set-Cookie", "a=1");
			headers.append("Set-Cookie", "b=2");
			return [headers.get("set-cookie"), headers.getSetCookie().join(";"), Array.from(headers).map(entry => entry.join(":")).join(";")].join("|");
		})()`)
		if value == nil {
			return &testError{"Headers evaluation returned nil"}
		}
		defer value.Free()
		if value.IsException() {
			return ctx.Exception()
		}
		result = value.ToString()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := result, "a=1, b=2|a=1;b=2|set-cookie:a=1;set-cookie:b=2"; got != want {
		t.Fatalf("Headers result = %q, want %q", got, want)
	}
}

func TestHeadersUseLiveIteratorsAndResponseGuard(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	var result string
	if err := loop.Run(func(ctx *quickjs.Context) error {
		if err := InstallGlobal(ctx); err != nil {
			return err
		}
		value := ctx.Eval(`(() => {
			const headers = new Headers([["fizz", "buzz"], ["x-header", "test"]]);
			const iterator = headers.entries();
			const first = iterator.next().value.join(":");
			headers.append("set-cookie", "a=b");
			const second = iterator.next().value.join(":");
			headers.append("accept", "text/html");
			const third = iterator.next().value.join(":");
			const normalized = new Headers({"Set-Cookie": "  a=b\n"});
			normalized.append("set-cookie", "\n\rc=d  ");
			const response = new Response();
			response.headers.append("set-cookie", "blocked=true");
			return [first, second, third, Array.from(normalized).map(entry => entry.join(":")).join(";"), response.headers.getSetCookie().length].join("|");
		})()`)
		if value == nil {
			return &testError{"Headers evaluation returned nil"}
		}
		defer value.Free()
		if value.IsException() {
			return ctx.Exception()
		}
		result = value.ToString()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := result, "fizz:buzz|set-cookie:a=b|set-cookie:a=b|set-cookie:a=b;set-cookie:c=d|0"; got != want {
		t.Fatalf("Headers result = %q, want %q", got, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type testError struct{ message string }

func (e *testError) Error() string { return e.message }
