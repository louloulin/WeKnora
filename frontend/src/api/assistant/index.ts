// AI Assistant Q&A API client (v0.7.17+).
//
// Provides:
//   - askAssistant(req)              → POST /api/v1/assistant/ask (JSON)
//   - askAssistantStream(req, cb, signal) → POST /api/v1/assistant/ask?stream=1 (SSE)
//   - listConversations(limit, offset) → GET /api/v1/assistant/conversations
//   - getConversation(conversationId) → GET /api/v1/assistant/conversations/:id

import { fetchEventSource } from '@microsoft/fetch-event-source'
import type {
  AssistantAskRequest,
  AssistantAskResponse,
  AssistantConversation,
  AssistantStreamEvent,
} from './types'

const BASE = '/api/v1/assistant'

/** Send a synchronous JSON request. */
export async function askAssistant(req: AssistantAskRequest): Promise<AssistantAskResponse> {
  const token = localStorage.getItem('weknora_token') || ''
  const resp = await fetch(`${BASE}/ask`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: token ? `Bearer ${token}` : '',
    },
    body: JSON.stringify(req),
  })
  if (!resp.ok) {
    const text = await resp.text().catch(() => '')
    throw new AssistantApiError(resp.status, text || resp.statusText)
  }
  return resp.json()
}

/**
 * Stream a response via SSE. The callback fires once per parsed
 * frame. AbortSignal lets the caller stop generation (e.g. when
 * the user clicks "Stop").
 *
 * Uses @microsoft/fetch-event-source so AbortSignal / auth headers
 * work the way fetch does not support natively with EventSource.
 */
export async function askAssistantStream(
  req: AssistantAskRequest,
  onEvent: (e: AssistantStreamEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const token = localStorage.getItem('weknora_token') || ''
  await fetchEventSource(`${BASE}/ask?stream=1`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: token ? `Bearer ${token}` : '',
    },
    body: JSON.stringify(req),
    signal,
    async onopen(resp) {
      if (resp.ok) return
      if (resp.status === 401) throw new AssistantApiError(401, 'unauthorized')
      throw new AssistantApiError(resp.status, await resp.text().catch(() => resp.statusText))
    },
    onmessage(msg) {
      const event = parseSSEMessage(msg.event, msg.data)
      if (event) onEvent(event)
    },
    onerror(err) {
      // Let fetchEventSource retry on transient errors; surface
      // hard failures to the caller.
      if (err instanceof AssistantApiError) throw err
    },
    openWhenHidden: true,
  })
}

/** Parse one SSE message into a typed AssistantStreamEvent. */
export function parseSSEMessage(event: string, data: string): AssistantStreamEvent | null {
  if (!data) return null
  let payload: any
  try {
    payload = JSON.parse(data)
  } catch {
    return null
  }
  switch (event) {
    case 'metadata':
      return {
        type: 'metadata',
        conversationId: payload.conversation_id ?? '',
        answerId: payload.answer_id ?? '',
        tenantId: payload.tenant_id ?? 0,
      }
    case 'citation':
      return {
        type: 'citation',
        index: payload.index ?? 0,
        citation: payload.citation,
      }
    case 'token':
      return { type: 'token', text: payload.text ?? '' }
    case 'done':
      return {
        type: 'done',
        promptTokens: payload.prompt_tokens ?? 0,
        completionTokens: payload.completion_tokens ?? 0,
        finishReason: payload.finish_reason ?? 'stop',
      }
    case 'error':
      return { type: 'error', error: payload.error ?? 'unknown' }
    default:
      return null
  }
}

/** Paginated audit listing. */
export async function listConversations(limit = 20, offset = 0): Promise<{
  items: AssistantConversation[]
  total: number
}> {
  const token = localStorage.getItem('weknora_token') || ''
  const resp = await fetch(
    `${BASE}/conversations?limit=${limit}&offset=${offset}`,
    { headers: { Authorization: token ? `Bearer ${token}` : '' } },
  )
  if (!resp.ok) {
    throw new AssistantApiError(resp.status, await resp.text().catch(() => resp.statusText))
  }
  return resp.json()
}

/** Get every turn of a single conversation. */
export async function getConversation(conversationId: string): Promise<{
  items: AssistantConversation[]
}> {
  const token = localStorage.getItem('weknora_token') || ''
  const resp = await fetch(`${BASE}/conversations/${encodeURIComponent(conversationId)}`, {
    headers: { Authorization: token ? `Bearer ${token}` : '' },
  })
  if (!resp.ok) {
    throw new AssistantApiError(resp.status, await resp.text().catch(() => resp.statusText))
  }
  return resp.json()
}

/** Typed error so the panel can branch on HTTP status. */
export class AssistantApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
    this.name = 'AssistantApiError'
  }
  get isUnauthorized(): boolean { return this.status === 401 }
  get isTransient(): boolean { return this.status === 502 || this.status === 503 || this.status === 504 }
}
