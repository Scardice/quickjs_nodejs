//go:build conformance

package conformance

import (
	"encoding/json"
	"fmt"
	"strings"

	cryptomodule "github.com/Scardice/quickjs_nodejs/crypto"
	"github.com/Scardice/quickjs_nodejs/eventloop"
	quickjs "github.com/buke/quickjs-go"
)

type wycheproofAESGCMFile struct {
	Algorithm     string                  `json:"algorithm"`
	NumberOfTests int                     `json:"numberOfTests"`
	TestGroups    []wycheproofAESGCMGroup `json:"testGroups"`
}

type wycheproofAESGCMGroup struct {
	TagSize int                      `json:"tagSize"`
	Tests   []wycheproofAESGCMVector `json:"tests"`
}

type wycheproofAESGCMVector struct {
	TCID   int    `json:"tcId"`
	Key    string `json:"key"`
	IV     string `json:"iv"`
	AAD    string `json:"aad"`
	Msg    string `json:"msg"`
	CT     string `json:"ct"`
	Tag    string `json:"tag"`
	Result string `json:"result"`
}

type wycheproofAESGCMFailure struct {
	TCID   int    `json:"tcId"`
	Result string `json:"result"`
	Reason string `json:"reason"`
}

type wycheproofAESGCMFailures []wycheproofAESGCMFailure

func (failures wycheproofAESGCMFailures) String() string {
	parts := make([]string, len(failures))
	for index, failure := range failures {
		parts[index] = fmt.Sprintf("tcId=%d result=%s: %s", failure.TCID, failure.Result, failure.Reason)
	}
	return strings.Join(parts, "; ")
}

type wycheproofAESGCMReport struct {
	Tested     int                      `json:"tested"`
	Valid      int                      `json:"valid"`
	Invalid    int                      `json:"invalid"`
	Acceptable int                      `json:"acceptable"`
	Skipped    int                      `json:"skipped"`
	Failures   wycheproofAESGCMFailures `json:"failures"`
}

func runWycheproofAESGCM(data []byte) (wycheproofAESGCMReport, error) {
	var vectors wycheproofAESGCMFile
	if err := json.Unmarshal(data, &vectors); err != nil {
		return wycheproofAESGCMReport{}, fmt.Errorf("decode Wycheproof AES-GCM vectors: %w", err)
	}
	if vectors.Algorithm != "AES-GCM" {
		return wycheproofAESGCMReport{}, fmt.Errorf("unexpected Wycheproof algorithm %q", vectors.Algorithm)
	}

	groups := make([]wycheproofAESGCMGroup, 0, len(vectors.TestGroups))
	skipped := 0
	for _, group := range vectors.TestGroups {
		if group.TagSize != 128 {
			skipped += len(group.Tests)
			continue
		}
		groups = append(groups, group)
	}
	encodedGroups, err := json.Marshal(groups)
	if err != nil {
		return wycheproofAESGCMReport{}, fmt.Errorf("encode Wycheproof AES-GCM vectors: %w", err)
	}

	loop, err := eventloop.New()
	if err != nil {
		return wycheproofAESGCMReport{}, err
	}
	defer loop.Close()

	report := wycheproofAESGCMReport{Skipped: skipped}
	err = loop.Run(func(ctx *quickjs.Context) error {
		if err := cryptomodule.InstallGlobal(ctx); err != nil {
			return err
		}
		value := ctx.Eval(fmt.Sprintf(`(async () => {
			const groups = %s;
			const report = { tested: 0, valid: 0, invalid: 0, acceptable: 0, failures: [] };
			const bytes = hex => {
				if (hex.length %% 2 !== 0) throw new Error("invalid hexadecimal input");
				const value = new Uint8Array(hex.length / 2);
				for (let index = 0; index < value.length; index++) value[index] = Number.parseInt(hex.slice(index * 2, index * 2 + 2), 16);
				return value;
			};
			const join = (left, right) => {
				const value = new Uint8Array(left.length + right.length);
				value.set(left);
				value.set(right, left.length);
				return value;
			};
			const equal = (left, right) => left.length === right.length && left.every((byte, index) => byte === right[index]);
			const fail = (test, reason) => report.failures.push({ tcId: test.tcId, result: test.result, reason });

			for (const group of groups) {
				for (const test of group.tests) {
					report.tested++;
					const ciphertext = join(bytes(test.ct), bytes(test.tag));
					const algorithm = { name: "AES-GCM", iv: bytes(test.iv), additionalData: bytes(test.aad), tagLength: group.tagSize };
					try {
						const key = await crypto.subtle.importKey("raw", bytes(test.key), "AES-GCM", false, ["encrypt", "decrypt"]);
						if (test.result === "valid") {
							const encrypted = new Uint8Array(await crypto.subtle.encrypt(algorithm, key, bytes(test.msg)));
							if (!equal(encrypted, ciphertext)) {
								fail(test, "encryption output differs from vector");
							} else {
								const decrypted = new Uint8Array(await crypto.subtle.decrypt(algorithm, key, ciphertext));
								if (!equal(decrypted, bytes(test.msg))) fail(test, "decryption output differs from vector");
							}
							report.valid++;
							continue;
						}

						let accepted = false;
						try {
							await crypto.subtle.decrypt(algorithm, key, ciphertext);
							accepted = true;
						} catch (_) {}
						if (test.result === "invalid" && accepted) fail(test, "invalid ciphertext decrypted successfully");
						if (test.result === "acceptable") report.acceptable++; else report.invalid++;
					} catch (error) {
						if (test.result === "valid") fail(test, String(error));
						else if (test.result === "acceptable") report.acceptable++;
						else report.invalid++;
					}
				}
			}
			return JSON.stringify(report);
		})()`, encodedGroups), quickjs.EvalAwait(true))
		if value == nil {
			return fmt.Errorf("Wycheproof AES-GCM evaluation returned nil")
		}
		defer value.Free()
		if value.IsException() {
			return ctx.Exception()
		}
		if err := json.Unmarshal([]byte(value.ToString()), &report); err != nil {
			return fmt.Errorf("decode Wycheproof AES-GCM report: %w", err)
		}
		return nil
	})
	if err != nil {
		return wycheproofAESGCMReport{}, err
	}
	if report.Tested+report.Skipped != vectors.NumberOfTests {
		return wycheproofAESGCMReport{}, fmt.Errorf("Wycheproof AES-GCM accounting mismatch: tested=%d skipped=%d declared=%d", report.Tested, report.Skipped, vectors.NumberOfTests)
	}
	return report, nil
}
