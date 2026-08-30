package messagechannel

import (
	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
	"testing"
)

func TestMessageChannelDeliversQueuedMessage(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		setup := ctx.Eval(`(() => {
			const channel = new MessageChannel();
			globalThis.__messageChannelReceived = undefined;
			channel.port1.onmessage = event => {
				globalThis.__messageChannelReceived = event.data.value;
			};
			channel.port2.postMessage({value: "queued"});
			return globalThis.__messageChannelReceived === undefined;
		})()`)
		if setup == nil {
			t.Fatal("message channel setup returned nil")
		}
		defer setup.Free()
		if setup.IsException() {
			t.Fatalf("message channel setup failed: %v", ctx.Exception())
		}
		if !setup.ToBool() {
			t.Fatal("postMessage dispatched synchronously")
		}

		ctx.ProcessJobs()
		result := ctx.Eval(`globalThis.__messageChannelReceived`)
		if result == nil {
			t.Fatal("message channel result returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("message channel result failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "queued"; got != want {
			t.Fatalf("message event data = %q, want %q", got, want)
		}
	})
}

func TestStructuredCloneTransfersMessagePort(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		setup := ctx.Eval(`(() => {
			const channel = new MessageChannel();
			const moved = structuredClone(channel.port2, {transfer: [channel.port2]});
			globalThis.__transferredMessage = undefined;
			channel.port1.onmessage = event => {
				globalThis.__transferredMessage = event.data.value;
			};
			moved.postMessage({value: "moved"});
			let detached = false;
			try {
				channel.port2.postMessage("old port");
			} catch (error) {
				detached = error instanceof TypeError;
			}
			return detached;
		})()`)
		if setup == nil {
			t.Fatal("message port transfer setup returned nil")
		}
		defer setup.Free()
		if setup.IsException() {
			t.Fatalf("message port transfer setup failed: %v", ctx.Exception())
		}
		if !setup.ToBool() {
			t.Fatal("transferred MessagePort remained usable")
		}

		ctx.ProcessJobs()
		result := ctx.Eval(`globalThis.__transferredMessage`)
		if result == nil {
			t.Fatal("transferred MessagePort result returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("transferred MessagePort result failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "moved"; got != want {
			t.Fatalf("transferred MessagePort event data = %q, want %q", got, want)
		}
	})
}

func TestMessageChannelClonesPayloadBeforeDelivery(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		setup := ctx.Eval(`(() => {
			const channel = new MessageChannel();
			globalThis.__clonedMessageValue = undefined;
			channel.port1.onmessage = event => {
				globalThis.__clonedMessageValue = event.data.nested.value;
			};
			const payload = {nested: {value: 1}};
			channel.port2.postMessage(payload);
			payload.nested.value = 2;
		})()`)
		if setup == nil {
			t.Fatal("message clone setup returned nil")
		}
		defer setup.Free()
		if setup.IsException() {
			t.Fatalf("message clone setup failed: %v", ctx.Exception())
		}

		ctx.ProcessJobs()
		result := ctx.Eval(`globalThis.__clonedMessageValue`)
		if result == nil {
			t.Fatal("message clone result returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("message clone result failed: %v", ctx.Exception())
		}
		if got, want := result.ToInt32(), int32(1); got != want {
			t.Fatalf("message payload value = %d, want %d", got, want)
		}
	})
}

func TestMessageChannelTransfersPortsInMessageEvents(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		setup := ctx.Eval(`(() => {
			const outer = new MessageChannel();
			const inner = new MessageChannel();
			globalThis.__transferredPortMessage = undefined;
			inner.port1.onmessage = event => {
				globalThis.__transferredPortMessage = event.data;
			};
			outer.port1.onmessage = event => {
				event.ports[0].postMessage("through transferred port");
			};
			outer.port2.postMessage("handoff", [inner.port2]);
			let detached = false;
			try {
				inner.port2.postMessage("old port");
			} catch (error) {
				detached = error instanceof TypeError;
			}
			return detached;
		})()`)
		if setup == nil {
			t.Fatal("message port transfer-list setup returned nil")
		}
		defer setup.Free()
		if setup.IsException() {
			t.Fatalf("message port transfer-list setup failed: %v", ctx.Exception())
		}
		if !setup.ToBool() {
			t.Fatal("transfer-list MessagePort remained usable")
		}

		ctx.ProcessJobs()
		result := ctx.Eval(`globalThis.__transferredPortMessage`)
		if result == nil {
			t.Fatal("message port transfer-list result returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("message port transfer-list result failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "through transferred port"; got != want {
			t.Fatalf("transferred MessagePort event data = %q, want %q", got, want)
		}
	})
}

func TestMessageChannelQueuesEventListenerMessagesUntilStart(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		setup := ctx.Eval(`(() => {
			const channel = new MessageChannel();
			globalThis.__startedMessage = undefined;
			channel.port1.addEventListener("message", event => {
				globalThis.__startedMessage = event.data;
			});
			channel.port2.postMessage("wait for start");
			globalThis.__startedChannel = channel;
		})()`)
		if setup == nil {
			t.Fatal("message start setup returned nil")
		}
		defer setup.Free()
		if setup.IsException() {
			t.Fatalf("message start setup failed: %v", ctx.Exception())
		}

		ctx.ProcessJobs()
		beforeStart := ctx.Eval(`globalThis.__startedMessage === undefined`)
		if beforeStart == nil {
			t.Fatal("message start precondition returned nil")
		}
		defer beforeStart.Free()
		if beforeStart.IsException() {
			t.Fatalf("message start precondition failed: %v", ctx.Exception())
		}
		if !beforeStart.ToBool() {
			t.Fatal("message listener received a message before start()")
		}

		started := ctx.Eval(`globalThis.__startedChannel.port1.start()`)
		if started == nil {
			t.Fatal("message start returned nil")
		}
		defer started.Free()
		if started.IsException() {
			t.Fatalf("message start failed: %v", ctx.Exception())
		}
		ctx.ProcessJobs()
		result := ctx.Eval(`globalThis.__startedMessage`)
		if result == nil {
			t.Fatal("message start result returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("message start result failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "wait for start"; got != want {
			t.Fatalf("message after start = %q, want %q", got, want)
		}
	})
}

func TestMessageChannelAcceptsTransferOptionsObject(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		setup := ctx.Eval(`(() => {
			const channel = new MessageChannel();
			const transferred = new MessageChannel();
			globalThis.__messagePortCount = -1;
			channel.port1.onmessage = event => {
				globalThis.__messagePortCount = event.ports.length;
			};
			channel.port2.postMessage("handoff", {transfer: [transferred.port2]});
			let detached = false;
			try {
				transferred.port2.postMessage("old port");
			} catch (error) {
				detached = error instanceof TypeError;
			}
			return detached;
		})()`)
		if setup == nil {
			t.Fatal("transfer options setup returned nil")
		}
		defer setup.Free()
		if setup.IsException() {
			t.Fatalf("transfer options setup failed: %v", ctx.Exception())
		}
		if !setup.ToBool() {
			t.Fatal("options-transferred MessagePort remained usable")
		}

		ctx.ProcessJobs()
		result := ctx.Eval(`globalThis.__messagePortCount`)
		if result == nil {
			t.Fatal("transfer options result returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("transfer options result failed: %v", ctx.Exception())
		}
		if got, want := result.ToInt32(), int32(1); got != want {
			t.Fatalf("message event ports length = %d, want %d", got, want)
		}
	})
}

func TestMessagePortInitializesMessageErrorHandler(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`new MessageChannel().port1.onmessageerror === null`)
		if result == nil {
			t.Fatal("messageerror result returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("messageerror evaluation failed: %v", ctx.Exception())
		}
		if !result.ToBool() {
			t.Fatal("MessagePort.onmessageerror did not initialize to null")
		}
	})
}

func TestMessageChannelCloseDropsQueuedMessages(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		setup := ctx.Eval(`(() => {
			const channel = new MessageChannel();
			globalThis.__closedPortMessage = undefined;
			channel.port1.onmessage = event => {
				globalThis.__closedPortMessage = event.data;
			};
			channel.port2.postMessage("discard");
			channel.port1.close();
		})()`)
		if setup == nil {
			t.Fatal("message close setup returned nil")
		}
		defer setup.Free()
		if setup.IsException() {
			t.Fatalf("message close setup failed: %v", ctx.Exception())
		}

		ctx.ProcessJobs()
		result := ctx.Eval(`globalThis.__closedPortMessage === undefined`)
		if result == nil {
			t.Fatal("message close result returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("message close result failed: %v", ctx.Exception())
		}
		if !result.ToBool() {
			t.Fatal("closed MessagePort delivered a queued message")
		}
	})
}

func TestMessageChannelModuleExportsConstructors(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		registry := module.NewRegistry()
		if err := registry.Add(Module()); err != nil {
			t.Fatal(err)
		}
		if err := registry.Register(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(async () => {
			const {MessageChannel, MessagePort} = await import("node:messagechannel");
			const channel = new MessageChannel();
			return channel.port1 instanceof MessagePort && channel.port2 instanceof MessagePort;
		})()`, quickjs.EvalAwait(true))
		if result == nil {
			t.Fatal("messagechannel module result returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("messagechannel module evaluation failed: %v", ctx.Exception())
		}
		if !result.ToBool() {
			t.Fatal("messagechannel module did not export working constructors")
		}
	})
}
