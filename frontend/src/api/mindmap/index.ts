/**
 * MindMap REST client — Build #43 思维导图 API wrapper.
 *
 * Mirrors internal/handler/mindmap.go. All paths require an Authorization
 * Bearer token (injected by the existing Axios client).
 */
import axios from 'axios'

export type MindMapLayout = 'tree' | 'fishbone' | 'timeline' | 'radial' | 'free'
export type MindMapNodeType = 'text' | 'image' | 'link' | 'doc_ref' | 'task' | 'formula'
export type ExportFormat = 'png' | 'svg' | 'markdown' | 'opml' | 'xmind'

export interface MindMap {
  id: string
  tenant_id: number
  title: string
  layout: MindMapLayout
  theme: string
  root_node_id: string
  kb_id: string
  owner_user_id: number
  visibility: string
  created_at: string
  updated_at: string
}

export interface MindMapNode {
  id: string
  tenant_id: number
  map_id: string
  parent_id?: string | null
  node_type: MindMapNodeType
  label: string
  body: string
  x: number
  y: number
  width: number
  height: number
  color: string
  icon: string
  doc_ref?: string | null
  kb_ref?: string | null
  task_ref?: number | null
  formula: string
  order_hint: number
  created_at: string
  updated_at: string
}

export interface CreateMindMapRequest {
  title: string
  layout?: MindMapLayout
  theme?: string
  kb_id?: string
  visibility?: string
  root_label?: string
  root_body?: string
  root_color?: string
  root_icon?: string
}

export interface UpdateMindMapRequest {
  title?: string
  layout?: MindMapLayout
  theme?: string
  visibility?: string
  root_node_id?: string
}

export interface CreateMindMapNodeRequest {
  parent_id?: string | null
  node_type: MindMapNodeType
  label: string
  body?: string
  x?: number
  y?: number
  width?: number
  height?: number
  color?: string
  icon?: string
  doc_ref?: string | null
  kb_ref?: string | null
  task_ref?: number | null
  formula?: string
  order_hint?: number
}

export interface UpdateMindMapNodeRequest {
  parent_id?: string | null
  node_type?: MindMapNodeType
  label?: string
  body?: string
  x?: number
  y?: number
  width?: number
  height?: number
  color?: string
  icon?: string
  doc_ref?: string | null
  kb_ref?: string | null
  task_ref?: number | null
  formula?: string
  order_hint?: number
}

export interface AIExpandRequest {
  prompt: string
  breadth?: number
  anchor_node_id?: string
}

const baseURL = import.meta.env.VITE_API_BASE_URL || '/api/v1'

async function get<T>(url: string): Promise<T> {
  const r = await axios.get(`${baseURL}${url}`)
  return r.data as T
}

async function post<T>(url: string, body: any): Promise<T> {
  const r = await axios.post(`${baseURL}${url}`, body)
  return r.data as T
}

async function patch<T>(url: string, body: any): Promise<T> {
  const r = await axios.patch(`${baseURL}${url}`, body)
  return r.data as T
}

async function del(url: string): Promise<void> {
  await axios.delete(`${baseURL}${url}`)
}

export const mindmapApi = {
  list(filter: { kb_id?: string; visibility?: string; owner_user_id?: number } = {}) {
    const qs = new URLSearchParams()
    if (filter.kb_id) qs.append('kb_id', filter.kb_id)
    if (filter.visibility) qs.append('visibility', filter.visibility)
    if (filter.owner_user_id) qs.append('owner_user_id', String(filter.owner_user_id))
    return get<{ items: MindMap[]; total: number }>(`/mindmaps?${qs.toString()}`)
  },
  get(id: string) {
    return get<MindMap>(`/mindmaps/${id}`)
  },
  create(req: CreateMindMapRequest) {
    return post<MindMap>('/mindmaps', req)
  },
  update(id: string, patch: UpdateMindMapRequest) {
    return patch<MindMap>(`/mindmaps/${id}`, patch)
  },
  remove(id: string) {
    return del(`/mindmaps/${id}`)
  },
  listNodes(id: string) {
    return get<{ items: MindMapNode[] }>(`/mindmaps/${id}/nodes`).then((r) => r.items)
  },
  createNode(id: string, req: CreateMindMapNodeRequest) {
    return post<MindMapNode>(`/mindmaps/${id}/nodes`, req)
  },
  updateNode(id: string, nodeID: string, patch: UpdateMindMapNodeRequest) {
    return patch<MindMapNode>(`/mindmaps/${id}/nodes/${nodeID}`, patch)
  },
  removeNode(id: string, nodeID: string) {
    return del(`/mindmaps/${id}/nodes/${nodeID}`)
  },
  autoLayout(id: string, layout: MindMapLayout, spacing = 80) {
    return post<{ items: MindMapNode[] }>(`/mindmaps/${id}/auto-layout`, { layout, spacing })
  },
  exportMap(id: string, format: ExportFormat) {
    return get<string>(`/mindmaps/${id}/export?format=${format}`)
  },
  aiExpand(id: string, req: AIExpandRequest) {
    return post<MindMapNode[]>(`/mindmaps/${id}/ai-expand`, req)
  },
  cluster(id: string) {
    return post<MindMapNode[]>(`/mindmaps/${id}/cluster`, {})
  },
}
