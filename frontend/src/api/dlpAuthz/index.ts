import { get, post, del } from '../../utils/request';
import type {
  CreateDLPPolicyRequest,
  CreateDLPRuleRequest,
  DLPPolicy,
  DLPRule,
  DLPScanRequest,
  DLPScanResponse,
  DLPViolation,
  DiffAuthZResponse,
  ListAuthZKeysResponse,
  ListAuthZVersionsResponse,
  ListDLPPoliciesResponse,
  ListDLPRulesResponse,
  ListViolationsResponse,
  PublishAuthZRequest,
  RollbackAuthZRequest,
  SimulateAuthZRequest,
  SimulateAuthZResponse,
  AuthZPolicyVersion,
} from './types';

// ============ DLP ============

export async function createDLPPolicy(body: CreateDLPPolicyRequest): Promise<DLPPolicy> {
  return post<DLPPolicy>('/api/v1/dlp/policies', body);
}

export async function listDLPPolicies(): Promise<ListDLPPoliciesResponse> {
  return get<ListDLPPoliciesResponse>('/api/v1/dlp/policies');
}

export async function getDLPPolicy(policyId: number): Promise<DLPPolicy> {
  return get<DLPPolicy>(`/api/v1/dlp/policies/${policyId}`);
}

export async function activateDLPPolicy(policyId: number): Promise<{ status: string }> {
  return post<{ status: string }>(`/api/v1/dlp/policies/${policyId}/activate`, {});
}

export async function addDLPRule(
  policyId: number,
  body: CreateDLPRuleRequest,
): Promise<DLPRule> {
  return post<DLPRule>(`/api/v1/dlp/policies/${policyId}/rules`, body);
}

export async function listDLPRules(policyId: number): Promise<ListDLPRulesResponse> {
  return get<ListDLPRulesResponse>(`/api/v1/dlp/policies/${policyId}/rules`);
}

export async function deleteDLPRule(ruleId: number): Promise<{ status: string }> {
  return del<{ status: string }>(`/api/v1/dlp/rules/${ruleId}`);
}

export async function scanDLP(body: DLPScanRequest): Promise<DLPScanResponse> {
  return post<DLPScanResponse>('/api/v1/dlp/scan', body);
}

export async function listViolations(): Promise<ListViolationsResponse> {
  return get<ListViolationsResponse>('/api/v1/dlp/violations');
}

// ============ AuthZ Admin ============

export async function publishAuthZPolicy(body: PublishAuthZRequest): Promise<AuthZPolicyVersion> {
  return post<AuthZPolicyVersion>('/api/v1/authz/policies', body);
}

export async function listAuthZKeys(): Promise<ListAuthZKeysResponse> {
  return get<ListAuthZKeysResponse>('/api/v1/authz/policies');
}

export async function getLatestAuthZ(policyKey: string): Promise<AuthZPolicyVersion> {
  return get<AuthZPolicyVersion>(
    `/api/v1/authz/policies/${encodeURIComponent(policyKey)}`,
  );
}

export async function listAuthZVersions(policyKey: string): Promise<ListAuthZVersionsResponse> {
  return get<ListAuthZVersionsResponse>(
    `/api/v1/authz/policies/${encodeURIComponent(policyKey)}/versions`,
  );
}

export async function getAuthZVersion(versionId: number): Promise<AuthZPolicyVersion> {
  return get<AuthZPolicyVersion>(`/api/v1/authz/versions/${versionId}`);
}

export async function rollbackAuthZ(
  policyKey: string,
  body: RollbackAuthZRequest,
): Promise<AuthZPolicyVersion> {
  return post<AuthZPolicyVersion>(
    `/api/v1/authz/policies/${encodeURIComponent(policyKey)}/rollback`,
    body,
  );
}

export async function simulateAuthZ(body: SimulateAuthZRequest): Promise<SimulateAuthZResponse> {
  return post<SimulateAuthZResponse>('/api/v1/authz/simulate', body);
}

// server-side Diff helper (currently implemented inline in service.Diff;
// the API wrapper stays for parity and to make rollback UX consistent).
export async function diffAuthZVersions(
  fromVersionId: number,
  toVersionId: number,
): Promise<{ from: AuthZPolicyVersion; to: AuthZPolicyVersion; summary: string }> {
  const [from, to] = await Promise.all([
    getAuthZVersion(fromVersionId),
    getAuthZVersion(toVersionId),
  ]);
  let summary = '';
  if (from.decision !== to.decision) {
    summary += `decision ${from.decision} → ${to.decision}; `;
  }
  if (from.expression !== to.expression) {
    summary += `expression diff len(${from.expression.length}) → len(${to.expression.length}); `;
  } else {
    summary += 'expression unchanged; ';
  }
  return { from, to, summary: summary.trim() };
}

export type { DiffAuthZResponse };
