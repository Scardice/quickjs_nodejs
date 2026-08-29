declare module "util" {
  export function format(format?: unknown, ...args: unknown[]): string;
  export function inspect(value: unknown, options?: { depth?: number | null }): string;

  export interface Types {
    [name: string]: (value: unknown) => boolean;
  }

  export const types: Types;
  export function promisify<T extends (...args: never[]) => unknown>(original: T): (...args: Parameters<T>) => Promise<unknown>;
  export function callbackify<T extends (...args: never[]) => unknown>(original: T): (...args: unknown[]) => void;

  const defaultExport: {
    format: typeof format;
    inspect: typeof inspect;
    types: typeof types;
    promisify: typeof promisify;
    callbackify: typeof callbackify;
  };
  export default defaultExport;
}

declare module "node:util" {
  export * from "util";
  export { default } from "util";
}

declare module "@seal/utilinspect" {
  export * from "util";
  export { default } from "util";
}
