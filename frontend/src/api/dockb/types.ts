// v0.7.23 — Doc ↔ KB AI Bridge + WeKnora Base / Database
// (Microsoft Loop / Notion database parity)

export type DatabaseFieldType = 'text' | 'number' | 'select' | 'checkbox' | 'date';

export interface DatabaseField {
  name: string;
  type: DatabaseFieldType;
  options?: string[];
  width?: number;
  required?: boolean;
}

export interface WKDatabase {
  id: number;
  tenant_id: string;
  name: string;
  description: string;
  schema: DatabaseField[];
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface WKDatabaseRow {
  id: number;
  tenant_id: string;
  database_id: number;
  values: Record<string, unknown>;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface DocKBSummary {
  id: number;
  tenant_id: string;
  knowledge_id: string;
  chunk_id: string;
  summary: string;
  keyphrases: string[];
  auto_tags: string[];
  model_name: string;
  confidence: number;
  created_at: string;
  updated_at: string;
}

export interface UpsertSummaryRequest {
  text: string;
  model_name?: string;
}

export interface CreateDatabaseRequest {
  name: string;
  description?: string;
  schema: DatabaseField[];
}

export interface UpdateDatabaseRequest {
  name?: string;
  description?: string;
  schema?: DatabaseField[];
}

export interface InsertRowRequest {
  values: Record<string, unknown>;
}

export interface ListDatabasesResponse {
  databases: WKDatabase[];
  total: number;
}

export interface ListRowsResponse {
  rows: WKDatabaseRow[];
  total: number;
}

export interface ListSummariesResponse {
  summaries: DocKBSummary[];
  total: number;
}
