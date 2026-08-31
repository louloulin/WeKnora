import { get, post, patch, del } from '../../utils/request';
import type {
  CreateConnectorRequest,
  IngestConnector,
  IngestJob,
  ListConnectorsResponse,
  ListJobsResponse,
} from './types';

export async function listConnectors(): Promise<ListConnectorsResponse> {
  return get<ListConnectorsResponse>('/api/v1/connectors');
}

export async function createConnector(body: CreateConnectorRequest): Promise<IngestConnector> {
  return post<IngestConnector>('/api/v1/connectors', body);
}

export async function getConnector(id: number): Promise<IngestConnector> {
  return get<IngestConnector>(`/api/v1/connectors/${id}`);
}

export async function updateConnector(
  id: number,
  body: CreateConnectorRequest,
): Promise<IngestConnector> {
  return patch<IngestConnector>(`/api/v1/connectors/${id}`, body);
}

export async function deleteConnector(id: number): Promise<{ status: string }> {
  return del<{ status: string }>(`/api/v1/connectors/${id}`);
}

export async function triggerConnector(id: number): Promise<IngestJob> {
  return post<IngestJob>(`/api/v1/connectors/${id}/trigger`, {});
}

export async function listConnectorJobs(id: number): Promise<ListJobsResponse> {
  return get<ListJobsResponse>(`/api/v1/connectors/${id}/jobs`);
}

export async function listTenantJobs(): Promise<ListJobsResponse> {
  return get<ListJobsResponse>('/api/v1/connectors/jobs');
}
