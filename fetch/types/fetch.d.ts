declare module "fetch" {
  export type BodyInit = string | ArrayBuffer | ArrayBufferView | URLSearchParams | FormData | import("blob").Blob;
  export type HeadersInit = Headers | Record<string, string> | Array<[string, string]>;

  export class Headers {
    constructor(init?: HeadersInit);
    append(name: string, value: string): void;
    set(name: string, value: string): void;
    get(name: string): string | null;
    getSetCookie(): string[];
    has(name: string): boolean;
    delete(name: string): void;
    entries(): IterableIterator<[string, string]>;
    keys(): IterableIterator<string>;
    values(): IterableIterator<string>;
    [Symbol.iterator](): IterableIterator<[string, string]>;
    toJSON(): Record<string, string>;
  }

  export interface RequestInit {
    method?: string;
    headers?: HeadersInit;
    body?: BodyInit | null;
    signal?: import("abort").AbortSignal | null;
  }

  export class Request {
    constructor(input: string | Request, init?: RequestInit);
    readonly url: string;
    readonly method: string;
    readonly headers: Headers;
    readonly body: BodyInit | null;
    readonly signal: import("abort").AbortSignal | null;
    clone(): Request;
  }

  export interface ResponseInit {
    status?: number;
    statusText?: string;
    headers?: HeadersInit;
    url?: string;
  }

  export class Response {
    constructor(body?: BodyInit | null, init?: ResponseInit);
    readonly status: number;
    readonly statusText: string;
    readonly headers: Headers;
    readonly url: string;
    readonly ok: boolean;
    readonly redirected: boolean;
    readonly type: string;
    readonly bodyUsed: boolean;
    clone(): Response;
    arrayBuffer(): Promise<ArrayBuffer>;
    text(): Promise<string>;
    json(): Promise<unknown>;
    blob(): Promise<import("blob").Blob>;
  }

  export class FormData {
    append(name: string, value: unknown, filename?: string): void;
    set(name: string, value: unknown, filename?: string): void;
    get(name: string): unknown | null;
    getAll(name: string): unknown[];
    has(name: string): boolean;
    delete(name: string): void;
    entries(): Array<[string, unknown]>;
  }

  export function fetch(input: string | Request, init?: RequestInit): Promise<Response>;

  const defaultExport: {
    fetch: typeof fetch;
    Headers: typeof Headers;
    Request: typeof Request;
    Response: typeof Response;
    FormData: typeof FormData;
  };
  export default defaultExport;
}

declare module "node:fetch" {
  export * from "fetch";
  export { default } from "fetch";
}
