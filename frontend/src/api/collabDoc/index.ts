/**
 * Collaborative document API client.
 *
 * v0.7.25 — collaborative_docs (Feishu / Tencent document parity).
 *
 * This client wraps the REST surface exposed by the backend at
 *   /api/v1/collaborative-docs
 * The realtime Yjs WebSocket connection is established by the editor
 * component itself (see useYjsCollabDoc) so the Yjs binary framing stays
 * out of the typed HTTP layer.
 */
import http from '@/utils/request'
import type { AxiosResponse } from 'axios'

export type CollabDocKind = 'doc' | 'sheet' | 'slide'

export interface CollabDoc {
  id: string
  tenant_id: number
  kb_id: string
  title: string
  doc_kind: CollabDocKind
  schema_version: number
  owner_user_id: number
  visibility: string
  share_token: string
  created_at: string
  updated_at: string
  archived_at?: string | null
}

export interface CollabDocSession {
  id: string
  tenant_id: number
  doc_id: string
  user_id: number
  client_id: number
  color: string
  display_name: string
  last_heartbeat: string
  joined_at: string
}

export interface ListCollabDocsFilter {
  kb_id?: string
  doc_kind?: CollabDocKind
  archived?: boolean
  limit?: number
  offset?: number
}

interface ApiEnvelope<T> { success: boolean; data: T; total?: number }

export async function listCollabDocs(filter: ListCollabDocsFilter = {}): Promise<{ items: CollabDoc[]; total: number }> {
  const res: AxiosResponse<ApiEnvelope<CollabDoc[]>> = await http.get('/collaborative-docs', { params: filter })
  return { items: res.data.data ?? [], total: res.data.total ?? 0 }
}

export async function getCollabDoc(id: string): Promise<CollabDoc> {
  const res: AxiosResponse<ApiEnvelope<CollabDoc>> = await http.get(`/collaborative-docs/${id}`)
  return res.data.data
}

export async function createCollabDoc(payload: {
  kb_id: string
  title: string
  doc_kind?: CollabDocKind
}): Promise<CollabDoc> {
  const res: AxiosResponse<ApiEnvelope<CollabDoc>> = await http.post('/collaborative-docs', payload)
  return res.data.data
}

export async function updateCollabDoc(id: string, payload: { title?: string; visibility?: string }): Promise<CollabDoc> {
  const res: AxiosResponse<ApiEnvelope<CollabDoc>> = await http.patch(`/collaborative-docs/${id}`, payload)
  return res.data.data
}

export async function archiveCollabDoc(id: string): Promise<void> {
  await http.post(`/collaborative-docs/${id}/archive`)
}

export async function deleteCollabDoc(id: string): Promise<void> {
  await http.delete(`/collaborative-docs/${id}`)
}

export async function listCollabDocPresence(id: string): Promise<CollabDocSession[]> {
  const res: AxiosResponse<ApiEnvelope<CollabDocSession[]>> = await http.get(`/collaborative-docs/${id}/presence`)
  return res.data.data ?? []
}

/**
 * openCollabDocRealtimeURL returns the WebSocket URL the editor passes to
 * WebsocketProvider. The JWT travels in the query string because the
 * browser cannot set custom headers on a WebSocket upgrade — same pattern
 * as the wiki realtime composable (v0.7.19).
 */
export function openCollabDocRealtimeURL(docId: string, token: string): string {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  return `${proto}//${host}/api/v1/collaborative-docs/${encodeURIComponent(docId)}/realtime?token=${encodeURIComponent(token)}`
}
