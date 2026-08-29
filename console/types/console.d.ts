declare module "console" {
  export interface Console {
    log(...args: unknown[]): void;
    info(...args: unknown[]): void;
    debug(...args: unknown[]): void;
    warn(...args: unknown[]): void;
    error(...args: unknown[]): void;
  }

  export const console: Console;
  export const log: Console["log"];
  export const info: Console["info"];
  export const debug: Console["debug"];
  export const warn: Console["warn"];
  export const error: Console["error"];
  const defaultExport: Console;
  export default defaultExport;
}

declare module "node:console" {
  export * from "console";
  export { default } from "console";
}
