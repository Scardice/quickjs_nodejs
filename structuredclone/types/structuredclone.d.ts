declare module "structuredclone" {
  export interface StructuredCloneOptions {
    transfer?: object[];
  }

  export function structuredClone<T>(value: T, options?: StructuredCloneOptions): T;

  const defaultExport: {
    structuredClone: typeof structuredClone;
  };
  export default defaultExport;
}

declare module "node:structuredclone" {
  export * from "structuredclone";
  export { default } from "structuredclone";
}
