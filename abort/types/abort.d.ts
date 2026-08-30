declare module "abort" {
  export interface AbortEvent {
    type: "abort";
    target: AbortSignal;
    currentTarget: AbortSignal;
    reason: unknown;
  }

  export type AbortListener = (event: AbortEvent) => void;

  export class AbortSignal {
    readonly aborted: boolean;
    readonly reason: unknown;
    onabort: AbortListener | null;
    addEventListener(type: "abort", listener: AbortListener, options?: { once?: boolean }): void;
    removeEventListener(type: "abort", listener: AbortListener): void;
    throwIfAborted(): void;
    static abort(reason?: unknown): AbortSignal;
    static timeout(milliseconds: number): AbortSignal;
    static any(signals: Iterable<AbortSignal>): AbortSignal;
  }

  export class AbortController {
    readonly signal: AbortSignal;
    abort(reason?: unknown): void;
  }

  const defaultExport: {
    AbortController: typeof AbortController;
    AbortSignal: typeof AbortSignal;
  };
  export default defaultExport;
}

declare module "node:abort" {
  export * from "abort";
  export { default } from "abort";
}
