package crypto

import "testing"

func TestWebCryptoRejectsInvalidKeyUsages(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "duplicate usage",
			source: `(async () => {
				try {
					await crypto.subtle.generateKey({name: "AES-GCM", length: 128}, false, ["encrypt", "encrypt"]);
					return "accepted";
				} catch (_) { return "rejected"; }
			})()`,
		},
		{
			name: "unsupported AES usage",
			source: `(async () => {
				try {
					await crypto.subtle.generateKey({name: "AES-GCM", length: 128}, false, ["sign"]);
					return "accepted";
				} catch (_) { return "rejected"; }
			})()`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if result := runWebCrypto(t, test.source); result != "rejected" {
				t.Fatalf("invalid key usage result = %q, want rejected", result)
			}
		})
	}
}

func TestWebCryptoHMACUsesKeyBoundHash(t *testing.T) {
	result := runWebCrypto(t, `(async () => {
		const key = await crypto.subtle.importKey(
			"raw", new Uint8Array(16), {name: "HMAC", hash: "SHA-256"}, false, ["sign"]);
		const signature = await crypto.subtle.sign(
			{name: "HMAC", hash: "SHA-1"}, key, new Uint8Array([1, 2, 3]));
		return String(signature.byteLength);
	})()`)
	if result != "32" {
		t.Fatalf("HMAC signature length = %q, want 32", result)
	}
}

func TestWebCryptoReportsOperationError(t *testing.T) {
	result := runWebCrypto(t, `(async () => {
		const key = await crypto.subtle.importKey(
			"raw", new Uint8Array(16), "AES-CTR", false, ["encrypt"]);
		try {
			await crypto.subtle.encrypt(
				{name: "AES-CTR", counter: new Uint8Array(16), length: 0},
				key,
				new Uint8Array());
			return "accepted";
		} catch (error) {
			return error.name;
		}
	})()`)
	if result != "OperationError" {
		t.Fatalf("invalid AES-CTR error name = %q, want OperationError", result)
	}
}

func TestWebCryptoGetRandomValuesFillsIntegerTypedArrays(t *testing.T) {
	result := runWebCrypto(t, `(async () => {
		const values = new Uint16Array(2);
		try {
			crypto.getRandomValues(values);
			return values.byteLength + ":" + Array.from(new Uint8Array(values.buffer)).some(value => value !== 0);
		} catch (_) {
			return "rejected";
		}
	})()`)
	if result != "4:true" {
		t.Fatalf("Uint16Array random result = %q, want 4:true", result)
	}
}

func TestWebCryptoKeyPropertiesAreReadOnly(t *testing.T) {
	result := runWebCrypto(t, `(async () => {
		const key = await crypto.subtle.importKey(
			"raw", new Uint8Array(16), "AES-GCM", false, ["encrypt"]);
		key.extractable = true;
		const descriptor = Object.getOwnPropertyDescriptor(key, "extractable");
		return String(key.extractable) + ":" + descriptor.writable + ":" + descriptor.configurable + ":" + Object.isFrozen(key.usages);
	})()`)
	if result != "false:false:false:true" {
		t.Fatalf("CryptoKey property descriptor = %q, want false:false:false:true", result)
	}
}

func TestWebCryptoRejectsNonStringKeyUsage(t *testing.T) {
	result := runWebCrypto(t, `(async () => {
		try {
			await crypto.subtle.importKey(
				"raw", new Uint8Array(16), "AES-GCM", false, ["encrypt", 1]);
			return "accepted";
		} catch (_) {
			return "rejected";
		}
	})()`)
	if result != "rejected" {
		t.Fatalf("non-string key usage result = %q, want rejected", result)
	}
}

func TestWebCryptoECDSARequiresOperationHash(t *testing.T) {
	result := runWebCrypto(t, `(async () => {
		const pair = await crypto.subtle.generateKey(
			{name: "ECDSA", namedCurve: "P-256"}, false, ["sign", "verify"]);
		const data = new Uint8Array([1]);
		let name;
		try {
			await crypto.subtle.sign("ECDSA", pair.privateKey, data);
			name = "accepted";
		} catch (error) {
			name = error.name;
		}
		return String("hash" in pair.privateKey.algorithm) + ":" + name;
	})()`)
	if result != "false:TypeError" {
		t.Fatalf("ECDSA hash semantics = %q, want false:TypeError", result)
	}
}
