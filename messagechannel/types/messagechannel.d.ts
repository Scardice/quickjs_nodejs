declare module "messagechannel" {
  export interface MessageEvent<T = unknown> {
    readonly type: "message";
    readonly data: T;
    readonly ports: MessagePort[];
    readonly target: MessagePort;
    readonly currentTarget: MessagePort;
  }


  export interface MessageErrorEvent {
    readonly type: "messageerror";
    readonly target: MessagePort;
    readonly currentTarget: MessagePort;
  }
  export type MessageListener<T = unknown> = (event: MessageEvent<T>) => void;

  export interface MessagePortOptions {
    transfer?: Iterable<MessagePort>;
  }

  export class MessagePort {
    private constructor();
    onmessage: MessageListener | null;
    onmessageerror: ((event: MessageErrorEvent) => void) | null;
    postMessage<T>(value: T, transfer?: Iterable<MessagePort> | MessagePortOptions): void;
    start(): void;
    close(): void;
    addEventListener<T>(type: "message", listener: MessageListener<T>): void;
    removeEventListener<T>(type: "message", listener: MessageListener<T>): void;
  }

  export class MessageChannel {
    readonly port1: MessagePort;
    readonly port2: MessagePort;
  }

  const defaultExport: {
    MessageChannel: typeof MessageChannel;
    MessagePort: typeof MessagePort;
  };
  export default defaultExport;
}

declare module "node:messagechannel" {
  export * from "messagechannel";
  export { default } from "messagechannel";
}

