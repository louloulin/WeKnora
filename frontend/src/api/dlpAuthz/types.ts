// v0.7.22 — DLP + AuthZ Admin (Microsoft Purview / 飞书安全中心 parity)
//
// Type definitions mirroring the Go structs in
// internal/types/dlp_authz.go.

export type DLPPatternType = 'regex' | 'dictionary' | 'builtin';

export type DLPSeverity = 'low' | 'medium' | 'high' | 'critical';

export type DLPAction = 'log' | 'block' | 'redact' | 'notify_dpo';

export type AuthZDecision = 'allow' | 'deny' | 'conditional';

export interface DLPPolicy {
  id: number;
  tenant_id: number;
  name: string;
  version: number;
  resource_scope: string;
  severity: DLPSeverity;
  action: DLPAction;
  is_active: boolean;
  description: string;
  created_by: number;
  created_at: string;
}

export interface DLPRule {
  id: number;
  policy_id: number;
  tenant_id: number;
  pattern_type: DLPPatternType;
  pattern_value: string;
  severity: DLPSeverity;
  enabled: boolean;
  description: string;
  created_at: string;
}

export interface DLPViolation {
  id: number;
  tenant_id: number;
  policy_id: number;
  rule_id?: number;
  resource: string;
  resource_id: string;
  actor_id: number;
  matched_value: string;
  context: string;
  matched_pattern: string;
  action_taken: DLPAction;
  severity: DLPSeverity;
  audit_log_id?: number;
  created_at: string;
}

export interface DLPScanRequest {
  text: string;
  resource?: string;
  resource_id?: string;
}

export interface DLPScanMatch {
  rule_id?: number;
  pattern_type: DLPPatternType;
  matched_pattern: string;
  matched_value: string;
  context: string;
  severity: DLPSeverity;
  action: DLPAction;
  start: number;
  end: number;
}

export interface DLPScanResponse {
  matches: DLPScanMatch[];
  scanned_chars: number;
  scan_duration_ms: number;
}

export interface CreateDLPPolicyRequest {
  name: string;
  resource_scope?: string;
  severity?: DLPSeverity;
  action?: DLPAction;
  description?: string;
}

export interface CreateDLPRuleRequest {
  pattern_type: DLPPatternType;
  pattern_value: string;
  severity?: DLPSeverity;
  description?: string;
}

export interface ListDLPPoliciesResponse {
  policies: DLPPolicy[];
  total: number;
}

export interface ListDLPRulesResponse {
  rules: DLPRule[];
  total: number;
}

export interface ListViolationsResponse {
  violations: DLPViolation[];
  total: number;
}

// ---- AuthZ Admin ----

export interface AuthZPolicyVersion {
  id: number;
  tenant_id: number;
  policy_key: string;
  version: number;
  expression: string;
  decision: AuthZDecision;
  metadata: string; // JSON-encoded free-form
  created_by: number;
  created_at: string;
}

export interface PublishAuthZRequest {
  policy_key: string;
  expression: string;
  decision: AuthZDecision;
  metadata?: string;
}

export interface RollbackAuthZRequest {
  version_id: number;
}

export interface SimulateAuthZRequest {
  policy_key: string;
  actor: Record<string, unknown>;
  resource?: Record<string, unknown>;
  action?: string;
}

export interface SimulateAuthZResponse {
  decision: AuthZDecision | string;
}

export interface DiffAuthZResponse {
  from_version: number;
  to_version: number;
  expression_changed: boolean;
  decision_changed: boolean;
  summary: string;
}

export interface ListAuthZKeysResponse {
  keys: string[];
  total: number;
}

export interface ListAuthZVersionsResponse {
  versions: AuthZPolicyVersion[];
  total: number;
}

export const DLP_BUILTIN_PATTERNS = [
  { name: 'credit_card', label: '信用卡号 (Visa/MC/Amex/Discover/JCB)' },
  { name: 'id_card_cn', label: '中国大陆身份证' },
  { name: 'ssn_us', label: '美国社会安全号 (SSN)' },
  { name: 'email', label: '电子邮箱' },
  { name: 'phone_cn', label: '中国大陆手机号' },
  { name: 'phone_intl', label: '国际电话号码' },
  { name: 'ip_addr', label: 'IPv4/IPv6 地址' },
] as const;

export type DLPBuiltinName = (typeof DLP_BUILTIN_PATTERNS)[number]['name'];
