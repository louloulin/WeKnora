// v0.7.21 — Custom Agent Studio (飞书妙搭 / Notion Custom Agents parity)
//
// Type definitions mirroring the Go structs in
// internal/types/agent_studio.go.

export type TriggerType = 'manual' | 'cron' | 'event' | 'webhook';
export type TriggerStatus = 'active' | 'paused' | 'archived';
export type RunStatus =
  | 'queued'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'timeout'
  | 'cancelled';
export type CredentialType = 'api_key' | 'oauth2' | 'basic' | 'bearer' | 'custom';

export interface AgentTrigger {
  id: number;
  tenant_id: number;
  agent_id: string;
  trigger_type: TriggerType;
  name: string;
  trigger_config: string; // JSON-encoded
  payload_template: string;
  status: TriggerStatus;
  last_fired_at?: string;
  last_fire_status?: string;
  next_fire_at?: string;
  created_by: number;
  created_at: string;
  updated_at: string;
}

export interface AgentRun {
  id: number;
  tenant_id: number;
  agent_id: string;
  trigger_id?: number;
  triggered_by: string;
  triggered_user?: number;
  status: RunStatus;
  input_payload: string;
  output_payload: string;
  error_message: string;
  steps_count: number;
  tokens_used: number;
  cost_micros: number;
  started_at?: string;
  finished_at?: string;
  duration_ms: number;
  created_at: string;
}

export interface AgentCredential {
  id: number;
  tenant_id: number;
  name: string;
  credential_type: CredentialType;
  expires_at?: string;
  last_used_at?: string;
  created_at: string;
}

export interface CreateTriggerRequest {
  name: string;
  trigger_type: TriggerType;
  trigger_config?: string;
  payload_template?: string;
}

export interface CreateCredentialRequest {
  name: string;
  credential_type: CredentialType;
  secret: string;
  expires_at?: string;
}

export interface RunRequest {
  input?: Record<string, unknown>;
}

export interface ListRunsResponse {
  runs: AgentRun[];
  total: number;
}

export interface ListTriggersResponse {
  triggers: AgentTrigger[];
  total: number;
}

export interface ListCredentialsResponse {
  credentials: AgentCredential[];
  total: number;
}
