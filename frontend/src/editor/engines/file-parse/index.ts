/**
 * file-parse — browser-safe subset.
 *
 * The original genoffice/file-parse also exposes parseFileToText (Electron),
 * docToText (legacy .doc) and pptToText (legacy .ppt), plus a Node-only pdfToText.
 * WeKnora runs in the browser; those helpers depend on `node:fs/promises` /
 * `node:path` / `node:module`, so they are intentionally not re-exported here.
 * The remaining three helpers are pure (Uint8Array → string) and power the
 * KB-ingest path.
 */
export { docxToText } from './docx'
export { pptxToText } from './pptx'
export { xlsxToText } from './xlsx'
