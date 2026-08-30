<div align="center">

# QuickJS Node.js

**为 buke/quickjs-go 提供可组合的 Node.js 与 Web 平台宿主能力**

在 Go 中按需注册 ESM 模块，或显式安装 QuickJS 全局对象。

</div>

<table>
  <tr>
    <td width="50%" valign="top"><p><strong>01 · Event loop</strong></p><p>一个 OS 线程拥有一个 QuickJS runtime、context 与任务调度器。</p></td>
    <td width="50%" valign="top"><p><strong>02 · Explicit surface</strong></p><p>ESM 注册与全局安装分离；只暴露宿主明确允许的能力。</p></td>
  </tr>
  <tr>
    <td width="50%" valign="top"><p><strong>03 · Web primitives</strong></p><p>URL、Fetch、WebCrypto、Blob、AbortController、WebSocket 与 MessageChannel。</p></td>
    <td width="50%" valign="top"><p><strong>04 · Node essentials</strong></p><p>Buffer、console、process、util、受控 fs 与 CommonJS require。</p></td>
  </tr>
</table>

## Install
在目标 Go module 中运行：
```bash
go get github.com/Scardice/quickjs_nodejs
```
## Quickstart
创建 `main.go`。该程序同时验证 `node:url` ESM 导入和显式安装的全局 `URL` 使用同一构造器；运行 `go run main.go` 后输出 `dice.example`。
```go
package main

import (
	"fmt"
	"log"

	"github.com/Scardice/quickjs_nodejs/eventloop"
	"github.com/Scardice/quickjs_nodejs/module"
	urlmodule "github.com/Scardice/quickjs_nodejs/url"
	quickjs "github.com/buke/quickjs-go"
)

func main() {
	registry := module.NewRegistry()
	if err := registry.Add(urlmodule.Module()); err != nil {
		log.Fatal(err)
	}

	loop, err := eventloop.New(
		eventloop.WithRegistry(registry),
		eventloop.WithGlobals(urlmodule.InstallGlobal),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer loop.Close()

	if err := loop.Run(func(ctx *quickjs.Context) error {
		value := ctx.Eval(`(async () => {
			const { URL: ImportedURL } = await import("node:url");
			if (URL !== ImportedURL) throw new Error("URL constructors differ");
			const rollEndpoint = new ImportedURL("https://dice.example/roll?count=2");
			return rollEndpoint.hostname;
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
## Concepts
- **EventLoop 所有权：** `eventloop.New` 将 QuickJS 限制在一个 owner goroutine 与 OS 线程。通过 `Run`、`Do`、`Schedule` 或 `ContextTask` 执行 QuickJS 工作；关闭时调用 `Close`。
- **ESM registry：** 将目标 `module.Definition` 加入 `module.Registry`；本库各包的 `Module()` 返回可注册的定义。注册 ESM 不会自动创建同名全局对象。
- **显式 globals：** 将 `InstallGlobal` 传给 `eventloop.WithGlobals`，才会向 `globalThis` 安装构造器或函数。
- **异步调度：** 事件循环泵送 QuickJS jobs、宿主任务和计时器。`fetch`、`fs/promises`、WebSocket 与 MessageChannel 依赖这个循环继续运行。

## Guides
- [模块接入教程](docs/module-setup.md)：注册 ESM、显式安装 globals、管理 event loop 与 CommonJS。
- [模块参考](docs/module-reference.md)：全部模块的 factory、specifier、JavaScript 导出与宿主边界。
- [受控文件访问](docs/fs.md)：按操作和 root 相对路径授权 `fs`。

## API and integration
### Go 宿主 API
| 目标 | 公开 API | 用途 |
| --- | --- | --- |
| 创建运行时 | `eventloop.New`、`WithRegistry`、`WithGlobals`、`WithModuleImport`、`WithLogger` | 创建一个受 owner 线程约束的 QuickJS 环境。 |
| 执行与调度 | `Run`、`Do`、`Schedule`、`Start`、`Stop`、`Close`、`Reload` | 在循环中运行工作，或从任意 goroutine 排队宿主任务。 |
| 计时器 | `SetTimeout`、`SetInterval`、`SetImmediate`、`ClearTimeout`、`ClearInterval`、`ClearImmediate` | 调度 Go 回调；返回的 handle 也提供 `Cancel` 与 `Canceled`。 |
| ESM 与 CommonJS | `module.NewRegistry`、`Registry.Add`、`Registry.Names`、`Registry.RegisterModule`、`Registry.EnableRequire` | 注册内存 ESM。`EnableRequire` 仅安装 `globalThis.require`，不会隐式开放模块或文件系统。 |
| 上下文适配器 | `ContextTask`、`DoContext`、`RunContext`、`Context` | 在 owner goroutine 上求值、加载 ESM、读取 globals 或绑定 Go 对象。 |
| 二进制与错误 | `buffer.Bytes`、`buffer.DecodeBytes`、`buffer.EncodeBytes`、`buffer.WrapBytes`、`blob.Bytes`、`errors.NewError`、`errors.ThrowTypeError` | 在 Go 与 JavaScript 值之间转换字节，或创建带 Node 错误码的异常。 |

### JavaScript 模块

将所需包的 `Module()` 加入 registry。每个模块也导出 `default`；下表仅列出具名导出和 `InstallGlobal` 安装的全局名称。

| 包 | 模块 specifier | 具名 ESM 导出 | 全局安装 |
| --- | --- | --- | --- |
| `abort` | `abort`、`node:abort` | `AbortController`、`AbortSignal` | `AbortController`、`AbortSignal` |
| `blob` | `blob`、`node:blob` | `Blob`、`File` | `Blob`、`File` |
| `buffer` | `buffer`、`node:buffer` | `Buffer` | `Buffer` |
| `console` | `console`、`node:console` | `console`、`log`、`info`、`debug`、`warn`、`error` | `console` |
| `crypto` | `crypto`、`node:crypto` | `CryptoKey`、`subtle`、`getRandomValues`、`randomUUID`、`webcrypto` | `crypto` |
| `fetch` | `fetch`、`node:fetch` | `fetch`、`Headers`、`Request`、`Response`、`FormData` | 同名五项 |
| `fs` | `fs`、`node:fs` | `promises`；`WithSync(true)` 时额外导出 `*Sync` 方法 | 无 |
| `fs/promises` | `fs/promises`、`node:fs/promises` | `readFile`、`writeFile`、`mkdir`、`readdir`、`stat`、`lstat`、`unlink`、`rename` | 无 |
| `messagechannel` | `messagechannel`、`node:messagechannel` | `MessageChannel`、`MessagePort` | 同名两项 |
| `process` | `process`、`node:process` | `env` | `process` |
| `structuredclone` | `structuredclone`、`node:structuredclone` | `structuredClone` | `structuredClone` |
| `url` | `url`、`node:url` | `URL`、`URLSearchParams`、`domainToASCII`、`domainToUnicode` | `URL`、`URLSearchParams` |
| `util` | `util`、`node:util` | `format`、`inspect`、`types`、`promisify`、`callbackify` | 无 |
| `websocket` | `websocket`、`node:websocket` | `WebSocket`、`CONNECTING`、`OPEN`、`CLOSING`、`CLOSED` | 同名五项 |

### 注入宿主能力

| 包 | 配置 API | 边界 |
| --- | --- | --- |
| `fetch` | `WithTransport(http.RoundTripper)`、`WithPolicy(Policy)` | 未注入 transport 时 `fetch` 返回 rejected promise；policy 在请求发出前执行。 |
| `websocket` | `WithDialer(Dialer)`、`WithHeaders(http.Header)`、`WithPolicy(Policy)` | 未注入 dialer 时不能建立连接；policy 在拨号前执行。 |
| `process` | `WithEnvProvider(EnvProvider)`、`WithEnvSnapshot(map[string]string)` | 只将提供的环境变量写入 `process.env`。 |
| `console` | `ModuleWithPrinter(Printer)`、`InstallGlobalWithPrinter` | 将 JavaScript 日志交给宿主的 `Log`、`Warn`、`Error`。 |
| `fs` | `Module(...)`、`PromisesModule(...)`、`WithRoot(string)`、`WithPolicy(Policy)`、`WithSymlinkPolicy(SymlinkPolicy)`、`WithUnrestrictedAccess()`、`WithSync(bool)` | 默认无 root 或 Policy 时全部拒绝；只接受 root 内相对路径，并拒绝路径穿越及会跟随到 root 外的符号链接。`WithUnrestrictedAccess` 与 root 互斥，仍要求 Policy，并要求 SymlinkPolicy 显式授权遇到的符号链接。Promise Policy 在 worker goroutine 调用。 |

**依赖与类型：** `messagechannel` 会安装所需的 `structuredClone`；`structuredclone` 和 `fetch` 会安装所需的 `Blob` globals。各模块声明位于对应包的 `types/*.d.ts`，现有全局声明汇总在 `global-types/globals.d.ts`。

## Verification
提交前运行：
```bash
go test -race ./... -count=1 && go vet ./...
```

该命令以退出码 `0` 表示测试集通过。`go test` 中每个含测试的包应显示 `ok`；`? [no test files]` 仅表示该包没有测试文件。`-race` 同时检测 Go 数据竞争，`go vet` 必须没有诊断输出。

### Conformance suites
首次运行先取得固定版本的测试向量：
```bash
git submodule update --init --recursive
```

再运行已接入的规范测试：
```bash
go test -tags=conformance -v ./conformance -count=1
```


| 测试集 | 当前接入范围 | 通过判定 |
| --- | --- | --- |
| Test262 | 1 个脚本：`test/language/expressions/addition/bigint-arithmetic.js`。 | 该脚本及其 `sta.js`、`assert.js` harness 均无异常。 |
| WPT URL | `urltestdata.json` 中 1 个成功 vector。 | URL 的 `href` 与 vector 期望值相同。 |
| WPT testharness | 16 个 WPT 脚本；其中 Blob 5 个、Fetch Headers 8 个脚本会断言每个 harness 测例通过，另外 3 个只验证 harness 结果可收集、location 和全局注入。 | 已断言状态的 harness 测例必须全部为 `0`。Blob constructor 中 1 个依赖 `MessageChannel` transfer 的断言明确跳过。 |
| C2SP/Wycheproof | `testdata/wycheproof` 固定版本中的 `aes_gcm_test.json`：316 个 AES-GCM 向量。 | 所有 valid 向量必须同时匹配加密和解密结果；invalid 向量必须拒绝解密。当前 profile 覆盖 229 valid、87 invalid，零跳过。 |


## License

[MIT](./LICENSE)
