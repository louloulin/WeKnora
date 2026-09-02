// v0.7.106 — Tiny zip extractor used by docxSavePlanRevisions.test.ts to pull
// word/document.xml out of saveDocxBytes output. Avoids adding a zip dep
// to the test suite.

import { inflateRawSync } from 'node:zlib'

/** Extract the first `word/document.xml` entry from a .docx (zip) buffer. */
export function extractDocxDocumentXml(bytes: Uint8Array): string {
  // Find EOCD (end of central directory) record — last 22 bytes contain a
  // pointer to the central directory. Most .docx files we emit here are
  // small, so we walk back from the end until we find the 0x06054b50 signature.
  const sig = 0x06054b50
  let eocd = -1
  for (let i = bytes.length - 22; i >= 0 && i >= bytes.length - 65557; i--) {
    if (
      bytes[i] === 0x50 &&
      bytes[i + 1] === 0x4b &&
      bytes[i + 2] === 0x05 &&
      bytes[i + 3] === 0x06
    ) {
      eocd = i
      break
    }
  }
  if (eocd < 0) throw new Error('EOCD record not found — not a zip?')
  const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  const cdSize = dv.getUint32(eocd + 12, true)
  const cdOffset = dv.getUint32(eocd + 16, true)
  let p = cdOffset
  const end = cdOffset + cdSize
  while (p < end) {
    if (
      bytes[p] !== 0x50 ||
      bytes[p + 1] !== 0x4b ||
      bytes[p + 2] !== 0x01 ||
      bytes[p + 3] !== 0x02
    ) {
      throw new Error(`bad central dir entry signature at ${p}`)
    }
    const compMethod = dv.getUint16(p + 10, true)
    const compSize = dv.getUint32(p + 20, true)
    const fnameLen = dv.getUint16(p + 28, true)
    const extraLen = dv.getUint16(p + 30, true)
    const commentLen = dv.getUint16(p + 32, true)
    const lhOffset = dv.getUint32(p + 42, true)
    const fname = new TextDecoder().decode(bytes.subarray(p + 46, p + 46 + fnameLen))
    if (fname === 'word/document.xml') {
      // Walk to the local header to grab the file payload directly.
      if (
        bytes[lhOffset] !== 0x50 ||
        bytes[lhOffset + 1] !== 0x4b ||
        bytes[lhOffset + 2] !== 0x03 ||
        bytes[lhOffset + 3] !== 0x04
      ) {
        throw new Error(`bad local header at ${lhOffset}`)
      }
      const lhFnameLen = dv.getUint16(lhOffset + 26, true)
      const lhExtraLen = dv.getUint16(lhOffset + 28, true)
      const dataStart = lhOffset + 30 + lhFnameLen + lhExtraLen
      const raw = bytes.subarray(dataStart, dataStart + compSize)
      if (compMethod === 0) return new TextDecoder('utf-8').decode(raw)
      if (compMethod === 8) return new TextDecoder('utf-8').decode(inflateRawSync(raw))
      throw new Error(`unsupported compression method ${compMethod}`)
    }
    p += 46 + fnameLen + extraLen + commentLen
  }
  throw new Error('word/document.xml not in zip')
}
