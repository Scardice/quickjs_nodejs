declare module "crypto" {
  export type BufferSource = ArrayBuffer | ArrayBufferView;
  export type AlgorithmIdentifier = string | Algorithm;

  export interface Algorithm {
    name: string;
    [key: string]: unknown;
  }

  export interface JsonWebKey {
    kty?: string;
    use?: string;
    key_ops?: string[];
    alg?: string;
    ext?: boolean;
    crv?: string;
    x?: string;
    y?: string;
    d?: string;
    n?: string;
    e?: string;
    p?: string;
    q?: string;
    dp?: string;
    dq?: string;
    qi?: string;
    k?: string;
    [key: string]: unknown;
  }

  export interface CryptoKeyAlgorithm {
    name: string;
    hash?: { name: string } | string;
    namedCurve?: string;
    length?: number;
  }

  export interface CryptoKey {
    readonly type: "secret" | "public" | "private";
    readonly extractable: boolean;
    readonly algorithm: CryptoKeyAlgorithm;
    readonly usages: string[];
  }

  export interface CryptoKeyConstructor {
    new (...args: never[]): CryptoKey;
  }

  export interface CryptoKeyPair {
    publicKey: CryptoKey;
    privateKey: CryptoKey;
  }
  export interface HmacKeyGenParams extends Algorithm {
    hash: AlgorithmIdentifier;
    length?: number;
  }

  export interface AesKeyGenParams extends Algorithm {
    length: 128 | 192 | 256;
  }

  export interface RsaHashedKeyGenParams extends Algorithm {
    modulusLength: number;
    publicExponent: BufferSource;
    hash: AlgorithmIdentifier;
  }

  export interface EcKeyGenParams extends Algorithm {
    namedCurve: string;
  }

  export interface RsaPssParams extends Algorithm {
    saltLength: number;
  }

  export interface RsaOaepParams extends Algorithm {
    label?: BufferSource;
  }

  export interface AesIvParams extends Algorithm {
    iv: BufferSource;
  }

  export interface AesGcmParams extends AesIvParams {
    additionalData?: BufferSource;
    tagLength?: number;
  }

  export interface AesCtrParams extends Algorithm {
    counter: BufferSource;
    length: number;
  }

  export interface Pbkdf2Params extends Algorithm {
    salt: BufferSource;
    iterations: number;
    hash: AlgorithmIdentifier;
  }

  export interface HkdfParams extends Algorithm {
    salt: BufferSource;
    info: BufferSource;
    hash: AlgorithmIdentifier;
  }

  export interface EcdhKeyDeriveParams extends Algorithm {
    public: CryptoKey;
  }

  export interface DerivedKeyAlgorithm extends Algorithm {
    length: number;
    hash?: AlgorithmIdentifier;
  }

  export type KeyUsage =
    | "encrypt"
    | "decrypt"
    | "sign"
    | "verify"
    | "deriveKey"
    | "deriveBits"
    | "wrapKey"
    | "unwrapKey";

  export interface SubtleCrypto {
    digest(algorithm: AlgorithmIdentifier, data: BufferSource): Promise<ArrayBuffer>;
    generateKey(
      algorithm: AlgorithmIdentifier,
      extractable: boolean,
      keyUsages: KeyUsage[],
    ): Promise<CryptoKey | CryptoKeyPair>;
    importKey(
      format: string,
      keyData: BufferSource | JsonWebKey,
      algorithm: AlgorithmIdentifier,
      extractable: boolean,
      keyUsages: KeyUsage[],
    ): Promise<CryptoKey>;
    exportKey(format: string, key: CryptoKey): Promise<ArrayBuffer | JsonWebKey>;
    sign(algorithm: AlgorithmIdentifier, key: CryptoKey, data: BufferSource): Promise<ArrayBuffer>;
    verify(
      algorithm: AlgorithmIdentifier,
      key: CryptoKey,
      signature: BufferSource,
      data: BufferSource,
    ): Promise<boolean>;
    encrypt(algorithm: AlgorithmIdentifier, key: CryptoKey, data: BufferSource): Promise<ArrayBuffer>;
    decrypt(algorithm: AlgorithmIdentifier, key: CryptoKey, data: BufferSource): Promise<ArrayBuffer>;
    deriveBits(algorithm: AlgorithmIdentifier, baseKey: CryptoKey, length: number): Promise<ArrayBuffer>;
    deriveKey(
      algorithm: AlgorithmIdentifier,
      baseKey: CryptoKey,
      derivedKeyAlgorithm: AlgorithmIdentifier,
      extractable: boolean,
      keyUsages: KeyUsage[],
    ): Promise<CryptoKey>;
    wrapKey(format: string, key: CryptoKey, wrappingKey: CryptoKey, wrapAlgorithm: AlgorithmIdentifier): Promise<ArrayBuffer>;
    unwrapKey(
      format: string,
      wrappedKey: BufferSource,
      unwrappingKey: CryptoKey,
      unwrapAlgorithm: AlgorithmIdentifier,
      unwrappedKeyAlgorithm: AlgorithmIdentifier,
      extractable: boolean,
      keyUsages: KeyUsage[],
    ): Promise<CryptoKey>;
  }

  export interface Crypto {
    readonly subtle: SubtleCrypto;
    readonly CryptoKey: CryptoKeyConstructor;
    getRandomValues<T extends ArrayBufferView>(array: T): T;
    randomUUID(): string;
  }

  export const subtle: SubtleCrypto;
  export const CryptoKey: CryptoKeyConstructor;
  export const getRandomValues: Crypto["getRandomValues"];
  export const randomUUID: Crypto["randomUUID"];
  export const webcrypto: Crypto;
  const defaultExport: Crypto;
  export default defaultExport;
}

declare module "node:crypto" {
  export * from "crypto";
  export { default } from "crypto";
}

declare module "@seal/crypto" {
  export * from "crypto";
  export { default } from "crypto";
}
