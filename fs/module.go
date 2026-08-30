package fs

import (
	stdErrors "errors"
	"fmt"
	iofs "io/fs"
	"strings"
	"sync/atomic"

	"github.com/Scardice/quickjs_nodejs/buffer"
	nodeerrors "github.com/Scardice/quickjs_nodejs/errors"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

var apiSequence atomic.Uint64

type apiBuilder struct {
	access access
	key    string
}

func newAPIBuilder(config Config) *apiBuilder {
	return &apiBuilder{
		access: newAccess(config),
		key:    fmt.Sprintf("__quickjs_nodejs_fs_api_%d", apiSequence.Add(1)),
	}
}

// Module returns the fs and node:fs ESM definition. It always exports promises
// and exports synchronous methods only when configured with WithSync(true).
func Module(options ...Option) module.Definition {
	builder := newAPIBuilder(applyOptions(options))
	exports := []module.Export{
		{Name: "promises", Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
			return builder.fsExport(ctx, "promises")
		}}},
		{Name: "default", Spec: quickjs.FactorySpec{Factory: builder.fsAPI}},
	}
	if builder.access.config.Sync {
		for _, name := range syncMethodNames {
			name := name
			exports = append(exports, module.Export{Name: name, Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
				return builder.fsExport(ctx, name)
			}}})
		}
	}
	return module.Definition{
		Name:    ModuleName,
		Aliases: []string{"node:fs"},
		Exports: exports,
	}
}

// PromisesModule returns the fs/promises and node:fs/promises ESM definition.
func PromisesModule(options ...Option) module.Definition {
	builder := newAPIBuilder(applyOptions(options))
	exports := make([]module.Export, 0, len(promiseMethodNames)+1)
	for _, name := range promiseMethodNames {
		name := name
		exports = append(exports, module.Export{Name: name, Spec: quickjs.FactorySpec{Factory: func(ctx *quickjs.Context) (*quickjs.Value, error) {
			return builder.promisesExport(ctx, name)
		}}})
	}
	exports = append(exports, module.Export{Name: "default", Spec: quickjs.FactorySpec{Factory: builder.promisesAPI}})
	return module.Definition{
		Name:    PromisesModuleName,
		Aliases: []string{"node:fs/promises"},
		Exports: exports,
	}
}

var promiseMethodNames = []string{
	"readFile",
	"writeFile",
	"mkdir",
	"readdir",
	"stat",
	"lstat",
	"unlink",
	"rename",
}

var syncMethodNames = []string{
	"readFileSync",
	"writeFileSync",
	"mkdirSync",
	"readdirSync",
	"statSync",
	"lstatSync",
	"unlinkSync",
	"renameSync",
}

func (builder *apiBuilder) fsExport(ctx *quickjs.Context, name string) (*quickjs.Value, error) {
	api, err := builder.fsAPI(ctx)
	if err != nil {
		return nil, err
	}
	value := api.Get(name)
	api.Free()
	if value == nil {
		return nil, fmt.Errorf("fs: export %q is unavailable", name)
	}
	return value, nil
}

func (builder *apiBuilder) promisesExport(ctx *quickjs.Context, name string) (*quickjs.Value, error) {
	api, err := builder.promisesAPI(ctx)
	if err != nil {
		return nil, err
	}
	value := api.Get(name)
	api.Free()
	if value == nil {
		return nil, fmt.Errorf("fs/promises: export %q is unavailable", name)
	}
	return value, nil
}

func (builder *apiBuilder) fsAPI(ctx *quickjs.Context) (*quickjs.Value, error) {
	return builder.cachedAPI(ctx, builder.key+"_fs", func() (*quickjs.Value, error) {
		api := ctx.NewObject()
		if api == nil {
			return nil, stdErrors.New("fs: create API")
		}
		promises, err := builder.promisesAPI(ctx)
		if err != nil {
			api.Free()
			return nil, err
		}
		api.Set("promises", promises)
		if builder.access.config.Sync {
			api.Set("readFileSync", ctx.NewFunction(builder.readFileSync))
			api.Set("writeFileSync", ctx.NewFunction(builder.writeFileSync))
			api.Set("mkdirSync", ctx.NewFunction(builder.mkdirSync))
			api.Set("readdirSync", ctx.NewFunction(builder.readdirSync))
			api.Set("statSync", ctx.NewFunction(builder.statSync))
			api.Set("lstatSync", ctx.NewFunction(builder.lstatSync))
			api.Set("unlinkSync", ctx.NewFunction(builder.unlinkSync))
			api.Set("renameSync", ctx.NewFunction(builder.renameSync))
		}
		return api, nil
	})
}

func (builder *apiBuilder) promisesAPI(ctx *quickjs.Context) (*quickjs.Value, error) {
	return builder.cachedAPI(ctx, builder.key+"_promises", func() (*quickjs.Value, error) {
		api := ctx.NewObject()
		if api == nil {
			return nil, stdErrors.New("fs: create promises API")
		}
		api.Set("readFile", ctx.NewFunction(builder.readFile))
		api.Set("writeFile", ctx.NewFunction(builder.writeFile))
		api.Set("mkdir", ctx.NewFunction(builder.mkdir))
		api.Set("readdir", ctx.NewFunction(builder.readdir))
		api.Set("stat", ctx.NewFunction(builder.stat))
		api.Set("lstat", ctx.NewFunction(builder.lstat))
		api.Set("unlink", ctx.NewFunction(builder.unlink))
		api.Set("rename", ctx.NewFunction(builder.rename))
		return api, nil
	})
}

func (builder *apiBuilder) cachedAPI(ctx *quickjs.Context, key string, create func() (*quickjs.Value, error)) (*quickjs.Value, error) {
	globals := ctx.Globals()
	if globals == nil {
		return nil, stdErrors.New("fs: context is closed")
	}
	cached := globals.Get(key)
	if cached != nil && cached.IsObject() {
		return cached, nil
	}
	if cached != nil {
		cached.Free()
	}
	api, err := create()
	if err != nil {
		return nil, err
	}
	globals.Set(key, api)
	return globals.Get(key), nil
}

func (builder *apiBuilder) readFile(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	path, err := builder.requiredPath(args, 0)
	if err != nil {
		return rejectedPromise(ctx, err)
	}
	encoding, err := readEncoding(args, 1)
	if err != nil {
		return rejectedPromise(ctx, err)
	}
	return promise(ctx, func() (fileContents, error) {
		return builder.access.readFile(path, encoding, false)
	}, encodeFileContents)
}

func (builder *apiBuilder) writeFile(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	path, err := builder.requiredPath(args, 0)
	if err != nil {
		return rejectedPromise(ctx, err)
	}
	data, err := requiredData(ctx, args, 1)
	if err != nil {
		return rejectedPromise(ctx, err)
	}
	return promise(ctx, func() (struct{}, error) {
		return struct{}{}, builder.access.writeFile(path, data, false)
	}, encodeUndefined)
}

func (builder *apiBuilder) mkdir(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	path, err := builder.requiredPath(args, 0)
	if err != nil {
		return rejectedPromise(ctx, err)
	}
	return promise(ctx, func() (struct{}, error) {
		return struct{}{}, builder.access.mkdir(path, false)
	}, encodeUndefined)
}

func (builder *apiBuilder) readdir(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	path, err := builder.requiredPath(args, 0)
	if err != nil {
		return rejectedPromise(ctx, err)
	}
	return promise(ctx, func() ([]string, error) {
		return builder.access.readDir(path, false)
	}, encodeStrings)
}

func (builder *apiBuilder) stat(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return builder.fileInfo(ctx, args, true, false)
}

func (builder *apiBuilder) lstat(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return builder.fileInfo(ctx, args, false, false)
}

func (builder *apiBuilder) fileInfo(ctx *quickjs.Context, args []*quickjs.Value, follow, sync bool) *quickjs.Value {
	path, err := builder.requiredPath(args, 0)
	if err != nil {
		if sync {
			return throwFSError(ctx, err)
		}
		return rejectedPromise(ctx, err)
	}
	if sync {
		info, err := builder.access.stat(path, follow, true)
		if err != nil {
			return throwFSError(ctx, err)
		}
		value, err := encodeFileInfo(ctx, info)
		if err != nil {
			return throwFSError(ctx, err)
		}
		return value
	}
	return promise(ctx, func() (iofs.FileInfo, error) {
		return builder.access.stat(path, follow, false)
	}, encodeFileInfo)
}

func (builder *apiBuilder) unlink(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	path, err := builder.requiredPath(args, 0)
	if err != nil {
		return rejectedPromise(ctx, err)
	}
	return promise(ctx, func() (struct{}, error) {
		return struct{}{}, builder.access.unlink(path, false)
	}, encodeUndefined)
}

func (builder *apiBuilder) rename(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	source, err := builder.requiredPath(args, 0)
	if err != nil {
		return rejectedPromise(ctx, err)
	}
	destination, err := builder.requiredPath(args, 1)
	if err != nil {
		return rejectedPromise(ctx, err)
	}
	return promise(ctx, func() (struct{}, error) {
		return struct{}{}, builder.access.rename(source, destination, false)
	}, encodeUndefined)
}

func (builder *apiBuilder) readFileSync(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	path, err := builder.requiredPath(args, 0)
	if err != nil {
		return throwFSError(ctx, err)
	}
	encoding, err := readEncoding(args, 1)
	if err != nil {
		return throwFSError(ctx, err)
	}
	contents, err := builder.access.readFile(path, encoding, true)
	if err != nil {
		return throwFSError(ctx, err)
	}
	value, err := encodeFileContents(ctx, contents)
	if err != nil {
		return throwFSError(ctx, err)
	}
	return value
}

func (builder *apiBuilder) writeFileSync(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	path, err := builder.requiredPath(args, 0)
	if err != nil {
		return throwFSError(ctx, err)
	}
	data, err := requiredData(ctx, args, 1)
	if err != nil {
		return throwFSError(ctx, err)
	}
	if err := builder.access.writeFile(path, data, true); err != nil {
		return throwFSError(ctx, err)
	}
	return ctx.NewUndefined()
}

func (builder *apiBuilder) mkdirSync(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	path, err := builder.requiredPath(args, 0)
	if err != nil {
		return throwFSError(ctx, err)
	}
	if err := builder.access.mkdir(path, true); err != nil {
		return throwFSError(ctx, err)
	}
	return ctx.NewUndefined()
}

func (builder *apiBuilder) readdirSync(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	path, err := builder.requiredPath(args, 0)
	if err != nil {
		return throwFSError(ctx, err)
	}
	names, err := builder.access.readDir(path, true)
	if err != nil {
		return throwFSError(ctx, err)
	}
	value, err := encodeStrings(ctx, names)
	if err != nil {
		return throwFSError(ctx, err)
	}
	return value
}

func (builder *apiBuilder) statSync(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return builder.fileInfo(ctx, args, true, true)
}

func (builder *apiBuilder) lstatSync(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	return builder.fileInfo(ctx, args, false, true)
}

func (builder *apiBuilder) unlinkSync(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	path, err := builder.requiredPath(args, 0)
	if err != nil {
		return throwFSError(ctx, err)
	}
	if err := builder.access.unlink(path, true); err != nil {
		return throwFSError(ctx, err)
	}
	return ctx.NewUndefined()
}

func (builder *apiBuilder) renameSync(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
	source, err := builder.requiredPath(args, 0)
	if err != nil {
		return throwFSError(ctx, err)
	}
	destination, err := builder.requiredPath(args, 1)
	if err != nil {
		return throwFSError(ctx, err)
	}
	if err := builder.access.rename(source, destination, true); err != nil {
		return throwFSError(ctx, err)
	}
	return ctx.NewUndefined()
}

func (builder *apiBuilder) requiredPath(args []*quickjs.Value, index int) (string, error) {
	path, err := requiredPath(args, index)
	if err != nil {
		return "", err
	}
	return builder.access.preparePath(path)
}

func requiredPath(args []*quickjs.Value, index int) (string, error) {
	if index >= len(args) || args[index] == nil || !args[index].IsString() {
		return "", fmt.Errorf("fs: argument %d must be a string path", index+1)
	}
	return args[index].ToString(), nil
}

func requiredData(ctx *quickjs.Context, args []*quickjs.Value, index int) ([]byte, error) {
	if index >= len(args) || args[index] == nil || args[index].IsUndefined() || args[index].IsNull() {
		return nil, fmt.Errorf("fs: argument %d must be string or binary data", index+1)
	}
	return buffer.Bytes(ctx, args[index])
}

func readEncoding(args []*quickjs.Value, index int) (string, error) {
	if index >= len(args) || args[index] == nil || args[index].IsUndefined() || args[index].IsNull() {
		return "", nil
	}
	if !args[index].IsString() {
		return "", fmt.Errorf("fs: encoding must be utf8")
	}
	encoding := strings.ToLower(args[index].ToString())
	if encoding != "utf8" && encoding != "utf-8" {
		return "", fmt.Errorf("fs: encoding %q is unsupported", encoding)
	}
	return encoding, nil
}

func promise[T any](ctx *quickjs.Context, work func() (T, error), encode func(*quickjs.Context, T) (*quickjs.Value, error)) *quickjs.Value {
	return ctx.NewPromise(func(resolve, reject func(*quickjs.Value)) {
		go func() {
			result, err := work()
			ctx.Schedule(func(inner *quickjs.Context) {
				if err != nil {
					rejectFSError(inner, reject, err)
					return
				}
				value, err := encode(inner, result)
				if err != nil {
					rejectFSError(inner, reject, err)
					return
				}
				resolve(value)
				value.Free()
			})
		}()
	})
}

func rejectedPromise(ctx *quickjs.Context, err error) *quickjs.Value {
	return ctx.NewPromise(func(_ func(*quickjs.Value), reject func(*quickjs.Value)) {
		rejectFSError(ctx, reject, err)
	})
}

func rejectFSError(ctx *quickjs.Context, reject func(*quickjs.Value), err error) {
	value := newFSError(ctx, err)
	if value == nil {
		return
	}
	reject(value)
	value.Free()
}

func throwFSError(ctx *quickjs.Context, err error) *quickjs.Value {
	value := newFSError(ctx, err)
	if value == nil {
		return nil
	}
	return ctx.Throw(value)
}

func newFSError(ctx *quickjs.Context, err error) *quickjs.Value {
	if err == nil {
		err = stdErrors.New("fs operation failed")
	}
	return nodeerrors.NewError(ctx, nil, fsErrorCode(err), "%s", err)
}

func fsErrorCode(err error) string {
	var accessErr *accessDeniedError
	if stdErrors.As(err, &accessErr) {
		return ErrCodeAccessDenied
	}
	return ErrCodeIO
}

func encodeFileContents(ctx *quickjs.Context, contents fileContents) (*quickjs.Value, error) {
	if contents.encoding == "" {
		return ctx.NewUint8Array(contents.data), nil
	}
	return ctx.NewString(string(contents.data)), nil
}

func encodeUndefined(ctx *quickjs.Context, _ struct{}) (*quickjs.Value, error) {
	return ctx.NewUndefined(), nil
}

func encodeStrings(ctx *quickjs.Context, names []string) (*quickjs.Value, error) {
	return ctx.Marshal(names)
}

func encodeFileInfo(ctx *quickjs.Context, info iofs.FileInfo) (*quickjs.Value, error) {
	if info == nil {
		return nil, stdErrors.New("fs: stat returned nil file info")
	}
	value := ctx.NewObject()
	if value == nil {
		return nil, stdErrors.New("fs: create stat value")
	}
	value.Set("size", ctx.NewInt64(info.Size()))
	value.Set("mode", ctx.NewInt64(int64(info.Mode())))
	value.Set("mtimeMs", ctx.NewInt64(info.ModTime().UnixMilli()))
	value.Set("isFile", ctx.NewBool(info.Mode().IsRegular()))
	value.Set("isDirectory", ctx.NewBool(info.IsDir()))
	value.Set("isSymbolicLink", ctx.NewBool(info.Mode()&iofs.ModeSymlink != 0))
	return value, nil
}
