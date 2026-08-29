declare module "buffer" {
  export type BufferEncoding = "utf8" | "utf-8" | "hex" | "base64" | "base64url" | "base64Url";

  export interface Buffer extends Uint8Array {
    toString(encoding?: BufferEncoding, start?: number, end?: number): string;
    write(string: string, offset?: number, length?: number, encoding?: BufferEncoding): number;
    equals(otherBuffer: Uint8Array): boolean;

    readBigInt64BE(offset?: number): bigint;
    readBigInt64LE(offset?: number): bigint;
    readBigUInt64BE(offset?: number): bigint;
    readBigUInt64LE(offset?: number): bigint;
    writeBigInt64BE(value: bigint, offset?: number): number;
    writeBigInt64LE(value: bigint, offset?: number): number;
    writeBigUInt64BE(value: bigint, offset?: number): number;
    writeBigUInt64LE(value: bigint, offset?: number): number;
    readBigUint64BE(offset?: number): bigint;
    readBigUint64LE(offset?: number): bigint;

    readDoubleBE(offset?: number): number;
    readDoubleLE(offset?: number): number;
    readFloatBE(offset?: number): number;
    readFloatLE(offset?: number): number;
    readInt8(offset?: number): number;
    readInt16BE(offset?: number): number;
    readInt16LE(offset?: number): number;
    readInt32BE(offset?: number): number;
    readInt32LE(offset?: number): number;
    readIntBE(offset?: number, byteLength?: number): number;
    readIntLE(offset?: number, byteLength?: number): number;
    readUInt8(offset?: number): number;
    readUInt16BE(offset?: number): number;
    readUInt16LE(offset?: number): number;
    readUInt32BE(offset?: number): number;
    readUInt32LE(offset?: number): number;
    readUIntBE(offset?: number, byteLength?: number): number;
    readUIntLE(offset?: number, byteLength?: number): number;
    readUint8(offset?: number): number;
    readUint16BE(offset?: number): number;
    readUint16LE(offset?: number): number;
    readUint32BE(offset?: number): number;
    readUint32LE(offset?: number): number;
    readUintBE(offset?: number, byteLength?: number): number;
    readUintLE(offset?: number, byteLength?: number): number;

    writeDoubleBE(value: number, offset?: number): number;
    writeDoubleLE(value: number, offset?: number): number;
    writeFloatBE(value: number, offset?: number): number;
    writeFloatLE(value: number, offset?: number): number;
    writeInt8(value: number, offset?: number): number;
    writeInt16BE(value: number, offset?: number): number;
    writeInt16LE(value: number, offset?: number): number;
    writeInt32BE(value: number, offset?: number): number;
    writeInt32LE(value: number, offset?: number): number;
    writeIntBE(value: number, offset?: number, byteLength?: number): number;
    writeIntLE(value: number, offset?: number, byteLength?: number): number;
    writeUInt8(value: number, offset?: number): number;
    writeUInt16BE(value: number, offset?: number): number;
    writeUInt16LE(value: number, offset?: number): number;
    writeUInt32BE(value: number, offset?: number): number;
    writeUInt32LE(value: number, offset?: number): number;
    writeUIntBE(value: number, offset?: number, byteLength?: number): number;
    writeUIntLE(value: number, offset?: number, byteLength?: number): number;
    writeBigUint64BE(value: bigint, offset?: number): number;
    writeBigUint64LE(value: bigint, offset?: number): number;
    writeUint8(value: number, offset?: number): number;
    writeUint16BE(value: number, offset?: number): number;
    writeUint16LE(value: number, offset?: number): number;
    writeUint32BE(value: number, offset?: number): number;
    writeUint32LE(value: number, offset?: number): number;
    writeUintBE(value: number, offset?: number, byteLength?: number): number;
    writeUintLE(value: number, offset?: number, byteLength?: number): number;
  }

  export interface BufferConstructor {
    new (value: string | ArrayLike<number> | ArrayBuffer | Uint8Array, encoding?: BufferEncoding): Buffer;
    from(value: string | ArrayLike<number> | ArrayBuffer | Uint8Array, encoding?: BufferEncoding): Buffer;
    alloc(size: number, fill?: string | number, encoding?: BufferEncoding): Buffer;
    isBuffer(value: unknown): value is Buffer;
    byteLength(value: string | ArrayBuffer | Uint8Array, encoding?: BufferEncoding): number;
  }

  export const Buffer: BufferConstructor;
  const defaultExport: { Buffer: BufferConstructor };
  export default defaultExport;
}

declare module "node:buffer" {
  export * from "buffer";
  export { default } from "buffer";
}
