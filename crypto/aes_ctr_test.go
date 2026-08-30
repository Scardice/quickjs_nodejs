package crypto

import "testing"

func TestWebCryptoRejectsExhaustedAESCTRCounter(t *testing.T) {
	result := runWebCrypto(t, `(async () => {
		const key = await crypto.subtle.importKey(
			"raw", new Uint8Array(16), "AES-CTR", false, ["encrypt"]);
		try {
			await crypto.subtle.encrypt(
				{name: "AES-CTR", counter: new Uint8Array(16), length: 1},
				key,
				new Uint8Array(48));
			return "accepted";
		} catch (_) {
			return "rejected";
		}
	})()`)
	if result != "rejected" {
		t.Fatalf("AES-CTR exhausted counter result = %q, want rejected", result)
	}
}

func TestWebCryptoAESCTRMatchesNISTVector(t *testing.T) {
	result := runWebCrypto(t, `(async () => {
		const bytes = hex => new Uint8Array(hex.match(/../g).map(value => parseInt(value, 16)));
		const key = await crypto.subtle.importKey(
			"raw", bytes("2b7e151628aed2a6abf7158809cf4f3c"), "AES-CTR", false, ["encrypt"]);
		const ciphertext = await crypto.subtle.encrypt(
			{name: "AES-CTR", counter: bytes("f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff"), length: 128},
			key,
			bytes("6bc1bee22e409f96e93d7e117393172aae2d8a571e03ac9c9eb76fac45af8e5130c81c46a35ce411e5fbc1191a0a52eff69f2445df4f9b17ad2b417be66c3710"));
		return Array.from(new Uint8Array(ciphertext)).map(value => value.toString(16).padStart(2, "0")).join("");
	})()`)
	want := "874d6191b620e3261bef6864990db6ce9806f66b7970fdff8617187bb9fffdff5ae4df3edbd5d35e5b4f09020db03eab1e031dda2fbe03d1792170a0f3009cee"
	if result != want {
		t.Fatalf("AES-CTR ciphertext = %q, want %q", result, want)
	}
}
