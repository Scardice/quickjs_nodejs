package crypto

import (
	"strings"
	"testing"

	"github.com/Scardice/quickjs_nodejs/eventloop"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

func TestWebCryptoBasics(t *testing.T) {
	registry := module.NewRegistry()
	if err := registry.Add(Module()); err != nil {
		t.Fatal(err)
	}
	loop, err := eventloop.New(eventloop.WithRegistry(registry))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()

	var result string
	if err := loop.Run(func(ctx *quickjs.Context) error {
		if err := InstallGlobal(ctx); err != nil {
			return err
		}
		value := ctx.Eval(`(async () => {
			const random = new Uint8Array(8);
			crypto.getRandomValues(random);
			const uuid = crypto.randomUUID();
			const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode("abc"));
			const key = await crypto.subtle.generateKey({name: "HMAC", hash: "SHA-256", length: 256}, true, ["sign", "verify"]);
			const signature = await crypto.subtle.sign("HMAC", key, new TextEncoder().encode("message"));
			const verified = await crypto.subtle.verify("HMAC", key, signature, new TextEncoder().encode("message"));
			const raw = await crypto.subtle.exportKey("raw", key);
			const imported = await crypto.subtle.importKey("raw", raw, {name: "HMAC", hash: "SHA-256"}, false, ["sign", "verify"]);
			const aes = await crypto.subtle.generateKey({name: "AES-GCM", length: 128}, true, ["encrypt", "decrypt"]);
			const iv = crypto.getRandomValues(new Uint8Array(12));
			const encrypted = await crypto.subtle.encrypt({name: "AES-GCM", iv}, aes, new TextEncoder().encode("hello"));
			const decrypted = await crypto.subtle.decrypt({name: "AES-GCM", iv}, aes, encrypted);
			return [
				Array.from(new Uint8Array(digest)).map(v => v.toString(16).padStart(2, "0")).join(""),
				verified,
				new Uint8Array(signature).byteLength,
				new Uint8Array(raw).byteLength,
				await crypto.subtle.verify("HMAC", imported, signature, new TextEncoder().encode("message")),
				new TextDecoder().decode(decrypted),
				random.some(v => v !== 0),
				/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(uuid)
			].join("|");
		})()`, quickjs.EvalAwait(true))
		if value == nil {
			return &testError{"crypto evaluation returned nil"}
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
	parts := strings.Split(result, "|")
	if len(parts) != 8 {
		t.Fatalf("unexpected crypto result %q", result)
	}
	if parts[0] != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("unexpected digest %q", parts[0])
	}
	if parts[1] != "true" || parts[4] != "true" || parts[5] != "hello" || parts[6] != "true" || parts[7] != "true" {
		t.Fatalf("unexpected crypto result %q", result)
	}
	if parts[2] != "32" || parts[3] != "32" {
		t.Fatalf("unexpected HMAC sizes %q", result)
	}
}
func TestWebCryptoDeriveAndWrap(t *testing.T) {
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
		value := ctx.Eval(`(async () => {
			const base = await crypto.subtle.importKey("raw", new Uint8Array([112,97,115,115,119,111,114,100]), "PBKDF2", false, ["deriveBits"]);
			const bits = await crypto.subtle.deriveBits({name:"PBKDF2", salt:new Uint8Array([115,97,108,116]), iterations:2, hash:"SHA-256"}, base, 128);
			const wrapping = await crypto.subtle.importKey("raw", new Uint8Array(16), "AES-KW", true, ["wrapKey", "unwrapKey"]);
			const original = await crypto.subtle.importKey("raw", new Uint8Array([1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]), {name:"HMAC", hash:"SHA-256"}, true, ["sign"]);
			const wrapped = await crypto.subtle.wrapKey("raw", original, wrapping, "AES-KW");
			const restored = await crypto.subtle.unwrapKey("raw", wrapped, wrapping, "AES-KW", {name:"HMAC", hash:"SHA-256"}, true, ["sign"]);
			const raw = await crypto.subtle.exportKey("raw", restored);
			return [Array.from(new Uint8Array(bits)).map(v => v.toString(16).padStart(2, "0")).join(""), new Uint8Array(wrapped).byteLength, Array.from(new Uint8Array(raw)).join(",")].join("|");
		})()`, quickjs.EvalAwait(true))
		if value == nil {
			return &testError{"derive evaluation returned nil"}
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
	parts := strings.Split(result, "|")
	if len(parts) != 3 || parts[0] != "ae4d0c95af6b46d32d0adff928f06dd0" || parts[1] != "24" || parts[2] != "1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16" {
		t.Fatalf("unexpected derive/wrap result %q", result)
	}
}

func TestWebCryptoAsymmetricKeysAndFormats(t *testing.T) {
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
		value := ctx.Eval(`(async () => {
			const rsa = await crypto.subtle.generateKey(
				{name:"RSASSA-PKCS1-v1_5", modulusLength:1024, publicExponent:new Uint8Array([1,0,1]), hash:"SHA-256"},
				true, ["sign", "verify"]);
			const data = new TextEncoder().encode("rsa");
			const rsaSignature = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", rsa.privateKey, data);
			const rsaVerified = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", rsa.publicKey, rsaSignature, data);
			const rsaJWK = await crypto.subtle.exportKey("jwk", rsa.publicKey);
			const rsaImported = await crypto.subtle.importKey("jwk", rsaJWK,
				{name:"RSASSA-PKCS1-v1_5", hash:"SHA-256"}, false, ["verify"]);
			const rsaImportedVerified = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", rsaImported, rsaSignature, data);
			const ed = await crypto.subtle.generateKey("Ed25519", true, ["sign", "verify"]);
			const edSignature = await crypto.subtle.sign("Ed25519", ed.privateKey, data);
			const edVerified = await crypto.subtle.verify("Ed25519", ed.publicKey, edSignature, data);
			return [rsaVerified, rsaImportedVerified, edVerified, rsaSignature.byteLength, edSignature.byteLength, rsaJWK.kty].join("|");
		})()`, quickjs.EvalAwait(true))
		if value == nil {
			return &testError{"asymmetric evaluation returned nil"}
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
	parts := strings.Split(result, "|")
	if len(parts) != 6 || parts[0] != "true" || parts[1] != "true" || parts[2] != "true" || parts[3] != "128" || parts[4] != "64" || parts[5] != "RSA" {
		t.Fatalf("unexpected asymmetric result %q", result)
	}

}

func TestWebCryptoExtendedAsymmetricOperations(t *testing.T) {
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
		value := ctx.Eval(`(async () => {
			const data = new TextEncoder().encode("compatibility");
			const pss = await crypto.subtle.generateKey(
				{name:"RSA-PSS", modulusLength:1024, publicExponent:new Uint8Array([1,0,1]), hash:"SHA-256"},
				true, ["sign", "verify"]);
			const pssSignature = await crypto.subtle.sign({name:"RSA-PSS", saltLength:32}, pss.privateKey, data);
			const pssVerified = await crypto.subtle.verify({name:"RSA-PSS", saltLength:32}, pss.publicKey, pssSignature, data);

			const oaep = await crypto.subtle.generateKey(
				{name:"RSA-OAEP", modulusLength:1024, publicExponent:new Uint8Array([1,0,1]), hash:"SHA-256"},
				true, ["encrypt", "decrypt"]);
			const oaepCiphertext = await crypto.subtle.encrypt({name:"RSA-OAEP", label:new Uint8Array([7])}, oaep.publicKey, data);
			const oaepPlaintext = await crypto.subtle.decrypt({name:"RSA-OAEP", label:new Uint8Array([7])}, oaep.privateKey, oaepCiphertext);
			const oaepSPKI = await crypto.subtle.exportKey("spki", oaep.publicKey);
			const oaepImported = await crypto.subtle.importKey("spki", oaepSPKI, {name:"RSA-OAEP", hash:"SHA-256"}, false, ["encrypt"]);
			const oaepRoundTrip = await crypto.subtle.encrypt("RSA-OAEP", oaepImported, data);

			const ecdsa = await crypto.subtle.generateKey({name:"ECDSA", namedCurve:"P-256", hash:"SHA-256"}, true, ["sign", "verify"]);
			const ecdsaSignature = await crypto.subtle.sign({name:"ECDSA", hash:"SHA-256"}, ecdsa.privateKey, data);
			const ecdsaVerified = await crypto.subtle.verify({name:"ECDSA", hash:"SHA-256"}, ecdsa.publicKey, ecdsaSignature, data);
			const ecdsaJWK = await crypto.subtle.exportKey("jwk", ecdsa.publicKey);
			const ecdsaImported = await crypto.subtle.importKey("jwk", ecdsaJWK, {name:"ECDSA", namedCurve:"P-256"}, false, ["verify"]);
			const ecdsaImportedVerified = await crypto.subtle.verify({name:"ECDSA", hash:"SHA-256"}, ecdsaImported, ecdsaSignature, data);
			const ecdsaPKCS8 = await crypto.subtle.exportKey("pkcs8", ecdsa.privateKey);
			const ecdsaPrivateImported = await crypto.subtle.importKey("pkcs8", ecdsaPKCS8, {name:"ECDSA", namedCurve:"P-256"}, false, ["sign"]);
			const ecdsaDERVerified = await crypto.subtle.verify({name:"ECDSA", hash:"SHA-256"}, ecdsa.publicKey, await crypto.subtle.sign({name:"ECDSA", hash:"SHA-256"}, ecdsaPrivateImported, data), data);

			const ecdhA = await crypto.subtle.generateKey({name:"ECDH", namedCurve:"P-256"}, true, ["deriveBits"]);
			const ecdhB = await crypto.subtle.generateKey({name:"ECDH", namedCurve:"P-256"}, true, ["deriveBits"]);
			const ecdhASecret = await crypto.subtle.deriveBits({name:"ECDH", public:ecdhB.publicKey}, ecdhA.privateKey, 256);
			const ecdhBSecret = await crypto.subtle.deriveBits({name:"ECDH", public:ecdhA.publicKey}, ecdhB.privateKey, 256);
			const ecdhSame = Array.from(new Uint8Array(ecdhASecret)).join(",") === Array.from(new Uint8Array(ecdhBSecret)).join(",");

			const xA = await crypto.subtle.generateKey("X25519", true, ["deriveBits"]);
			const xB = await crypto.subtle.generateKey("X25519", true, ["deriveBits"]);
			const xASecret = await crypto.subtle.deriveBits({name:"X25519", public:xB.publicKey}, xA.privateKey, 256);
			const xBSecret = await crypto.subtle.deriveBits({name:"X25519", public:xA.publicKey}, xB.privateKey, 256);
			const xSame = Array.from(new Uint8Array(xASecret)).join(",") === Array.from(new Uint8Array(xBSecret)).join(",");

			return [
				pssVerified,
				new TextDecoder().decode(oaepPlaintext) === "compatibility",
				oaepRoundTrip.byteLength > 0,
				ecdsaVerified,
				ecdsaImportedVerified,
				ecdsaDERVerified,
				ecdhSame,
				xSame,
				ecdsaSignature.byteLength
			].join("|");
		})()`, quickjs.EvalAwait(true))
		if value == nil {
			return &testError{"extended asymmetric evaluation returned nil"}
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
	parts := strings.Split(result, "|")
	if len(parts) != 9 || parts[0] != "true" || parts[1] != "true" || parts[2] != "true" || parts[3] != "true" || parts[4] != "true" || parts[5] != "true" || parts[6] != "true" || parts[7] != "true" || parts[8] != "64" {
		t.Fatalf("unexpected extended asymmetric result %q", result)
	}
}

func TestWebCryptoWrapAndUnwrapKeyFormats(t *testing.T) {
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
		value := ctx.Eval(`(async () => {
			const rsa = await crypto.subtle.generateKey(
				{name:"RSA-OAEP", modulusLength:1024, publicExponent:new Uint8Array([1,0,1]), hash:"SHA-256"},
				true, ["wrapKey", "unwrapKey"]);
			const original = await crypto.subtle.generateKey({name:"AES-GCM", length:128}, true, ["encrypt", "decrypt"]);
			const wrappedRaw = await crypto.subtle.wrapKey("raw", original, rsa.publicKey, "RSA-OAEP");
			const restoredRaw = await crypto.subtle.unwrapKey(
				"raw", wrappedRaw, rsa.privateKey, "RSA-OAEP",
				{name:"AES-GCM", length:128}, true, ["encrypt", "decrypt"]);
			const originalRaw = await crypto.subtle.exportKey("raw", original);
			const restoredRawBytes = await crypto.subtle.exportKey("raw", restoredRaw);

			const signing = await crypto.subtle.generateKey(
				{name:"RSASSA-PKCS1-v1_5", modulusLength:1024, publicExponent:new Uint8Array([1,0,1]), hash:"SHA-256"},
				true, ["sign", "verify"]);
			const aes = await crypto.subtle.generateKey({name:"AES-GCM", length:128}, true, ["encrypt", "decrypt", "wrapKey", "unwrapKey"]);
			const iv = new Uint8Array(12);
			const wrappedJWK = await crypto.subtle.wrapKey("jwk", signing.privateKey, aes, {name:"AES-GCM", iv});
			const restoredJWK = await crypto.subtle.unwrapKey(
				"jwk", wrappedJWK, aes, {name:"AES-GCM", iv},
				{name:"RSASSA-PKCS1-v1_5", hash:"SHA-256"}, true, ["sign"]);
			const data = new TextEncoder().encode("wrapped");
			const signature = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", restoredJWK, data);
			const verified = await crypto.subtle.verify("RSASSA-PKCS1-v1_5", signing.publicKey, signature, data);
			return [
				Array.from(new Uint8Array(originalRaw)).join(",") === Array.from(new Uint8Array(restoredRawBytes)).join(","),
				verified,
				wrappedRaw.byteLength,
				wrappedJWK.byteLength > 0
			].join("|");
		})()`, quickjs.EvalAwait(true))
		if value == nil {
			return &testError{"wrap evaluation returned nil"}
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
	parts := strings.Split(result, "|")
	if len(parts) != 4 || parts[0] != "true" || parts[1] != "true" || parts[2] != "128" || parts[3] != "true" {
		t.Fatalf("unexpected wrap result %q", result)
	}
}

type testError struct{ message string }

func TestWebCryptoCustomRSAExponent(t *testing.T) {
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
		value := ctx.Eval(`(async () => {
			const pair = await crypto.subtle.generateKey(
				{name:"RSASSA-PKCS1-v1_5", modulusLength:512, publicExponent:new Uint8Array([3]), hash:"SHA-256"},
				true, ["sign", "verify"]);
			const data = new TextEncoder().encode("custom exponent");
			const signature = await crypto.subtle.sign("RSASSA-PKCS1-v1_5", pair.privateKey, data);
			return await crypto.subtle.verify("RSASSA-PKCS1-v1_5", pair.publicKey, signature, data);
		})()`, quickjs.EvalAwait(true))
		if value == nil {
			return &testError{"custom RSA evaluation returned nil"}
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
	if result != "true" {
		t.Fatalf("custom RSA verification = %q, want true", result)
	}
}
func (e *testError) Error() string { return e.message }
