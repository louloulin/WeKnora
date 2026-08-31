// WeKnora TypeScript SDK — schema types mirroring OpenAPI spec.

export type KnowledgeBaseType = 'wiki' | 'rag' | 'hybrid';

export interface KnowledgeBase {
  id: string;
  name: string;
  description?: string;
  type: KnowledgeBaseType;
  chunk_count?: number;
  created_at?: string;
  updated_at?: string;
}

export interface KnowledgeBaseInput {
  name: string;
  description?: string;
  type: KnowledgeBaseType;
}

export interface KnowledgeBasePatch {
  name?: string;
  description?: string;
}

export interface KnowledgeBasePage {
  items: KnowledgeBase[];
  next_page_token?: string;
}

export interface SearchRequest {
  query: string;
  top_k?: number;
  rerank?: boolean;
  filter?: Record<string, unknown>;
}

export interface SearchHit {
  chunk_id: string;
  score: number;
  text: string;
  document_id: string;
  document_title: string;
  highlights?: string[];
}

export interface SearchResponse {
  hits: SearchHit[];
}

export interface Citation {
  chunk_id: string;
  document_title: string;
  text: string;
  score: number;
}

export interface AskRequest {
  question: string;
  conversation_id?: string;
  stream?: boolean;
}

export interface AskResponse {
  answer: string;
  citations: Citation[];
  conversation_id?: string;
}

export interface ChatMessage {
  role: 'user' | 'assistant' | 'system';
  content: string;
}

export interface ChatRequest {
  messages: ChatMessage[];
  conversation_id?: string;
}

export type ChatChunkType = 'delta' | 'citation' | 'done' | 'error';

export interface ChatChunk {
  type: ChatChunkType;
  content?: string;
  citation?: Citation;
  error?: string;
}

export interface DatabaseColumn {
  id?: string;
  name: string;
  type: 'text' | 'number' | 'date' | 'select' | 'multi_select' | 'user' | 'file' | 'formula' | 'rollup' | 'link';
  options?: Record<string, unknown>;
}

export interface DatabaseView {
  id?: string;
  name: string;
  type: 'grid' | 'kanban' | 'calendar' | 'gallery';
}

export interface Database {
  id?: string;
  kb_id?: string;
  name: string;
  columns?: DatabaseColumn[];
  views?: DatabaseView[];
}

export interface DatabaseInput {
  name: string;
  columns: DatabaseColumn[];
}

export interface Row {
  id?: string;
  database_id?: string;
  values: Record<string, unknown>;
  created_at?: string;
  updated_at?: string;
}

export interface FormulaEvalRequest {
  expression: string;
  context?: Record<string, unknown>;
}

export interface FormulaEvalResponse {
  value: unknown;
  type?: string;
  error?: string;
}

export type AutomationTriggerType = 'manual' | 'scheduled' | 'row_changed' | 'webhook';
export type AutomationActionType =
  | 'update_field'
  | 'create_row'
  | 'send_webhook'
  | 'run_agent'
  | 'notify';

export interface AutomationStep {
  id: string;
  action_type: AutomationActionType;
  config?: Record<string, unknown>;
  next_ids?: string[];
}

export interface Automation {
  id?: string;
  tenant_id?: string;
  kb_id?: string;
  database_id: string;
  name: string;
  trigger_type: AutomationTriggerType;
  trigger_config?: Record<string, unknown>;
  steps: AutomationStep[];
  enabled?: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface CollabDoc {
  id?: string;
  kb_id?: string;
  title: string;
  kind: 'doc' | 'sheet' | 'slide';
  created_at?: string;
  updated_at?: string;
  current_version?: number;
}

export interface CollabDocFile {
  doc_id: string;
  version: number;
  sha256: string;
  size_bytes: number;
  content_type: string;
  created_at?: string;
}

export interface Agent {
  id?: string;
  tenant_id?: string;
  name: string;
  description?: string;
  model: string;
  tools?: string[];
  memory?: 'none' | 'short' | 'long';
  system_prompt?: string;
}

export interface AgentRun {
  id?: string;
  agent_id?: string;
  status?: 'queued' | 'running' | 'succeeded' | 'failed';
  triggered_by?: string;
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
  steps_count?: number;
  tokens_used?: number;
  started_at?: string;
  finished_at?: string;
}

export interface Connector {
  id?: string;
  tenant_id?: string;
  kind: string;
  name: string;
  status?: 'active' | 'paused' | 'error';
  last_sync_at?: string;
  config?: Record<string, unknown>;
}

export interface VerificationRequest {
  page_id?: string;
  include_kg?: boolean;
}

export interface VerificationReport {
  kb_id?: string;
  scanned_at?: string;
  trust_score?: number;
  findings?: Array<{
    page_id?: string;
    severity: 'info' | 'warn' | 'critical';
    message: string;
  }>;
}
