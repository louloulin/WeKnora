/**
 * Notification center API client (Build #P0.4).
 *
 * The bell dropdown in the top nav polls /notifications/unread-count
 * every 30s and fetches /notifications lazily when the user opens
 * the panel. Mutations (read / dismiss / read-all) are fire-and-
 * forget from the UI; the server is the source of truth.
 *
 * The shape mirrors the Go types in internal/types/notification.go.
 * If the server adds new fields they will simply be ignored here
 * until the frontend picks them up — keeping the contract additive.
 */

import { get, post, del } from '../utils/request'

export type NotificationKind =
  | 'wiki.comment.created'
  | 'wiki.comment.reply'
  | 'wiki.mentioned'
  | 'agent.shared'
  | 'kb.shared'
  | 'system.alert'

export type NotificationStatus = 'unread' | 'read' | 'dismissed'

export interface Notification {
  id: number
  tenant_id: number
  recipient_user_id: string
  kind: NotificationKind
  title: string
  body?: string
  payload?: Record<string, unknown>
  status: NotificationStatus
  actor_user_id?: string
  resource_type?: string
  resource_id?: string
  read_at?: string
  dismissed_at?: string
  created_at: string
  updated_at: string
}

export interface NotificationListQuery {
  page?: number
  page_size?: number
  status?: NotificationStatus
  kind?: NotificationKind
  since_days?: number
}

export interface NotificationListResult {
  items: Notification[]
  total: number
}

export interface NotificationUnreadCount {
  count: number
}

export async function listNotifications(
  q: NotificationListQuery = {},
): Promise<NotificationListResult> {
  const params: Record<string, string> = {}
  if (q.page) params.page = String(q.page)
  if (q.page_size) params.page_size = String(q.page_size)
  if (q.status) params.status = q.status
  if (q.kind) params.kind = q.kind
  if (q.since_days) params.since_days = String(q.since_days)
  const qs = new URLSearchParams(params).toString()
  return get<NotificationListResult>(
    `/api/v1/notifications${qs ? `?${qs}` : ''}`,
  )
}

export async function getUnreadCount(): Promise<NotificationUnreadCount> {
  return get<NotificationUnreadCount>('/api/v1/notifications/unread-count')
}

export async function markRead(id: number): Promise<void> {
  await post(`/api/v1/notifications/${id}/read`)
}

export async function markDismissed(id: number): Promise<void> {
  await post(`/api/v1/notifications/${id}/dismiss`)
}

export async function markAllRead(): Promise<{ updated: number }> {
  return post<{ updated: number }>('/api/v1/notifications/read-all')
}

export async function deleteNotification(id: number): Promise<void> {
  await del(`/api/v1/notifications/${id}`)
}
