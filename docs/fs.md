# 给 JavaScript 增加受控文件访问

[首页](../README.md) › [模块接入教程](module-setup.md) › [模块参考](module-reference.md) › 受控文件访问

使用 `fs.WithRoot` 限定文件树，再用 `fs.WithPolicy` 按操作和 root 相对路径授权。没有有效 root 或非 nil Policy 时，`fs` 默认拒绝所有操作；它不会读取宿主的当前目录，也不会安装 `globalThis.fs`。

本页面面向把 JavaScript 当作不完全可信扩展代码的 Go 宿主。它说明如何注册 `fs`、写细粒度 Policy、选择异步或同步 API，以及处理拒绝和 I/O 错误。

## 前置条件

- 创建并管理一个专用 root 目录。只给 JavaScript 需要的文件放入其中。
- 不要让不可信并发进程改名 root、替换其父目录，或在 root 中植入符号链接。路径检查不能消除同一台机器上检查与使用之间的竞争。
- Policy 会在 Promise 操作的 worker goroutine 调用。它必须可并发调用，且不能直接访问 QuickJS context 或 value。
- 文件路径始终相对 root。绝对路径、卷路径、逃逸 `..` 路径，以及会解析到 root 外的符号链接都会在 Policy 前以 `ERR_FS_ACCESS_DENIED` 拒绝。

## 映射宿主虚拟路径

`WithPathResolver` 可在调用 Promise 前、仍位于 QuickJS owner thread 时，将宿主虚拟路径映射成 root 相对路径。它适用于需要先捕获调用方身份的宿主；返回值仍会经过 root、路径穿越、符号链接和 Policy 检查。避免在 resolver 中执行耗时或阻塞 I/O；失败会以 `ERR_FS_ACCESS_DENIED` 拒绝操作。

```go
options := []fs.Option{
	fs.WithRoot(extensionRoot),
	fs.WithPathResolver(resolveExtensionDataURI),
	fs.WithPolicy(authorizeExtensionData),
}
```

Promise 的 `Policy` 依然在 worker goroutine 运行，因此不要把依赖 QuickJS runtime 或瞬时调用上下文的逻辑放进 Policy；在 resolver 中先将该上下文编码到返回的相对路径。

## 注册只读加写入白名单

下面的完整程序只允许读取 `rules.txt`，以及写入 `state/roll.txt`。它注册 `node:fs` 和 `node:fs/promises`，从 JavaScript 写入文件，再由 Go 输出保存结果。

创建 `main.go`，然后运行 `go run main.go`：

```go
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Scardice/quickjs_nodejs/eventloop"
	"github.com/Scardice/quickjs_nodejs/fs"
	"github.com/Scardice/quickjs_nodejs/module"
	quickjs "github.com/buke/quickjs-go"
)

func allowRollState(request fs.Request) error {
	switch request.Operation {
	case fs.OperationReadFile:
		if request.Path == "rules.txt" {
			return nil
		}
	case fs.OperationWriteFile:
		if request.Path == "state/roll.txt" {
			return nil
		}
	}
	return fmt.Errorf("fs operation %q on %q is not allowed", request.Operation, request.Path)
}

func main() {
	root, err := os.MkdirTemp("", "quickjs-fs-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(root)

	if err := os.WriteFile(filepath.Join(root, "rules.txt"), []byte("allow"), 0o600); err != nil {
		log.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "state"), 0o700); err != nil {
		log.Fatal(err)
	}

	options := []fs.Option{
		fs.WithRoot(root),
		fs.WithPolicy(allowRollState),
	}
	registry := module.NewRegistry()
	for _, definition := range []module.Definition{
		fs.Module(options...),
		fs.PromisesModule(options...),
	} {
		if err := registry.Add(definition); err != nil {
			log.Fatal(err)
		}
	}

	loop, err := eventloop.New(
		eventloop.WithRegistry(registry),
		eventloop.WithModuleImport(false),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer loop.Close()
	if err := loop.Start(); err != nil {
		log.Fatal(err)
	}

	finished := make(chan string, 1)
	if !loop.Schedule(func(ctx *quickjs.Context) error {
		report := ctx.NewFunction(func(ctx *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			if len(args) == 0 || args[0] == nil {
				finished <- ""
			} else {
				finished <- args[0].ToString()
			}
			return ctx.NewUndefined()
		})
		ctx.Globals().Set("report", report)

		value := ctx.Eval(`(async () => {
			const { readFile, writeFile } = await import("node:fs/promises");
			const rule = await readFile("rules.txt", "utf8");
			await writeFile("state/roll.txt", rule + ": d20", "utf8");
			return rule;
		})().then(
			value => report(value),
			error => report("error:" + (error.code || "") + ":" + error.message),
		)`)
		if value == nil {
			return fmt.Errorf("javascript evaluation returned nil")
		}
		defer value.Free()
		if value.IsException() {
			return ctx.Exception()
		}
		return nil
	}) {
		log.Fatal("event loop is closed")
	}

	if result := <-finished; result != "allow" {
		log.Fatalf("javascript failed: %s", result)
	}
	written, err := os.ReadFile(filepath.Join(root, "state", "roll.txt"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(written))
}
```

程序输出：

```text
allow: d20
```

在实际宿主中，把临时目录替换为该扩展独占的持久目录。不要用 `"."`、工作目录或包含其他租户数据的目录作为 root。

## 选择模块表面

| JavaScript 需要的 API | 注册方式 | 导出 |
| --- | --- | --- |
| `await import("node:fs")` 与 `fs.promises` | `registry.Add(fs.Module(options...))` | `promises`，以及启用时的同步方法。 |
| `await import("node:fs/promises")` | `registry.Add(fs.PromisesModule(options...))` | 八个 Promise 方法。 |
| 两种 import 都需要 | 同时注册两个 definition。 | 每个 specifier 各自可用。 |

`fs.Module` 和 `fs.PromisesModule` 接收独立的 options。为了让两个 import 表面使用同一个授权规则，把同一份 `[]fs.Option` 展开传入两者，如示例所示。

## 在 Policy 中按请求授权

每次已通过 root 和符号链接检查的操作都会产生一个 `fs.Request`：

| 字段 | 含义 | 示例 |
| --- | --- | --- |
| `Operation` | `readFile`、`writeFile`、`mkdir`、`readdir`、`stat`、`lstat`、`unlink` 或 `rename`。 | `fs.OperationRename` |
| `Path` | 规范化后的 root 相对源路径，使用 `/` 分隔。 | `state/roll.txt` |
| `Destination` | 仅 `rename` 的规范化 root 相对目标路径；其他操作为空。 | `archive/roll.txt` |
| `Sync` | JavaScript 是否调用同步 API。 | `true` |

下面的 Policy 允许读取公开规则、在 `state/` 下创建目录和写入、并只允许在状态目录中改名。其余操作全部返回错误：

```go
func allowState(request fs.Request) error {
	switch request.Operation {
	case fs.OperationReadFile, fs.OperationReadDir, fs.OperationStat, fs.OperationLstat:
		if request.Path == "rules/public.json" || strings.HasPrefix(request.Path, "state/") {
			return nil
		}
	case fs.OperationMkdir, fs.OperationWriteFile, fs.OperationUnlink:
		if strings.HasPrefix(request.Path, "state/") {
			return nil
		}
	case fs.OperationRename:
		if strings.HasPrefix(request.Path, "state/") && strings.HasPrefix(request.Destination, "state/") {
			return nil
		}
	}
	return fmt.Errorf("filesystem request denied: %+v", request)
}
```

Policy 应该从白名单开始。不要根据 JavaScript 提供的原始路径授权；`Request.Path` 和 `Request.Destination` 已经是 root 相对、规范化后的值，正是授权时应比较的字段。

## 保持异步，按需公开同步 API

Promise API 始终可用。`WithSync(true)` 只让 `fs` / `node:fs` 额外导出同步方法；它不会改变 `fs/promises`，也不会绕过 Policy。

```go
syncOptions := append([]fs.Option{}, options...)
syncOptions = append(syncOptions, fs.WithSync(true))
if err := registry.Add(fs.Module(syncOptions...)); err != nil {
	return err
}
```

不要在同一个 registry 中先添加 `fs.Module(options...)` 再添加 `fs.Module(syncOptions...)`；两个 definition 的 canonical name 都是 `fs`，第二次 `Add` 会返回重复模块名错误。选择其中一个表面。

JavaScript 中，默认情况下同步方法不存在：

```js
const fs = await import("node:fs");
typeof fs.readFileSync; // "undefined"
```

启用 `WithSync(true)` 后，以下方法存在并仍会产生 `Request{Sync: true}`：`readFileSync`、`writeFileSync`、`mkdirSync`、`readdirSync`、`statSync`、`lstatSync`、`unlinkSync`、`renameSync`。

同步 I/O 会阻塞 QuickJS owner goroutine。仅为确实需要同步 Node 兼容的短操作启用它；扩展代码的默认接口应使用 `fs/promises`。

## 错误与拒绝

| 情况 | JavaScript 结果 |
| --- | --- |
| 缺少有效 root 或 Policy、绝对路径、路径穿越、root 外符号链接、Policy 返回错误 | rejected Promise 或抛出的同步 Error，`error.code === "ERR_FS_ACCESS_DENIED"`。 |
| 宿主文件系统返回错误 | rejected Promise 或抛出的同步 Error，`error.code === "ERR_FS_IO"`。 |

使用标准 Promise 错误处理；不要从错误消息解析授权结果：

```js
try {
	const { readFile } = await import("node:fs/promises");
	const rules = await readFile("rules/private.json", "utf8");
	return rules;
} catch (error) {
	if (error.code === "ERR_FS_ACCESS_DENIED") {
		return "not authorized";
	}
	throw error;
}
```

## 类型声明

将 `fs/types/fs.d.ts` 加入 TypeScript 工程。声明包含 `fs`、`node:fs`、`fs/promises` 与 `node:fs/promises` 的模块名。声明会列出同步方法，但运行时是否导出它们仍由宿主的 `WithSync(true)` 决定。

## 下一步

- 在[模块参考](module-reference.md)中查看网络、环境变量、日志和 CommonJS 的宿主权限配置。
- 回到[模块接入教程](module-setup.md)了解 ESM、global 和事件循环生命周期。
