declare global {
  type BufferEncoding = "utf8" | "utf-8" | "hex" | "base64" | "base64url" | "base64Url";

  var Buffer: import("buffer").BufferConstructor;
  var console: import("console").Console;
  var process: import("process").Process;
  var URL: typeof import("url").URL;
  var URLSearchParams: typeof import("url").URLSearchParams;
  var AbortController: typeof import("abort").AbortController;
  var AbortSignal: typeof import("abort").AbortSignal;
  var structuredClone: typeof import("structuredclone").structuredClone;
  var crypto: import("crypto").Crypto;
  var fetch: typeof import("fetch").fetch;
  var Headers: typeof import("fetch").Headers;
  var Request: typeof import("fetch").Request;
  var Response: typeof import("fetch").Response;
  var FormData: typeof import("fetch").FormData;
  var WebSocket: typeof import("websocket").WebSocket;
  function require(specifier: string): unknown;

  namespace require {
    function resolve(specifier: string): string;
  }
}

export {};
