// TypeScript types for the AI Assistant Q&A backend (v0.7.17+).
// These mirror the Go types in internal/types/assistant.go and the
// SSE event shape produced by llmstream.FormatSSEEvent.

/** One source backing an assistant response. */
export interface AssistantCitation {
  /** "kb" or "wiki". */
  type: 'kb' | 'wiki'
  id: string
  title: string
  /** Wiki slug (empty for KB citations). */
  slug?: string
  /** Knowledge base id (KB citations only). */
  kb_id?: string
  /** Short text excerpt that justified the match. */
  snippet: string
  /** Normalised relevance score in [0, 1]. */
  score: number
}

/** Wire body for POST /assistant/ask. */
export interface AssistantAskRequest {
  query: string
  /** Optional list of KB ids to scope the retrieval. Empty means "all visible KBs". */
  source_kb_ids?: string[]
  /** Whether to include Wiki hits in the fused retrieval. */
  include_wiki?: boolean
  /** Cap per source. Default 5, max 20. */
  max_results_per_source?: number
  /** Thread an existing conversation. When empty a new uuid is generated server-side. */
  conversation_id?: string
}

/** Wire body returned by POST /assistant/ask. */
export interface AssistantAskResponse {
  answer_id: string
  conversation_id: string
  answer_text: string
  kb_citations: AssistantCitation[]
  wiki_citations: AssistantCitation[]
  model_name: string
  latency_ms: number
  result_count: number
  /** ISO 8601 timestamp. */
  created_at: string
}

/** One persisted Q&A turn. Returned by GET /assistant/conversations. */
export interface AssistantConversation {
  id: number
  tenant_id: string
  user_id: string
  conversation_id: string
  query_text: string
  kb_citations: AssistantCitation[]
  wiki_citations: AssistantCitation[]
  source_kb_ids: string[]
  include_wiki: boolean
  result_count: number
  model_name: string
  latency_ms: number
  /** ISO 8601 timestamp. */
  created_at: string
}

/** SSE event types emitted by the backend. */
export type AssistantStreamEventType =
  | 'metadata'
  | 'citation'
  | 'token'
  | 'done'
  | 'error'

/** Parsed SSE frame delivered to the panel. */
export type AssistantStreamEvent =
  | { type: 'metadata'; conversationId: string; answerId: string; tenantId: number }
  | { type: 'citation'; index: number; citation: AssistantCitation }
  | { type: 'token'; text: string }
  | {
      type: 'done'
      promptTokens: number
      completionTokens: number
      finishReason: string
    }
  | { type: 'error'; error: string }

/** Source KB id list the user can pin to the panel. */
export interface AssistantScope {
  kb_ids: string[]
  include_wiki: boolean
}
