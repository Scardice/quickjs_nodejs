declare module "blob" {
  export type BlobPart = Blob | ArrayBuffer | ArrayBufferView | string;

  export interface BlobPropertyBag {
    endings?: "transparent" | "native";
    type?: string;
  }

  export class Blob {
    constructor(blobParts?: Iterable<BlobPart>, options?: BlobPropertyBag);
    readonly size: number;
    readonly type: string;
    arrayBuffer(): Promise<ArrayBuffer>;
    bytes(): Promise<Uint8Array>;
    slice(start?: number, end?: number, contentType?: string): Blob;
    text(): Promise<string>;
  }

  export interface FilePropertyBag extends BlobPropertyBag {
    lastModified?: number;
  }

  export class File extends Blob {
    constructor(fileBits: Iterable<BlobPart>, fileName: string, options?: FilePropertyBag);
    readonly lastModified: number;
    readonly name: string;
  }

  const defaultExport: {
    Blob: typeof Blob;
    File: typeof File;
  };
  export default defaultExport;
}

declare module "node:blob" {
  export * from "blob";
  export { default } from "blob";
}

