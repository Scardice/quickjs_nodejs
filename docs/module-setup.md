# 配置 QuickJS 模块

[首页](../README.md) › 模块接入教程 · [模块参考](module-reference.md) · [受控文件访问](fs.md)

在创建 `eventloop.EventLoop` 前，把需要的 ESM 定义加入 `module.Registry`；只把确实需要的构造器或函数传给 `eventloop.WithGlobals`。ESM 注册不会创建同名全局对象。

本页面面向在 Go 进程中嵌入 JavaScript 的宿主开发者。它演示模块注册、全局安装和事件循环生命周期。各模块的导出、别名和权限选项见[模块参考](module-reference.md)。

## 注册 ESM 模块

先创建 registry，再调用每个包的 `Module()` 并检查 `Add` 的错误。将 registry 传入 `eventloop.WithRegistry` 后，JavaScript 才能使用 `import()`。

创建 `main.go` 并运行 `go run main.go`：

```go
package main

import (
	"fmt"
	"log"

	buffermodule "github.com/Scardice/quickjs_nodejs/buffer"
	"github.com/Scardice/quickjs_nodejs/eventloop"
	"github.com/Scardice/quickjs_nodejs/module"
	urlmodule "github.com/Scardice/quickjs_nodejs/url"
	quickjs "github.com/buke/quickjs-go"
)

func main() {
	registry := module.NewRegistry()
	for _, definition := range []module.Definition{
		urlmodule.Module(),
		buffermodule.Module(),
	} {
		if err := registry.Add(definition); err != nil {
			log.Fatal(err)
		}
	}

	loop, err := eventloop.New(
		eventloop.WithRegistry(registry),
		// Native ESM modules continue to work without QuickJS file imports.
		eventloop.WithModuleImport(false),
		// URL is a deliberate global; Buffer remains ESM-only in this example.
		eventloop.WithGlobals(urlmodule.InstallGlobal),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer loop.Close()

	if err := loop.Run(func(ctx *quickjs.Context) error {
		value := ctx.Eval(`(async () => {
			const { URL: ImportedURL } = await import("node:url");
			const { Buffer } = await import("node:buffer");
			if (URL !== ImportedURL) throw new Error("URL constructors differ");
			return new ImportedURL("https://dice.example/roll").hostname +
				"|" + Buffer.from("dice", "utf8").toString("base64");
		})()`, quickjs.EvalAwait(true))
		if value == nil {
			return fmt.Errorf("javascript evaluation returned nil")
		}
		defer value.Free()
		if value.IsException() {
			return ctx.Exception()
		}
		fmt.Println(value.ToString())
		return nil
	}); err != nil {
		log.Fatal(err)
	}
}
```

程序输出：

```text
dice.example|ZGljZQ==
```

`node:url` 和 `node:buffer` 是别名。使用其他模块时，将参考表中对应的 factory 加入同一个 registry；不要手写别名定义。

## 选择 ESM 或全局对象

`Module()` 只让 JavaScript 使用 `import()`；`InstallGlobal` 才修改 `globalThis`。两者可以同时使用，也可以只选一个。

| JavaScript 调用方式 | Go 配置 | 适用情况 |
| --- | --- | --- |
| `const { URL } = await import("node:url")` | `registry.Add(url.Module())` | 默认选择。依赖在源码中显式声明。 |
| `new URL("https://dice.example")` | `eventloop.WithGlobals(url.InstallGlobal)` | 兼容预期浏览器式全局对象的脚本。 |
| 两者都使用 | 两项配置都添加 | ESM 和全局得到同一类能力，但仍由宿主明确启用。 |

不要因为注册了 ESM 模块就假设存在全局变量。`fetch`、`process`、`WebSocket`、`console` 等全局同样需要各自的 `InstallGlobal`。

## 运行异步 JavaScript

`loop.Run` 适合一次任务：它执行回调并排空事件循环。脚本使用定时器、`fetch`、WebSocket 或 `fs/promises` 后仍会产生后续工作时，先用 `loop.Start()` 启动长期运行的循环，再从任意 goroutine 用 `loop.Schedule(...)` 投递 QuickJS 工作。

只在 `Run`、`Do`、`Schedule`、`ContextTask`、`DoContext` 或 `RunContext` 的回调中访问 `*quickjs.Context` 和 `*quickjs.Value`。事件循环将一个 QuickJS runtime 和 context 绑定到一个 owner goroutine 与 OS 线程。

关闭宿主时调用 `loop.Close()`。它永久释放 runtime、context 和已注册资源；关闭后的 loop 不能重启。

## 安装多个模块

使用一个 registry 聚合能力。下面的片段展示注册过程；完整 factory 和配置参数见[模块参考](module-reference.md)。

```go
registry := module.NewRegistry()
for _, definition := range []module.Definition{
	abort.Module(),
	blob.Module(),
	buffer.Module(),
	console.ModuleWithPrinter(printer),
	crypto.Module(),
	fetch.Module(fetch.WithTransport(transport), fetch.WithPolicy(allowRequest)),
	fs.Module(fs.WithRoot(sandbox), fs.WithPolicy(allowFile)),
	fs.PromisesModule(fs.WithRoot(sandbox), fs.WithPolicy(allowFile)),
	messagechannel.Module(),
	process.Module(process.WithEnvSnapshot(env)),
	structuredclone.Module(),
	url.Module(),
	util.Module(),
	websocket.Module(websocket.WithDialer(dialer), websocket.WithPolicy(allowSocket)),
} {
	if err := registry.Add(definition); err != nil {
		return err
	}
}
```

`fetch`, `fs` 和 `websocket` 不会从宿主自动取得网络或磁盘权限：分别传入 transport、root/Policy 和 dialer。`process` 默认暴露空环境；使用快照或 provider 指定可读变量。

## 使用 CommonJS `require`

CommonJS 是 registry 的一个显式全局功能，不是 ESM 注册的副作用。路径源码加载也默认关闭。

```go
registry := require.NewRegistry(
	require.WithSourceLoader(require.DefaultSourceLoader),
	require.WithBaseDir("./scripts"),
)

loop, err := eventloop.New(
	eventloop.WithRegistry(registry),
	eventloop.WithModuleImport(false),
	eventloop.WithGlobals(registry.EnableRequire),
)
```

`require.DefaultSourceLoader` 会读取宿主文件系统。对不可信脚本，实现 `require.SourceLoader`，只返回允许目录和文件的字节；不要把未受约束的宿主路径暴露给脚本。`WithGlobalFolders` 会增加 `node_modules` 搜索根，只有在该搜索范围可信时才配置它。

## 下一步

- 查询每个模块的 factory、别名、全局和 JavaScript 导出：[模块参考](module-reference.md)。
- 将磁盘权限缩小到操作和相对路径：[受控文件访问](fs.md)。
- 从最小配置开始；每增加一个模块或 global，都是额外的 JavaScript 能力。
