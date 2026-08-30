//go:build conformance

package conformance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWycheproofAESGCM(t *testing.T) {
	root, err := suiteRoot("wycheproof")
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := os.ReadFile(filepath.Join(root, "testvectors_v1", "aes_gcm_test.json"))
	if err != nil {
		t.Fatal(err)
	}

	report, err := runWycheproofAESGCM(vectors)
	if err != nil {
		t.Fatal(err)
	}
	if report.Tested == 0 {
		t.Fatal("Wycheproof AES-GCM runner executed no vectors")
	}
	if len(report.Failures) != 0 {
		t.Fatalf("Wycheproof AES-GCM failures: %s", report.Failures)
	}
	t.Logf("Wycheproof AES-GCM: tested=%d valid=%d invalid=%d acceptable=%d skipped=%d", report.Tested, report.Valid, report.Invalid, report.Acceptable, report.Skipped)
}
