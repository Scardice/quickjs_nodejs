declare module "websocket" {
  export interface WebSocketEvent {
    type: "open" | "message" | "error" | "close";
    target: WebSocket;
    data?: string | ArrayBuffer | Uint8Array;
    message?: string;
    code?: number;
    reason?: string;
    wasClean?: boolean;
  }

  export type WebSocketListener = (event: WebSocketEvent) => void;

  export class WebSocket {
    constructor(url: string, protocols?: string | string[]);
    readonly url: string;
    readonly protocol: string;
    readonly readyState: number;
    readonly bufferedAmount: number;
    binaryType: "arraybuffer" | "uint8array";
    onopen: WebSocketListener | null;
    onmessage: WebSocketListener | null;
    onerror: WebSocketListener | null;
    onclose: WebSocketListener | null;
    send(data: string | ArrayBuffer | ArrayBufferView): void;
    close(code?: number, reason?: string): void;
    addEventListener(type: "open" | "message" | "error" | "close", listener: WebSocketListener): void;
    removeEventListener(type: "open" | "message" | "error" | "close", listener: WebSocketListener): void;
  }

  export const CONNECTING = 0;
  export const OPEN = 1;
  export const CLOSING = 2;
  export const CLOSED = 3;

  const defaultExport: {
    WebSocket: typeof WebSocket;
    CONNECTING: typeof CONNECTING;
    OPEN: typeof OPEN;
    CLOSING: typeof CLOSING;
    CLOSED: typeof CLOSED;
  };
  export default defaultExport;
}

declare module "node:websocket" {
  export * from "websocket";
  export { default } from "websocket";
}
