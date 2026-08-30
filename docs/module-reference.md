# 模块参考

[首页](../README.md) › [模块接入教程](module-setup.md) › 模块参考 · [受控文件访问](fs.md)

此参考列出每个可注册模块的 Go factory、JavaScript 导入名、显式全局和宿主边界。先按[模块接入教程](module-setup.md)创建 `module.Registry`，再把本页的 factory 加入 registry。

除 `require` 外，每个 ESM 模块都导出 `default`。本页只列出具名导出。模块别名由 factory 提供；调用 `Registry.Add` 一次即可注册该行的全部 specifier。

## 模块目录

| 包 | factory 与 specifier | 具名导出 | 显式 global | JavaScript 用法 |
| --- | --- | --- | --- | --- |
| `abort` | `abort.Module()`<br>`abort`、`node:abort` | `AbortController`、`AbortSignal` | `AbortController`、`AbortSignal` | `const { AbortSignal } = await import("node:abort"); const signal = AbortSignal.timeout(1_000);` |
| `blob` | `blob.Module()`<br>`blob`、`node:blob` | `Blob`、`File` | `Blob`、`File` | `const { Blob } = await import("node:blob"); const body = new Blob(["roll"]);` |
| `buffer` | `buffer.Module()`<br>`buffer`、`node:buffer` | `Buffer` | `Buffer` | `const { Buffer } = await import("node:buffer"); const text = Buffer.from("roll").toString("utf8");` |
| `console` | `console.Module()` 或 `console.ModuleWithPrinter(printer)`<br>`console`、`node:console` | `console`、`log`、`info`、`debug`、`warn`、`error` | `console` | `const { console: logger } = await import("node:console"); logger.info("roll started");` |
| `crypto` | `crypto.Module()`<br>`crypto`、`node:crypto` | `CryptoKey`、`subtle`、`getRandomValues`、`randomUUID`、`webcrypto` | `crypto` | `const { subtle } = await import("node:crypto"); const hash = await subtle.digest("SHA-256", new TextEncoder().encode("roll"));` |
| `fetch` | `fetch.Module(options...)`<br>`fetch`、`node:fetch` | `fetch`、`Headers`、`Request`、`Response`、`FormData` | 同名五项 | `const { fetch } = await import("node:fetch"); const response = await fetch("https://api.example/roll");` |
| `fs` | `fs.Module(options...)`<br>`fs`、`node:fs` | `promises`；`WithSync(true)` 时额外导出八个 `*Sync` 方法 | 无 | `const fs = await import("node:fs"); const text = await fs.promises.readFile("rules.txt", "utf8");` |
| `fs/promises` | `fs.PromisesModule(options...)`<br>`fs/promises`、`node:fs/promises` | `readFile`、`writeFile`、`mkdir`、`readdir`、`stat`、`lstat`、`unlink`、`rename` | 无 | `const { readFile } = await import("node:fs/promises"); const text = await readFile("rules.txt", "utf8");` |
| `messagechannel` | `messagechannel.Module()`<br>`messagechannel`、`node:messagechannel` | `MessageChannel`、`MessagePort` | 同名两项 | `const { MessageChannel } = await import("node:messagechannel"); const channel = new MessageChannel();` |
| `process` | `process.Module(options...)`<br>`process`、`node:process` | `env` | `process` | `const { env } = await import("node:process"); const mode = env.MODE;` |
| `structuredclone` | `structuredclone.Module()`<br>`structuredclone`、`node:structuredclone` | `structuredClone` | `structuredClone` | `const { structuredClone } = await import("node:structuredclone"); const copy = structuredClone({ roll: 20 });` |
| `url` | `url.Module()`<br>`url`、`node:url` | `URL`、`URLSearchParams`、`domainToASCII`、`domainToUnicode` | `URL`、`URLSearchParams` | `const { URL } = await import("node:url"); const host = new URL("https://dice.example").host;` |
| `util` | `util.Module()` 或 `util.ExtendedModule()`<br>`util`、`node:util` | `format`、`inspect`、`types`、`promisify`、`callbackify` | 无 | `const { format } = await import("node:util"); const message = format("rolled %d", 20);` |
| `websocket` | `websocket.Module(options...)`<br>`websocket`、`node:websocket` | `WebSocket`、`CONNECTING`、`OPEN`、`CLOSING`、`CLOSED` | 同名五项 | `const { WebSocket } = await import("node:websocket"); const socket = new WebSocket("wss://api.example/roll");` |

`fetch`、`fs` 和 `websocket` 的 JavaScript 调用只有在宿主提供对应能力后才会成功。`fetch.Module` 会在需要时安装 `Blob` globals；`structuredclone` 会安装 `Blob` globals；`messagechannel` 会安装 `structuredClone`。不要依赖这些间接安装来组织自己的公开 API；需要全局时仍在 `WithGlobals` 中明确列出。

## 显式全局安装

ESM 与全局对象独立。只想让脚本使用 `import()` 时，不传 global installer。需要浏览器式名称时，将 installer 传给 `eventloop.WithGlobals`：

```go
loop, err := eventloop.New(
	eventloop.WithRegistry(registry),
	eventloop.WithGlobals(
		abort.InstallGlobal,
		blob.InstallGlobal,
		buffer.InstallGlobal,
		console.InstallGlobal,
		crypto.InstallGlobal,
		messagechannel.InstallGlobal,
		structuredclone.InstallGlobal,
		url.InstallGlobal,
	),
)
```

可配置 installer 使用闭包固定其 options。为 ESM factory 和 global installer 构造同一组逻辑配置；否则两个表面可能拥有不同权限：

```go
fetchOptions := []fetch.Option{
	fetch.WithTransport(transport),
	fetch.WithPolicy(allowRequest),
}
processOptions := []process.Option{
	process.WithEnvSnapshot(map[string]string{"MODE": "production"}),
}
websocketOptions := []websocket.Option{
	websocket.WithDialer(dialer),
	websocket.WithPolicy(allowSocket),
}

loop, err := eventloop.New(
	eventloop.WithRegistry(registry),
	eventloop.WithGlobals(
		func(ctx *quickjs.Context) error {
			return fetch.InstallGlobal(ctx, fetchOptions...)
		},
		func(ctx *quickjs.Context) error {
			return process.InstallGlobal(ctx, processOptions...)
		},
		func(ctx *quickjs.Context) error {
			return websocket.InstallGlobal(ctx, websocketOptions...)
		},
	),
)
```

`fs` 与 `util` 没有 global installer。

## 宿主配置与权限边界

| 能力 | 配置 API | 默认行为 | 控制粒度 |
| --- | --- | --- | --- |
| 日志 | `console.ModuleWithPrinter(printer)`、`console.InstallGlobalWithPrinter(ctx, printer)` | 默认 printer 写到标准输出或标准错误。 | `console.Printer` 的 `Log`、`Warn`、`Error` 方法。宿主可将日志写入请求日志、审计流或丢弃。 |
| HTTP | `fetch.WithTransport(http.RoundTripper)`、`fetch.WithPolicy(func(*http.Request) error)` | 没有 transport 时，每个 `fetch` 返回 rejected promise。 | Policy 在发请求前接收完整 `*http.Request`；按 URL、方法、header 或 body 元数据拒绝。transport 决定实际连接、代理、TLS 和超时。 |
| 文件系统 | `fs.WithRoot(string)`、`fs.WithPolicy(func(fs.Request) error)`、`fs.WithSymlinkPolicy(func(fs.Request) error)`、`fs.WithUnrestrictedAccess()`、`fs.WithSync(bool)` | 默认没有有效 root 或非 nil Policy 时，所有操作都以 `ERR_FS_ACCESS_DENIED` 拒绝。非沙箱模式与 root 互斥，且遇到符号链接时要求 SymlinkPolicy。 | Policy 接收操作、规范化路径、rename 目标和同步标志；SymlinkPolicy 只接收遇到符号链接的操作。完整规则与可运行示例见[受控文件访问](fs.md)。 |
| 环境变量 | `process.WithEnvSnapshot(map[string]string)`、`process.WithEnvProvider(func() map[string]string)` | `process.env` 默认为空。 | 快照只公开指定键值；provider 可以为每个 context 返回不同副本。不要把 `os.Environ()` 原样交给不可信脚本。 |
| WebSocket | `websocket.WithDialer(dialer)`、`websocket.WithHeaders(headers)`、`websocket.WithPolicy(func(*url.URL) error)` | 没有 dialer 时，连接失败。 | Policy 在拨号前接收解析后的 URL；headers 为每个连接复制。dialer 决定真实网络路径。 |
| CommonJS 源码 | `require.WithSourceLoader(loader)`、`require.WithPathResolver(resolver)`、`require.WithBaseDir(base)`、`require.WithGlobalFolders(folders...)` | path-backed `require` 默认禁用。 | loader 决定可读文件；resolver 决定 specifier 如何映射；base 和全局目录决定搜索范围。 |

网络 Policy 不是网络栈替代品。用 Policy 做 JavaScript 层请求授权，再让 transport 或 dialer 实施代理、证书、超时和连接级限制。

## `require` 兼容层

`require` 包不提供 ESM `Module()`；它复用 `module.Registry`，再显式把 `require` 函数安装为 global。

```go
registry := require.NewRegistry(
	require.WithSourceLoader(loader),
	require.WithPathResolver(resolver),
	require.WithBaseDir("/srv/quickjs-app"),
)

loop, err := eventloop.New(
	eventloop.WithRegistry(registry),
	eventloop.WithGlobals(registry.EnableRequire),
)
```

`loader` 的签名是 `func(filename string) ([]byte, error)`。当文件不在允许集合中时，返回 `require.ErrModuleNotFound`。`require.DefaultSourceLoader` 是读取普通宿主文件的显式便利函数，不会自动启用，也不会替你实现目录隔离。

注册在同一 registry 中的 ESM 模块可以被 CommonJS `require()` 解析。启用 CommonJS 不会改变 ESM `import()` 的行为，也不会自动开启 QuickJS 的 file-backed ESM loader。

## Go 值转换 helper

| 包 | helper | 用途 |
| --- | --- | --- |
| `buffer` | `Bytes(ctx, value)`、`DecodeBytes(ctx, value, encoding)` | 从 JavaScript string、ArrayBuffer、TypedArray 或 Buffer 复制字节。 |
| `buffer` | `EncodeBytes(ctx, data, encoding)`、`WrapBytes(ctx, data)` | 复制 Go 字节并创建编码字符串或 Buffer。 |
| `blob` | `Bytes(ctx, value)` | 从 JavaScript `Blob` 或 `File` 复制字节。 |
| `errors` | `NewError(ctx, code, message)`、`ThrowTypeError(ctx, message)` | 创建带 Node 风格 `code` 的 JavaScript 异常。 |

这些 helper 与 `*quickjs.Context` 一样只能在事件循环 owner 回调中使用。

## 类型声明

每个模块的 TypeScript 声明都在对应包的 `types/*.d.ts`。将需要的文件纳入你的 TypeScript `tsconfig.json` 的 `include`；全局名称的声明位于 `global-types/globals.d.ts`。声明描述可用 JavaScript 表面，不会替宿主启用模块或权限。

## 相关页面

- [模块接入教程](module-setup.md)：注册 ESM、安装 global、运行 event loop。
- [受控文件访问](fs.md)：按操作和路径授权 `fs`。
