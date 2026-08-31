// API client — v0.7.37 Build #44 / v0.7.38 Build #46.x Slides backend
// (separate from the per-page collab doc surface; the deck/slide API
// is for the higher-level "doc -> 演示文稿" auto-generation flow
// plus hand-edited decks).
import { request } from '@/utils/request'

export interface SlideDeck {
  id: string
  tenant_id: number
  title: string
  theme: SlideTheme
  source_doc_id: string
  kb_id: string
  owner_user_id: number
  visibility: string
  slide_count: number
  created_at: string
  updated_at: string
}

export type SlideTheme =
  | 'notion' | 'confluence' | 'coda' | 'lark'
  | 'apple' | 'google' | 'academic' | 'dark'

export type SlideLayout =
  | 'title' | 'section' | 'bullet' | 'two_col' | 'image' | 'quote' | 'end'

export interface Slide {
  id: string
  deck_id: string
  index: number
  layout: SlideLayout
  title: string
  body_md: string
  bullets: string[]
  image_url: string
  notes: string
  created_at: string
  updated_at: string
}

export interface ListSlideDecksResponse {
  items: SlideDeck[]
  total: number
}

export interface ListSlidesResponse {
  items: Slide[]
  total: number
}

export interface CreateSlideDeckRequest {
  title: string
  theme?: SlideTheme
  source_doc_id?: string
  kb_id?: string
  visibility?: string
  slides?: Array<{
    title: string
    layout?: SlideLayout
    bullets?: string[]
    body_md?: string
  }>
}

export interface UpdateSlideDeckRequest {
  title?: string
  theme?: SlideTheme
  visibility?: string
}

export interface CreateSlideRequest {
  layout?: SlideLayout
  title?: string
  bullets?: string[]
  body_md?: string
  image_url?: string
  notes?: string
}

export interface UpdateSlideRequest {
  layout?: SlideLayout
  title?: string
  bullets?: string[]
  body_md?: string
  image_url?: string
  notes?: string
  index?: number
}

export interface AutoGenerateRequest {
  source_doc_id: string
  kb_id?: string
  title: string
  theme?: SlideTheme
  max_slides?: number
}

export type SlideExportFormat = 'markdown' | 'json' | 'html'

export function listSlideDecks(params: Record<string, any> = {}): Promise<ListSlideDecksResponse> {
  return request.get('/slides', { params })
}

export function createSlideDeck(req: CreateSlideDeckRequest): Promise<SlideDeck> {
  return request.post('/slides', req)
}

export function getSlideDeck(id: string): Promise<SlideDeck> {
  return request.get(`/slides/${id}`)
}

export function updateSlideDeck(id: string, patch: UpdateSlideDeckRequest): Promise<SlideDeck> {
  return request.patch(`/slides/${id}`, patch)
}

export function deleteSlideDeck(id: string): Promise<void> {
  return request.delete(`/slides/${id}`)
}

export function autoGenerateSlides(req: AutoGenerateRequest): Promise<SlideDeck> {
  return request.post('/slides/auto-generate', req)
}

export function listSlides(deckID: string): Promise<ListSlidesResponse> {
  return request.get(`/slides/${deckID}/slides`)
}

export function createSlide(deckID: string, req: CreateSlideRequest): Promise<Slide> {
  return request.post(`/slides/${deckID}/slides`, req)
}

export function updateSlide(deckID: string, slideID: string, patch: UpdateSlideRequest): Promise<Slide> {
  return request.patch(`/slides/${deckID}/slides/${slideID}`, patch)
}

export function deleteSlide(deckID: string, slideID: string): Promise<void> {
  return request.delete(`/slides/${deckID}/slides/${slideID}`)
}

export function exportSlides(deckID: string, format: SlideExportFormat): Promise<{ content: string }> {
  return request.get(`/slides/${deckID}/export`, { params: { format } })
}
