declare module 'pako' {
  export function deflate(input: Uint8Array | string, options?: { level?: number }): Uint8Array
  export function deflateRaw(input: Uint8Array | string, options?: { level?: number }): Uint8Array
  export function inflate(input: Uint8Array): Uint8Array
  export function inflateRaw(input: Uint8Array): Uint8Array
  export function gzip(input: Uint8Array | string): Uint8Array
  export function ungzip(input: Uint8Array): Uint8Array
  const _default: {
    deflate: typeof deflate
    deflateRaw: typeof deflateRaw
    inflate: typeof inflate
    inflateRaw: typeof inflateRaw
    gzip: typeof gzip
    ungzip: typeof ungzip
  }
  export default _default
}
