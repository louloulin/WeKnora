import { get, post, patch, del, put } from '../../utils/request';
import type {
  CreateDatabaseRequest,
  DocKBSummary,
  InsertRowRequest,
  ListDatabasesResponse,
  ListRowsResponse,
  ListSummariesResponse,
  UpdateDatabaseRequest,
  UpsertSummaryRequest,
  WKDatabase,
  WKDatabaseRow,
} from './types';

// ===== Doc ↔ KB AI Bridge =====

export async function upsertDocKBSummary(
  knowledgeId: string,
  chunkId: string,
  body: UpsertSummaryRequest,
): Promise<DocKBSummary> {
  return put<DocKBSummary>(
    `/api/v1/dockb/chunks/${encodeURIComponent(knowledgeId)}/${encodeURIComponent(chunkId)}`,
    body,
  );
}

export async function getDocKBSummary(
  knowledgeId: string,
  chunkId: string,
): Promise<DocKBSummary> {
  return get<DocKBSummary>(
    `/api/v1/dockb/chunks/${encodeURIComponent(knowledgeId)}/${encodeURIComponent(chunkId)}`,
  );
}

export async function listDocKBSummaries(knowledgeId: string): Promise<ListSummariesResponse> {
  return get<ListSummariesResponse>(
    `/api/v1/dockb/summaries/${encodeURIComponent(knowledgeId)}`,
  );
}

export async function deleteDocKBSummary(id: number): Promise<{ status: string }> {
  return del<{ status: string }>(`/api/v1/dockb/summaries/${id}`);
}

// ===== WeKnora Base / Database =====

export async function listDatabases(): Promise<ListDatabasesResponse> {
  return get<ListDatabasesResponse>('/api/v1/databases');
}

export async function createDatabase(body: CreateDatabaseRequest): Promise<WKDatabase> {
  return post<WKDatabase>('/api/v1/databases', body);
}

export async function getDatabase(id: number): Promise<WKDatabase> {
  return get<WKDatabase>(`/api/v1/databases/${id}`);
}

export async function updateDatabase(
  id: number,
  body: UpdateDatabaseRequest,
): Promise<WKDatabase> {
  return patch<WKDatabase>(`/api/v1/databases/${id}`, body);
}

export async function deleteDatabase(id: number): Promise<{ status: string }> {
  return del<{ status: string }>(`/api/v1/databases/${id}`);
}

export async function listRows(databaseId: number, limit = 100, offset = 0): Promise<ListRowsResponse> {
  return get<ListRowsResponse>(
    `/api/v1/databases/${databaseId}/rows?limit=${limit}&offset=${offset}`,
  );
}

export async function insertRow(databaseId: number, body: InsertRowRequest): Promise<WKDatabaseRow> {
  return post<WKDatabaseRow>(`/api/v1/databases/${databaseId}/rows`, body);
}

export async function getRow(databaseId: number, rowId: number): Promise<WKDatabaseRow> {
  return get<WKDatabaseRow>(`/api/v1/databases/${databaseId}/rows/${rowId}`);
}

export async function updateRow(
  databaseId: number,
  rowId: number,
  body: InsertRowRequest,
): Promise<WKDatabaseRow> {
  return patch<WKDatabaseRow>(`/api/v1/databases/${databaseId}/rows/${rowId}`, body);
}

export async function deleteRow(databaseId: number, rowId: number): Promise<{ status: string }> {
  return del<{ status: string }>(`/api/v1/databases/${databaseId}/rows/${rowId}`);
}
