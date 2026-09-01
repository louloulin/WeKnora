/**
 * Browser polyfills for pptx-engine's Node-only imports.
 *
 * pptx-engine vendors three Node built-ins:
 *   - node:crypto  -> createHash('sha256').update(bytes).digest('hex')
 *                     + randomUUID()
 *   - node:zlib    -> deflateSync(bytes) (for PNG IDAT chunks)
 *
 * We swap them for browser equivalents so the engine can run in the WeKnora
 * frontend. The SHA-256 replacement uses a pure-TS implementation that
 * matches Node's createHash byte-for-byte so unchanged files still
 * round-trip with the same originalHash (the value is only used for change
 * detection, not for security).
 */
import pako from 'pako'

/** Pure-TS SHA-256. Adapted from public-domain reference impl; produces the
 *  same 32-byte digest as Node's `createHash('sha256')`. */
export function sha256Hex(bytes: Uint8Array): string {
  // SHA-256 constants
  const K = new Uint32Array([
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
    0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
    0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
    0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
    0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
    0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
    0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
    0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
  ])

  const H = new Uint32Array([
    0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
  ])

  // Pre-processing
  const ml = bytes.length + 9
  const padLen = ((ml + 63) >> 6) << 6
  const padded = new Uint8Array(padLen)
  padded.set(bytes)
  padded[bytes.length] = 0x80
  const bitLen = bytes.length * 8
  const dv = new DataView(padded.buffer)
  dv.setUint32(padded.length - 4, bitLen >>> 0, false)
  dv.setUint32(padded.length - 8, Math.floor(bitLen / 0x100000000), false)

  const w = new Uint32Array(64)
  for (let i = 0; i < padded.length; i += 64) {
    for (let j = 0; j < 16; j++) {
      w[j] = dv.getUint32(i + j * 4, false)
    }
    for (let j = 16; j < 64; j++) {
      const s0 = rotr(w[j - 15], 7) ^ rotr(w[j - 15], 18) ^ (w[j - 15] >>> 3)
      const s1 = rotr(w[j - 2], 17) ^ rotr(w[j - 2], 19) ^ (w[j - 2] >>> 10)
      w[j] = (w[j - 16] + s0 + w[j - 7] + s1) >>> 0
    }
    let [a, b, c, d, e, f, g, h] = H
    for (let j = 0; j < 64; j++) {
      const S1 = rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25)
      const ch = (e & f) ^ (~e & g)
      const t1 = (h + S1 + ch + K[j] + w[j]) >>> 0
      const S0 = rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22)
      const mj = (a & b) ^ (a & c) ^ (b & c)
      const t2 = (S0 + mj) >>> 0
      h = g
      g = f
      f = e
      e = (d + t1) >>> 0
      d = c
      c = b
      b = a
      a = (t1 + t2) >>> 0
    }
    H[0] = (H[0] + a) >>> 0
    H[1] = (H[1] + b) >>> 0
    H[2] = (H[2] + c) >>> 0
    H[3] = (H[3] + d) >>> 0
    H[4] = (H[4] + e) >>> 0
    H[5] = (H[5] + f) >>> 0
    H[6] = (H[6] + g) >>> 0
    H[7] = (H[7] + h) >>> 0
  }

  let hex = ''
  for (let i = 0; i < 8; i++) {
    hex += H[i].toString(16).padStart(8, '0')
  }
  return hex
}

function rotr(n: number, b: number): number {
  return ((n >>> b) | (n << (32 - b))) >>> 0
}

/** Sync deflate replacement via pako. Returns the raw bytes (no zlib header). */
export function deflateRawSync(bytes: Uint8Array): Uint8Array {
  return pako.deflateRaw(bytes)
}

/** Sync zlib deflate replacement via pako. Includes the 2-byte zlib header and
 *  adler32 trailer that node:zlib's `deflateSync` produces by default. */
export function deflateSync(bytes: Uint8Array): Uint8Array {
  return pako.deflate(bytes)
}

/** Sync randomUUID replacement via globalThis.crypto. */
export function randomUUID(): string {
  return globalThis.crypto.randomUUID()
}

/** Install the polyfills onto globalThis so the engine's static imports of
 *  `node:crypto`/`node:zlib` resolve to our shims at runtime.
 *
 *  Vite bundles static imports of `node:` modules only when they are reachable
 *  from the entry graph; for the engine modules that still import them we
 *  patch via a side-effecting `globalThis` shim so the engine's
 *  `import { createHash } from 'node:crypto'` line can be redirected by
 *  tsconfig's `paths` to this module (see tsconfig.json "compilerOptions.paths").
 */
export function installPolyfills(): void {
  const g = globalThis as unknown as Record<string, unknown>
  if (!g.crypto) g.crypto = globalThis.crypto
}
