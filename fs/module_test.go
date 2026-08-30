package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Scardice/quickjs_nodejs/eventloop"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

func TestPromisesModuleReadsAndWritesAfterPolicyApproval(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rules.txt"), []byte("initial"), 0o600); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var requests []Request
	policy := func(request Request) error {
		mu.Lock()
		defer mu.Unlock()
		requests = append(requests, request)
		return nil
	}
	options := []Option{WithRoot(root), WithPolicy(policy)}

	got := runFSProgram(t, []module.Definition{Module(options...), PromisesModule(options...)}, `async () => {
		const promises = await import("node:fs/promises");
		const before = await promises.readFile("rules.txt", "utf8");
		await promises.writeFile("out.txt", before + "+approved");
		return await promises.readFile("out.txt", "utf8");
	}`)
	if got != "initial+approved" {
		t.Fatalf("fs/promises result = %q", got)
	}

	contents, err := os.ReadFile(filepath.Join(root, "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(contents), "initial+approved"; got != want {
		t.Fatalf("written contents = %q, want %q", got, want)
	}

	mu.Lock()
	defer mu.Unlock()
	if got, want := requests, []Request{
		{Operation: OperationReadFile, Path: "rules.txt"},
		{Operation: OperationWriteFile, Path: "out.txt"},
		{Operation: OperationReadFile, Path: "out.txt"},
	}; len(got) != len(want) {
		t.Fatalf("policy requests = %#v, want %#v", got, want)
	} else {
		for i := range want {
			if got[i].Operation != want[i].Operation || got[i].Path != want[i].Path || got[i].Sync {
				t.Fatalf("policy request %d = %#v, want %#v", i, got[i], want[i])
			}
		}
	}
}

func TestPromisesModuleRejectsTraversalAndSymlinkEscapesBeforePolicy(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(root, "escape.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	var mu sync.Mutex
	var requests []Request
	policy := func(request Request) error {
		mu.Lock()
		defer mu.Unlock()
		requests = append(requests, request)
		return nil
	}
	options := []Option{WithRoot(root), WithPolicy(policy)}

	got := runFSProgram(t, []module.Definition{PromisesModule(options...)}, `async () => {
		const promises = await import("fs/promises");
		const codeFor = async path => {
			try {
				await promises.readFile(path, "utf8");
				return "allowed";
			} catch (error) {
				return error.code;
			}
		};
		return [await codeFor("../outside.txt"), await codeFor("escape.txt")].join("|");
	}`)
	if got != "ERR_FS_ACCESS_DENIED|ERR_FS_ACCESS_DENIED" {
		t.Fatalf("escape result = %q", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 0 {
		t.Fatalf("escaped paths reached policy: %#v", requests)
	}
}

func TestFSModuleControlsSyncSurfaceAndExportsPromises(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rules.txt"), []byte("sync value"), 0o600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var syncRequests []Request
	policy := func(request Request) error {
		if request.Sync {
			mu.Lock()
			syncRequests = append(syncRequests, request)
			mu.Unlock()
		}
		return nil
	}
	options := []Option{WithRoot(root), WithPolicy(policy)}

	withoutSync := runFSProgram(t, []module.Definition{Module(options...)}, `async () => {
		const fs = await import("node:fs");
		return typeof fs.readFileSync + "|" + typeof fs.promises.readFile;
	}`)
	if withoutSync != "undefined|function" {
		t.Fatalf("default fs surface = %q", withoutSync)
	}

	withSync := runFSProgram(t, []module.Definition{Module(append(options, WithSync(true))...)}, `async () => {
		const fs = await import("fs");
		return fs.readFileSync("rules.txt", "utf8");
	}`)
	if withSync != "sync value" {
		t.Fatalf("sync fs result = %q", withSync)
	}

	mu.Lock()
	defer mu.Unlock()
	if got, want := syncRequests, []Request{{Operation: OperationReadFile, Path: "rules.txt", Sync: true}}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("sync policy requests = %#v, want %#v", got, want)
	}
}

func TestPromisesModuleExportsAllSupportedOperations(t *testing.T) {
	options := []Option{WithRoot(t.TempDir()), WithPolicy(func(Request) error { return nil })}
	got := runFSProgram(t, []module.Definition{PromisesModule(options...)}, `async () => {
		const promises = await import("fs/promises");
		return ["readFile", "writeFile", "mkdir", "readdir", "stat", "lstat", "unlink", "rename"].every(name => typeof promises[name] === "function");
	}`)
	if got != "true" {
		t.Fatalf("fs/promises operations are incomplete: %q", got)
	}
}

func TestPromisesModuleManagesDirectoriesMetadataAndEntries(t *testing.T) {
	root := t.TempDir()
	options := []Option{WithRoot(root), WithPolicy(func(Request) error { return nil })}

	got := runFSProgram(t, []module.Definition{PromisesModule(options...)}, `async () => {
		const promises = await import("fs/promises");
		await promises.mkdir("journal");
		await promises.writeFile("journal/entry.txt", "note");
		const entries = await promises.readdir("journal");
		const file = await promises.stat("journal/entry.txt");
		await promises.rename("journal/entry.txt", "journal/renamed.txt");
		const directory = await promises.lstat("journal");
		await promises.unlink("journal/renamed.txt");
		const rootEntries = await promises.readdir(".");
		return [rootEntries.join(","), entries.join(","), file.size, file.isFile, directory.isDirectory, directory.isSymbolicLink].join("|");
	}`)
	if got != "journal|entry.txt|4|true|true|false" {
		t.Fatalf("filesystem operation result = %q", got)
	}
}

func TestPromisesModuleDeniesOperationsWithoutPolicy(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rules.txt"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := runFSProgram(t, []module.Definition{PromisesModule(WithRoot(root))}, `async () => {
		const promises = await import("fs/promises");
		try {
			await promises.readFile("rules.txt", "utf8");
			return "allowed";
		} catch (error) {
			return error.code;
		}
	}`)
	if got != ErrCodeAccessDenied {
		t.Fatalf("missing policy result = %q, want %q", got, ErrCodeAccessDenied)
	}
}

func runFSProgram(t *testing.T, definitions []module.Definition, program string) string {
	t.Helper()
	registry := module.NewRegistry()
	for _, definition := range definitions {
		if err := registry.Add(definition); err != nil {
			t.Fatal(err)
		}
	}

	loop, err := eventloop.New(eventloop.WithRegistry(registry), eventloop.WithModuleImport(false))
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		t.Fatal(err)
	}

	result := make(chan string, 1)
	if !loop.Schedule(func(ctx *quickjs.Context) error {
		report := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			if len(args) == 0 || args[0] == nil {
				result <- ""
				return ctx.NewUndefined()
			}
			result <- args[0].ToString()
			return ctx.NewUndefined()
		})
		ctx.Globals().Set("reportFS", report)
		value := ctx.Eval(fmt.Sprintf(`Promise.resolve((%s)()).then(value => reportFS(String(value)), error => reportFS("error:" + (error.code || "") + ":" + error.message))`, program))
		if value == nil {
			return fmt.Errorf("fs program returned nil")
		}
		defer value.Free()
		if value.IsException() {
			return ctx.Exception()
		}
		return nil
	}) {
		t.Fatal("fs task was rejected")
	}

	select {
	case got := <-result:
		if len(got) >= len("error:") && got[:len("error:")] == "error:" {
			t.Fatal(got)
		}
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("fs program timed out")
		return ""
	}
}
