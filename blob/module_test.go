package blob

import (
	"testing"

	"github.com/Scardice/quickjs_nodejs/internal/testutil"
	quickjs "github.com/buke/quickjs-go"
)

func TestBlobCopiesPartsSlicesAndReads(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(async () => {
			const source = new Uint8Array([98, 99]);
			const blob = new Blob(["A", source], { type: "Text/Plain" });
			source[0] = 120;
			const bytes = new Uint8Array(await blob.arrayBuffer());
			bytes[0] = 120;
			const slice = blob.slice(1, 3, "Application/JSON");
			return [
				Object.prototype.toString.call(blob),
				blob.size,
				blob.type,
				await blob.text(),
				await slice.text(),
				slice.type,
				await blob.text()
			].join("|");
		})()`, quickjs.EvalAwait(true))
		if result == nil {
			t.Fatal("Blob evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("Blob evaluation failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "[object Blob]|3|text/plain|Abc|bc|application/json|Abc"; got != want {
			t.Fatalf("Blob result = %q, want %q", got, want)
		}
	})
}

func TestBlobSliceTruncatesFractionalOffsets(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(async () => await new Blob(["abcd"]).slice(1.5).text())()`, quickjs.EvalAwait(true))
		if result == nil {
			t.Fatal("Blob slice evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("Blob slice evaluation failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "cd"; got != want {
			t.Fatalf("Blob slice = %q, want %q", got, want)
		}
	})
}

func TestFileExtendsBlobAndRetainsMetadata(t *testing.T) {
	testutil.WithContext(t, func(ctx *quickjs.Context) {
		if err := InstallGlobal(ctx); err != nil {
			t.Fatal(err)
		}
		result := ctx.Eval(`(async () => {
			const file = new File(["report"], "report.txt", { type: "Text/Plain", lastModified: 123 });
			return [
				Object.prototype.toString.call(file),
				file instanceof Blob,
				file.name,
				file.lastModified,
				file.type,
				await file.text()
			].join("|");
		})()`, quickjs.EvalAwait(true))
		if result == nil {
			t.Fatal("File evaluation returned nil")
		}
		defer result.Free()
		if result.IsException() {
			t.Fatalf("File evaluation failed: %v", ctx.Exception())
		}
		if got, want := result.ToString(), "[object File]|true|report.txt|123|text/plain|report"; got != want {
			t.Fatalf("File result = %q, want %q", got, want)
		}
	})
}
