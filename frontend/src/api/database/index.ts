// Database multi-view REST API client (Build #26, G06).
// Mirrors internal/handler/database.go and types/database.go.

import request from '@/utils/request'

export interface Database {
  id: string
  tenant_id: number
  knowledge_base_id: string
  name: string
  description: string
  icon: string
  created_by: string
  created_at: string
  updated_at: string
}

export interface DatabaseField {
  id: string
  database_id: string
  name: string
  type: string
  options: Record<string, any>
  width: number
  sort_order: number
  is_primary: boolean
  created_at: string
}

export interface DatabaseRow {
  id: string
  database_id: string
  data: Record<string, any> | string // backend may return raw JSON
  sort_order: number
  created_by: string
  created_at: string
  updated_at: string
}

export interface DatabaseView {
  id: string
  database_id: string
  type: string
  name: string
  config: Record<string, any> | string
  sort_order: number
  is_default: boolean
  created_by: string
  created_at: string
  updated_at: string
}

export interface DatabaseDetail {
  database: Database
  fields: DatabaseField[]
  views: DatabaseView[]
}

// --- databases ---

export async function listDatabases(knowledgeBaseId: string, params?: { limit?: number; offset?: number }) {
  return request.get<{ items: Database[]; total: number }>(`/knowledge-bases/${knowledgeBaseId}/databases`, { params })
}

export async function createDatabase(knowledgeBaseId: string, body: { name: string; description?: string; icon?: string }) {
  return request.post<Database>(`/knowledge-bases/${knowledgeBaseId}/databases`, body)
}

export async function getDatabase(_knowledgeBaseId: string, id: string) {
  // We pass knowledge base ID for context but the API doesn't need it.
  return request.get<DatabaseDetail>(`/databases/${id}`)
}

export async function updateDatabase(id: string, body: Partial<Database>) {
  return request.patch<Database>(`/databases/${id}`, body)
}

export async function deleteDatabase(id: string) {
  return request.delete(`/databases/${id}`)
}

// --- fields ---

export async function createField(databaseId: string, body: Partial<DatabaseField>) {
  return request.post<DatabaseField>(`/databases/${databaseId}/fields`, body)
}

export async function updateField(databaseId: string, fieldId: string, body: Partial<DatabaseField>) {
  return request.patch<DatabaseField>(`/databases/${databaseId}/fields/${fieldId}`, body)
}

export async function deleteField(databaseId: string, fieldId: string) {
  return request.delete(`/databases/${databaseId}/fields/${fieldId}`)
}

// --- rows ---

export async function listRows(databaseId: string, params?: { limit?: number; offset?: number }) {
  return request.get<{ items: DatabaseRow[]; total: number }>(`/databases/${databaseId}/rows`, { params })
}

export async function createRow(databaseId: string, data: string) {
  return request.post<DatabaseRow>(`/databases/${databaseId}/rows`, { data })
}

export async function updateRow(databaseId: string, rowId: string, data: string) {
  return request.patch<DatabaseRow>(`/databases/${databaseId}/rows/${rowId}`, { data })
}

export async function deleteRow(databaseId: string, rowId: string) {
  return request.delete(`/databases/${databaseId}/rows/${rowId}`)
}

// --- views ---

export async function listViews(databaseId: string) {
  return request.get<{ items: DatabaseView[] }>(`/databases/${databaseId}/views`)
}

export async function createView(databaseId: string, body: Partial<DatabaseView>) {
  return request.post<DatabaseView>(`/databases/${databaseId}/views`, body)
}

export async function updateView(databaseId: string, viewId: string, body: Partial<DatabaseView>) {
  return request.patch<DatabaseView>(`/databases/${databaseId}/views/${viewId}`, body)
}

export async function deleteView(databaseId: string, viewId: string) {
  return request.delete(`/databases/${databaseId}/views/${viewId}`)
}
