declare module "fs" {
  export type Encoding = "utf8" | "utf-8";

  export interface Stats {
    readonly size: number;
    readonly mode: number;
    readonly mtimeMs: number;
    readonly isFile: boolean;
    readonly isDirectory: boolean;
    readonly isSymbolicLink: boolean;
  }

  export interface Promises {
    readFile(path: string): Promise<Uint8Array>;
    readFile(path: string, encoding: Encoding): Promise<string>;
    writeFile(path: string, data: string | ArrayBuffer | ArrayBufferView): Promise<void>;
    mkdir(path: string): Promise<void>;
    readdir(path: string): Promise<string[]>;
    stat(path: string): Promise<Stats>;
    lstat(path: string): Promise<Stats>;
    unlink(path: string): Promise<void>;
    rename(source: string, destination: string): Promise<void>;
  }

  export const promises: Promises;

  // These functions are present only when the Go host enables WithSync(true).
  export function readFileSync(path: string): Uint8Array;
  export function readFileSync(path: string, encoding: Encoding): string;
  export function writeFileSync(path: string, data: string | ArrayBuffer | ArrayBufferView): void;
  export function mkdirSync(path: string): void;
  export function readdirSync(path: string): string[];
  export function statSync(path: string): Stats;
  export function lstatSync(path: string): Stats;
  export function unlinkSync(path: string): void;
  export function renameSync(source: string, destination: string): void;

  export interface FileSystem {
    promises: Promises;
    readFileSync: typeof readFileSync;
    writeFileSync: typeof writeFileSync;
    mkdirSync: typeof mkdirSync;
    readdirSync: typeof readdirSync;
    statSync: typeof statSync;
    lstatSync: typeof lstatSync;
    unlinkSync: typeof unlinkSync;
    renameSync: typeof renameSync;
  }

  const defaultExport: FileSystem;
  export default defaultExport;
}

declare module "node:fs" {
  export * from "fs";
  export { default } from "fs";
}

declare module "fs/promises" {
  export type { Encoding, Stats } from "fs";
  export function readFile(path: string): Promise<Uint8Array>;
  export function readFile(path: string, encoding: import("fs").Encoding): Promise<string>;
  export function writeFile(path: string, data: string | ArrayBuffer | ArrayBufferView): Promise<void>;
  export function mkdir(path: string): Promise<void>;
  export function readdir(path: string): Promise<string[]>;
  export function stat(path: string): Promise<import("fs").Stats>;
  export function lstat(path: string): Promise<import("fs").Stats>;
  export function unlink(path: string): Promise<void>;
  export function rename(source: string, destination: string): Promise<void>;

  const defaultExport: {
    readFile: typeof readFile;
    writeFile: typeof writeFile;
    mkdir: typeof mkdir;
    readdir: typeof readdir;
    stat: typeof stat;
    lstat: typeof lstat;
    unlink: typeof unlink;
    rename: typeof rename;
  };
  export default defaultExport;
}

declare module "node:fs/promises" {
  export * from "fs/promises";
  export { default } from "fs/promises";
}
