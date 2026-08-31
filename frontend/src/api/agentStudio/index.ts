import { get, post, del } from '../../utils/request';
import type {
  AgentCredential,
  AgentRun,
  AgentTrigger,
  CreateCredentialRequest,
  CreateTriggerRequest,
  ListCredentialsResponse,
  ListRunsResponse,
  ListTriggersResponse,
  RunRequest,
} from './types';

const base = (kbId: string, agentId: string) =>
  `/api/v1/knowledgebase/${encodeURIComponent(kbId)}/agents/${encodeURIComponent(agentId)}/studio`;

// --- triggers ---

export async function createTrigger(
  kbId: string,
  agentId: string,
  body: CreateTriggerRequest,
): Promise<AgentTrigger> {
  return post<AgentTrigger>(`${base(kbId, agentId)}/triggers`, body);
}

export async function listTriggers(
  kbId: string,
  agentId: string,
): Promise<ListTriggersResponse> {
  return get<ListTriggersResponse>(`${base(kbId, agentId)}/triggers`);
}

export async function pauseTrigger(
  kbId: string,
  agentId: string,
  triggerId: number,
): Promise<{ status: string }> {
  return post<{ status: string }>(
    `${base(kbId, agentId)}/triggers/${triggerId}/pause`,
    {},
  );
}

export async function resumeTrigger(
  kbId: string,
  agentId: string,
  triggerId: number,
): Promise<{ status: string }> {
  return post<{ status: string }>(
    `${base(kbId, agentId)}/triggers/${triggerId}/resume`,
    {},
  );
}

export async function deleteTrigger(
  kbId: string,
  agentId: string,
  triggerId: number,
): Promise<void> {
  await del<void>(`${base(kbId, agentId)}/triggers/${triggerId}`);
}

// --- runs ---

export async function runAgent(
  kbId: string,
  agentId: string,
  body: RunRequest,
): Promise<AgentRun> {
  return post<AgentRun>(`${base(kbId, agentId)}/run`, body);
}

export async function listRuns(
  kbId: string,
  agentId: string,
  limit = 50,
  offset = 0,
): Promise<ListRunsResponse> {
  return get<ListRunsResponse>(
    `${base(kbId, agentId)}/runs?limit=${limit}&offset=${offset}`,
  );
}

export async function getRun(
  kbId: string,
  agentId: string,
  runId: number,
): Promise<AgentRun> {
  return get<AgentRun>(`${base(kbId, agentId)}/runs/${runId}`);
}

// --- credentials (vault) ---

export async function createCredential(
  kbId: string,
  agentId: string,
  body: CreateCredentialRequest,
): Promise<AgentCredential> {
  return post<AgentCredential>(
    `${base(kbId, agentId)}/credentials`,
    body,
  );
}

export async function listCredentials(
  kbId: string,
  agentId: string,
): Promise<ListCredentialsResponse> {
  return get<ListCredentialsResponse>(
    `${base(kbId, agentId)}/credentials`,
  );
}

export async function deleteCredential(
  kbId: string,
  agentId: string,
  name: string,
): Promise<void> {
  await del<void>(
    `${base(kbId, agentId)}/credentials/${encodeURIComponent(name)}`,
  );
}
