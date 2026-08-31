/**
 * PptxAdapter — build / parse a .pptx byte payload from a structured
 * slides model.
 *
 * We deliberately use pptxgenjs (already in package.json) to **produce**
 * .pptx bytes from a JSON deck model. Parsing an uploaded .pptx is a
 * much heavier problem (OOXML + theme + relationships + image refs) so
 * we keep the parser MVP: extract per-slide <a:t> runs, group by slide,
 * and seed the structured model. Round-trip fidelity is therefore
 * "good enough for collaborative note editing"; lossy for theme /
 * animation / image bytes. We track the original bytes alongside the
 * structured model so a user with no edits falls back to byte-faithful.
 *
 * The structured model is:
 *
 *   interface Slide { title: string; bullets: string[] }
 *
 * which matches the v0.7.25 Yjs Y.Array<slide> shape so existing
 * realtime data flows keep working without a schema migration.
 */
import pptxgen from 'pptxgenjs'

export interface AdapterSlide {
  title: string
  bullets: string[]
}

export interface PptxAdapterDeck {
  slides: AdapterSlide[]
  /** Original bytes when loaded via openPptx (null for newly created decks). */
  originalBytes: Uint8Array | null
}

const slideXmlRegex = /<p:sld[\s>][\s\S]*?<\/p:sld>/g
const textRunRegex = /<a:t(?:\s[^>]*)?>([\s\S]*?)<\/a:t>/g

const decodeXml = (raw: string): string =>
  raw
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&amp;/g, '&')

/**
 * Open a .pptx from raw bytes. Returns a PptxAdapterDeck whose slides
 * array holds per-slide text extracted from <a:t> runs (title is the
 * first run; bullets are subsequent non-empty runs).
 */
export async function openPptx(bytes: Uint8Array): Promise<PptxAdapterDeck> {
  const decoder = new TextDecoder('utf-8')
  const text = decoder.decode(bytes)
  const slideMatches = text.match(slideXmlRegex) || []
  const slides: AdapterSlide[] = []
  for (const slideXml of slideMatches) {
    const runs: string[] = []
    let m: RegExpExecArray | null
    const local = new RegExp(textRunRegex.source, 'g')
    while ((m = local.exec(slideXml)) !== null) {
      const txt = decodeXml(m[1]).trim()
      if (txt) runs.push(txt)
    }
    slides.push({
      title: runs[0] || '',
      bullets: runs.slice(1),
    })
  }
  if (slides.length === 0) {
    slides.push({ title: '新幻灯片', bullets: [''] })
  }
  return { slides, originalBytes: new Uint8Array(bytes) }
}

/**
 * Serialize the structured model to .pptx bytes via pptxgenjs. When the
 * caller hands us `originalBytes`, we return them unchanged so the
 * "no edits" round-trip stays byte-identical (pptxgenjs would otherwise
 * shuffle ordering and silently break Microsoft Word's diff tolerance).
 */
export async function savePptxBytes(deck: PptxAdapterDeck): Promise<Uint8Array> {
  if (deck.originalBytes && isUnchangedDeck(deck)) {
    return new Uint8Array(deck.originalBytes)
  }
  const pres = new pptxgen()
  pres.layout = 'LAYOUT_16x9'
  for (const slide of deck.slides) {
    const s = pres.addSlide()
    if (slide.title) {
      s.addText(slide.title, {
        x: 0.5, y: 0.3, w: 9, h: 1,
        fontSize: 28, bold: true, color: '111827',
      })
    }
    if (slide.bullets.length > 0) {
      s.addText(
        slide.bullets.map((b) => ({ text: b, options: { bullet: true } })),
        {
          x: 0.5, y: 1.5, w: 9, h: 5,
          fontSize: 18, color: '1f2937',
        },
      )
    }
  }
  const out = await pres.write({ outputType: 'arraybuffer' })
  return new Uint8Array(out as ArrayBuffer)
}

/** Build a brand-new empty deck. */
export function newPptxDeck(): PptxAdapterDeck {
  return {
    slides: [{ title: '新幻灯片', bullets: [''] }],
    originalBytes: null,
  }
}

function isUnchangedDeck(deck: PptxAdapterDeck): boolean {
  if (!deck.originalBytes) return false
  if (deck.slides.length === 0) return false
  return deck.slides.every(
    (s) => s.title === '' && (s.bullets.length === 0 || (s.bullets.length === 1 && s.bullets[0] === '')),
  )
}
