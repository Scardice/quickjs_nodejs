package crypto

import (
	"testing"

	"github.com/Scardice/quickjs_nodejs/eventloop"
	"github.com/Scardice/quickjs_nodejs/limits"
	quickjs "github.com/buke/quickjs-go"
)

func TestWebCryptoRejectsForgedCryptoKey(t *testing.T) {
	result := runWebCrypto(t, `(async () => {
		const key = await crypto.subtle.importKey(
			"raw", new Uint8Array(16), "AES-GCM", false, ["encrypt"]);
		const forged = {};
		Object.defineProperty(forged, "__quickjs_nodejs_crypto_key", {
			value: key.__quickjs_nodejs_crypto_key,
		});
		try {
			await crypto.subtle.encrypt(
				{name: "AES-GCM", iv: new Uint8Array(12)}, forged, new Uint8Array());
			return "accepted";
		} catch (_) {
			return "rejected";
		}
	})()`)
	if result != "rejected" {
		t.Fatalf("forged CryptoKey result = %q, want rejected", result)
	}
}

func TestWebCryptoRejectsVerifyWithoutVerifyUsage(t *testing.T) {
	result := runWebCrypto(t, `(async () => {
		const input = new TextEncoder().encode("usage-boundary");
		const hmac = await crypto.subtle.importKey(
			"raw", new Uint8Array(16), {name: "HMAC", hash: "SHA-256"}, false, ["sign"]);
		const signature = await crypto.subtle.sign("HMAC", hmac, input);
		try {
			await crypto.subtle.verify("HMAC", hmac, signature, input);
			return "accepted";
		} catch (_) {
			return "rejected";
		}
	})()`)
	if result != "rejected" {
		t.Fatalf("verify with sign-only key = %q, want rejected", result)
	}
}

func TestWebCryptoRejectsDeriveBitsWithoutUsage(t *testing.T) {
	result := runWebCrypto(t, `(async () => {
		const pbkdf = await crypto.subtle.importKey(
			"raw", new Uint8Array([112, 97, 115, 115]), "PBKDF2", false, ["deriveKey"]);
		const params = {name: "PBKDF2", salt: new Uint8Array([115, 97, 108, 116]), iterations: 1, hash: "SHA-256"};
		try {
			await crypto.subtle.deriveBits(params, pbkdf, 128);
			return "accepted";
		} catch (_) {
			return "rejected";
		}
	})()`)
	if result != "rejected" {
		t.Fatalf("deriveBits with deriveKey-only key = %q, want rejected", result)
	}
}

func TestWebCryptoRejectsPBKDF2PastConfiguredLimit(t *testing.T) {
	resourceLimits, err := limits.NewRuntime(limits.Config{MaxPBKDF2Iterations: 1, MaxPBKDF2OutputBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	result := runWebCryptoWithOptions(t, []Option{WithResourceLimits(resourceLimits)}, `(async () => {
		const key = await crypto.subtle.importKey("raw", new Uint8Array([112, 97, 115, 115]), "PBKDF2", false, ["deriveBits"]);
		try {
			await crypto.subtle.deriveBits({name: "PBKDF2", salt: new Uint8Array([115, 97, 108, 116]), iterations: 2, hash: "SHA-256"}, key, 128);
			return "accepted";
		} catch (_) {
			return "rejected";
		}
	})()`)
	if result != "rejected" {
		t.Fatalf("over-limit PBKDF2 result = %q, want rejected", result)
	}
}

func runWebCrypto(t *testing.T, source string) string {
	return runWebCryptoWithOptions(t, nil, source)
}

func runWebCryptoWithOptions(t *testing.T, options []Option, source string) string {
	t.Helper()

	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	var result string
	if err := loop.Run(func(ctx *quickjs.Context) error {
		if err := InstallGlobal(ctx, options...); err != nil {
			return err
		}
		value := ctx.Eval(source, quickjs.EvalAwait(true))
		if value == nil {
			return &testError{"WebCrypto evaluation returned nil"}
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
	return result
}
