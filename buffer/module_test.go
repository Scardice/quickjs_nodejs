package buffer

import (
	"testing"

	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

func registerBuffer(t *testing.T, ctx *quickjs.Context) {
	t.Helper()
	registry := module.NewRegistry()
	if err := registry.Add(Module()); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(ctx); err != nil {
		t.Fatal(err)
	}
}

func evalBuffer(t *testing.T, ctx *quickjs.Context, source string) *quickjs.Value {
	t.Helper()
	value := ctx.Eval(`(async () => {
		const m = await import("node:buffer");
		`+source+`
	})()`, quickjs.EvalAwait(true))
	if value == nil {
		t.Fatal("buffer evaluation returned nil")
	}
	if value.IsException() {
		t.Fatalf("buffer evaluation failed: %v", ctx.Exception())
	}
	return value
}

func TestModuleExportsBufferAndCopiesInput(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		registerBuffer(t, ctx)
		result := evalBuffer(t, ctx, `
			const { Buffer } = m;
			const input = new Uint8Array([1, 2, 3]);
			const b = Buffer.from(input);
			const constructed = new Buffer("ok");
			input[0] = 9;
			let error;
			try { Buffer.from("x", "unknown"); } catch (e) { error = e.name + ":" + e.code; }
			return [typeof Buffer, b instanceof Buffer, b instanceof Uint8Array, b.toString("hex"), b[0], constructed.toString(), error].join(":");
		`)
		defer result.Free()
		if got := result.ToString(); got != "function:true:true:010203:1:ok:TypeError:ERR_UNKNOWN_ENCODING" {
			t.Fatalf("unexpected Buffer result %q", got)
		}
	})
}

func TestBufferEncodingAllocationAndNumbers(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		registerBuffer(t, ctx)
		result := evalBuffer(t, ctx, `
			const { Buffer } = m;
			const b = Buffer.alloc(14, "ab");
			b.writeUInt32BE(0x12345678, 0);
			b.writeInt16LE(-2, 4);
			b.writeDoubleLE(1.5, 6);
			return [b.toString("hex"), b.readUInt32BE(0), b.readInt16LE(4), b.readDoubleLE(6), Buffer.from("AQI_", "base64").toString("hex")].join(":");
		`)
		defer result.Free()
		if got := result.ToString(); got != "12345678feff000000000000f83f:305419896:-2:1.5:01023f" {
			t.Fatalf("unexpected Buffer numeric result %q", got)
		}
	})
}

func TestBufferPublicCodecHelpers(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		text := ctx.NewString("AQI_")
		encoding := ctx.NewString("base64")
		decoded, err := DecodeBytes(ctx, text, encoding)
		text.Free()
		encoding.Free()
		if err != nil {
			t.Fatal(err)
		}
		if string(decoded) != string([]byte{1, 2, 63}) {
			t.Fatalf("unexpected decoded bytes %v", decoded)
		}

		encoding = ctx.NewString("hex")
		encoded := EncodeBytes(ctx, []byte{0x01, 0xab}, encoding)
		encoding.Free()
		if encoded == nil {
			t.Fatal("EncodeBytes returned nil")
		}
		if got := encoded.ToString(); got != "01ab" {
			t.Fatalf("unexpected encoded string %q", got)
		}
		encoded.Free()

		buffer := EncodeBytes(ctx, []byte{1, 2, 3}, nil)
		if buffer == nil || !isBuffer(buffer) {
			if buffer != nil {
				buffer.Free()
			}
			t.Fatal("EncodeBytes without encoding did not return a Buffer")
		}
		copied, err := Bytes(ctx, buffer)
		buffer.Free()
		if err != nil {
			t.Fatal(err)
		}
		if string(copied) != string([]byte{1, 2, 3}) {
			t.Fatalf("unexpected copied bytes %v", copied)
		}
	})
}

func TestBufferBigIntBoundsAndAliases(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		registerBuffer(t, ctx)
		result := evalBuffer(t, ctx, `
			const { Buffer } = m;
			const signed = Buffer.alloc(8);
			signed.writeBigInt64BE(9223372036854775807n);
			const unsigned = Buffer.alloc(8);
			unsigned.writeBigUint64LE(18446744073709551615n);
			let code = "";
			try { signed.writeBigInt64BE(9223372036854775808n); } catch (e) { code = e.code; }
			return [signed.readBigInt64BE(), unsigned.readBigUint64LE(), code].join("|");
		`)
		defer result.Free()
		if got := result.ToString(); got != "9223372036854775807|18446744073709551615|ERR_OUT_OF_RANGE" {
			t.Fatalf("unexpected Buffer BigInt result %q", got)
		}
	})
}

func TestBufferGlobalInstaller(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`Buffer.from("ok").toString() + ":" + (typeof process)`, quickjs.EvalAwait(false))
		if result == nil {
			t.Fatal("global evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("global evaluation failed: %v", ctx.Exception())
		}
		if got := result.ToString(); got != "ok:undefined" {
			t.Fatalf("unexpected global result %q", got)
		}
	})
}
