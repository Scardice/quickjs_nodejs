declare module "url" {
  export class URL {
    constructor(input: string, base?: string | URL);
    hash: string;
    host: string;
    hostname: string;
    href: string;
    readonly origin: string;
    password: string;
    pathname: string;
    port: string;
    protocol: string;
    search: string;
    readonly searchParams: URLSearchParams;
    username: string;
    toString(): string;
    toJSON(): string;
  }

  export class URLSearchParams {
    constructor(init?: string | Record<string, string> | Iterable<[string, string]>);
    readonly size: number;
    append(name: string, value: string): void;
    delete(name: string, value?: string): void;
    get(name: string): string | null;
    getAll(name: string): string[];
    has(name: string, value?: string): boolean;
    set(name: string, value: string): void;
    sort(): void;
    toString(): string;
    forEach(callback: (value: string, name: string, searchParams: URLSearchParams) => void, thisArg?: unknown): void;
    keys(): IterableIterator<string>;
    values(): IterableIterator<string>;
    entries(): IterableIterator<[string, string]>;
    [Symbol.iterator](): IterableIterator<[string, string]>;
  }

  export function domainToASCII(domain: string): string;
  export function domainToUnicode(domain: string): string;
  const defaultExport: {
    URL: typeof URL;
    URLSearchParams: typeof URLSearchParams;
    domainToASCII: typeof domainToASCII;
    domainToUnicode: typeof domainToUnicode;
  };
  export default defaultExport;
}

declare module "node:url" {
  export * from "url";
  export { default } from "url";
}
