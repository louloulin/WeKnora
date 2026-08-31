/**
 * v0.7.28 — DOC image binary embedding smoke test.
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import JSZip from 'jszip'
import {
  buildBlankDocxDoc,
  collectImagesFromPmDoc,
  embedImagesInDocx,
  saveDocxBytes,
  saveDocxBytesWithImages,
} from '../docxAdapter'

const TINY_PNG_B64 =
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII='

test('collectImagesFromPmDoc assigns filenames to data-URL image nodes', async () => {
  const pmDoc: any = {
    type: 'doc',
    content: [
      {
        type: 'image',
        attrs: { src: `data:image/png;base64,${TINY_PNG_B64}`, alt: 'tiny', width: 50, height: 50 },
        content: [],
      },
    ],
  }
  const assets = collectImagesFromPmDoc(pmDoc)
  assert.equal(assets.length, 1)
  assert.ok(assets[0].filename.startsWith('media/image'))
  assert.ok(assets[0].filename.endsWith('.png'))
  assert.equal(pmDoc.content[0].attrs.src, assets[0].filename)
})

test('saveDocxBytesWithImages embeds PNG bytes + content-type overrides', async () => {
  const doc = await buildBlankDocxDoc([{ text: 'hi' }])
  const pmDoc: any = {
    type: 'doc',
    content: [
      {
        type: 'image',
        attrs: { src: `data:image/png;base64,${TINY_PNG_B64}`, alt: 'tiny', width: 50, height: 50 },
        content: [],
      },
    ],
  }
  const bytes = await saveDocxBytesWithImages(doc, pmDoc)
  const zip = await JSZip.loadAsync(bytes)
  const mediaFile = zip.file('word/media/image1.png')
  assert.ok(mediaFile, 'image part should be written to word/media/image1.png')
  const mediaBytes = await mediaFile.async('uint8array')
  assert.ok(mediaBytes.byteLength > 0)
  const rels = await zip.file('word/_rels/document.xml.rels').async('string')
  assert.match(rels, /Type="http:\/\/schemas\.openxmlformats\.org\/officeDocument\/2006\/relationships\/image"/)
  assert.match(rels, /Target="media\/image1\.png"/)
  const ct = await zip.file('[Content_Types].xml').async('string')
  assert.match(ct, /Extension="png"/)
  assert.match(ct, /PartName="\/word\/media\/image1\.png"/)
})

test('embedImagesInDocx is a no-op when given no assets', async () => {
  const doc = await buildBlankDocxDoc([{ text: 'hi' }])
  const baseBytes = await saveDocxBytes(doc)
  const out = await embedImagesInDocx(baseBytes, [])
  assert.equal(out.byteLength, baseBytes.byteLength)
})
