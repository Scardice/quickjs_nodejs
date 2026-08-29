declare module "process" {
  export interface ProcessEnv {
    [key: string]: string | undefined;
  }

  export interface Process {
    env: ProcessEnv;
  }

  export const env: ProcessEnv;
  const defaultExport: Process;
  export default defaultExport;
}

declare module "node:process" {
  export * from "process";
  export { default } from "process";
}
