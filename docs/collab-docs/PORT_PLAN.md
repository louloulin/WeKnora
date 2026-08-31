# WeKnora Collaborative Docs — GenOffice Port Plan

## Goal

Port `/Users/louloulin/appx/genoffice` (Apache-2.0) capabilities into WeKnora
to deliver a Feishu/Tencent document-like editing surface for DOCX / PPTX /
XLSX with realtime collaboration.

## Source Inventory (genoffice)

- `packages/docx-engine/`  ~9k lines TS — parse/save byte-preserving .docx
- `packages/pptx-engine/` ~17k lines TS — parse/save .pptx + master/layout/edit
- `packages/pptx-render/` ~10k lines TS — Konva render layer (build-slide + coords + metrics)
- `packages/file-parse/`   ~500 lines TS — DOCX/PPTX/XLSX → text (multimodal-ready)
- `apps/docs/`            ~5k lines React+TipTap — DOC editor shell
- `apps/slides/`          ~10k lines React+Konva — PPT editor shell
- `apps/sheets/`          Univer 0.25.1 + **Rust xlsx sidecar** (closed-source EE)
- `apps/pdf/`             Skipped (out of scope)
- `apps/markdown/`        Skipped (WeKnora has its own)
- `packages/{agent-core,ai-provider,ai-search,project-store,ui,i18n,electron-utils}/`
                              DEFERRED — WeKnora already has its own chat/agent layer

## Target Architecture

```
Vue 3 + Vite frontend
├── src/editor/engines/             (port from genoffice/packages/)
│   ├── docx-engine/                 full port
│   ├── pptx-engine/                 full port
│   ├── pptx-render/                 partial (build-slide, coords, fill, metrics, render-tree)
│   └── file-parse/                  full port
├── src/components/collab/
│   ├── CollabDocEditor.vue          TipTap + docx-engine
│   ├── CollabSheetEditor.vue        Univer 0.25.1
│   ├── CollabSlideEditor.vue        vue-konva + pptx-engine + pptx-render
│   └── (adapters between TipTap's ProseMirror model and docx-engine block tree)
└── src/composables/useYjsCollabDoc.ts   (already in place — extend for DOC/PPT blocks)

Go backend (already has collab_doc v0.7.25)
├── internal/application/service/collaborative_doc.go
├── internal/handler/collaborative_doc*.go
└── (extend: per-paragraph / per-shape ops + BLOB snapshots + docparser pipeline)

docparser (Python, already running)
└── (extend) /render/pptx — accept slide JSON, return .pptx bytes via python-pptx
```

## Phases

### Phase 1 — Engine port (week 1)

1. `cp -r genoffice/packages/docx-engine/src frontend/src/editor/engines/docx-engine/`
2. Same for pptx-engine, pptx-render, file-parse
3. Rewrite internal `genoffice` workspace imports as relative paths
4. `tsc --noEmit` clean
5. Run genoffice unit tests against the port (vitest run)
6. Inject engines into Vite via `vite.config.ts` alias

### Phase 2 — Editors (Vue 3 port, week 1-2)

DOC editor:
- Upload .docx → `parseDocx(bytes)` → block tree
- Map each block to a TipTap custom node (`docxBlock` with `docxIndex`)
- TipTap onChange → for each dirty paragraph call `patchParagraphTexts(...)` → upload
- Load → `parseDocx`, render TipTap doc, hydrate

PPTX editor:
- `openPptx(bytes)` → SlideDeck
- Konva stage renders each slide (vue-konva)
- Text edit → `patchTextElementXml(el, originalXml)` → store slide patch in Y.Map
- Periodic `savePptx` flushes patched slides to backend

XLSX editor:
- Univer 0.25.1 + `@univerjs/preset-sheets-core`
- Univer has built-in Yjs provider; point it at our existing collab-doc WS endpoint

### Phase 3 — CRDT (week 2)

DOC: per-paragraph `Y.Text`. TipTap extension `Collaboration` with a custom
fragment key per paragraph. Patch granularity = docx paragraph.

PPTX: per-shape `Y.Map<text, transform, fill, ...>`. Konva edit handlers
write directly into the Y.Map; observers call `patchSlideElement` once on
WS tick.

XLSX: native Univer ↔ Yjs.

Snapshots: backend stores compressed binary blob (`docx_state`, `pptx_state`,
`xlsx_state`) rather than Yjs bytes — the file format is the source of truth.

### Phase 4 — KB sync + share (week 2-3)

- `POST /collaborative-docs/:id/sync-to-kb` — pull latest bytes, hand to
  docparser `/chunk`, push to KB ingestion
- `GET /collab-documents/share/:token` — public read-only preview
- docparser `/render/pptx` — accept our slide JSON, return real .pptx bytes

### Phase 5 — AI agent integration (week 3)

- Block selection → "ask AI" popover
- AI provider already in WeKnora; just feed docx-engine block JSON as context
- Apply AI suggestions via docx-engine `patchParagraphTexts`

## Not ported

- PDF editing (out of scope)
- Rust xlsx sidecar (EE closed-source) — replaced by Univer
- Electron shell (WeKnora is web)
- PPT animations/masters (deferred to v0.7.27)
- Multi-section headers/footers beyond first page (deferred)

## Timeline

| Week | Phase  | Deliverable                                         |
| ---- | ------ | --------------------------------------------------- |
| 1    | 1+2    | Single-user DOC/PPTX/XLSX open-edit-save loop       |
| 2    | 3+4    | Realtime collaboration + KB sync + share link       |
| 3    | 5      | AI agent block-level editing + smoke + e2e tests    |

## Risks

1. Engine import path conversion (workspaces → relative) — 1-2 days
2. TipTap ↔ docx-engine round-trip — bidirectional block mapping
3. Vue 3 ↔ react-konva — vue-konva API gaps require thin wrapper
4. CRDT + byte-level patch conflict resolution — last-write-wins on byte level
5. XLSX without Rust sidecar — formula recalc / charts / macros limited to read-only
