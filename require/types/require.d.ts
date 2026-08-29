
declare global {
  function require(specifier: string): unknown;

  namespace require {
    function resolve(specifier: string): string;
  }
}

export {};
