// v0.7.24 — AI Connector framework (Notion AI Connectors / Glean parity)

export type ConnectorKind =
  | 'slack'
  | 'email'
  | 'webhook'
  | 'rss'
  | 'confluence'
  | 'notion'
  | 'jira';

export const CONNECTOR_KINDS: { kind: ConnectorKind; label: string; description: string }[] = [
  { kind: 'slack', label: 'Slack', description: 'Channels → KB' },
  { kind: 'email', label: 'Email', description: 'IMAP mailbox → KB' },
  { kind: 'webhook', label: 'Webhook', description: 'External push → KB' },
  { kind: 'rss', label: 'RSS', description: 'Periodic feed → KB' },
  { kind: 'confluence', label: 'Confluence', description: 'Spaces → KB (v0.7.24 stub)' },
  { kind: 'notion', label: 'Notion', description: 'Pages → KB (v0.7.24 stub)' },
  { kind: 'jira', label: 'Jira', description: 'Issues → KB (v0.7.24 stub)' },
];

export type IngestJobStatus = 'queued' | 'running' | 'succeeded' | 'failed';

export interface IngestConnector {
  id: number;
  tenant_id: string;
  name: string;
  kind: ConnectorKind;
  config: string;
  knowledge_base_id: string;
  enabled: boolean;
  last_sync_at?: string;
  last_error: string;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface IngestJob {
  id: number;
  tenant_id: string;
  connector_id: number;
  status: IngestJobStatus;
  result_count: number;
  error: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
}

export interface CreateConnectorRequest {
  name: string;
  kind: ConnectorKind;
  config: string; // JSON string
  knowledge_base_id?: string;
  enabled?: boolean;
}

export interface ListConnectorsResponse {
  connectors: IngestConnector[];
  total: number;
  kinds: ConnectorKind[];
}

export interface ListJobsResponse {
  jobs: IngestJob[];
  total: number;
}
