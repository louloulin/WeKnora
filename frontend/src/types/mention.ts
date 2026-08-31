export type MentionItemType =
  | 'kb'
  | 'file'
  | 'tag'
  | 'mcp'
  | 'skill'
  // People / runtime kinds — parsed by the backend mention parser
  // (see internal/utils/mentions.go). They are emitted as chat
  // notifications when the recipient is a tenant member; the UI
  // surface for picking them lands in the next iteration.
  | 'user'
  | 'agent'
  | 'task';

/** TDesign icon for Skills in the agent editor, @-mention menu, and chat chips. */
export const SKILL_ICON = 'system-code';

export interface MentionItem {
  id: string;
  name: string;
  type: MentionItemType;
  group?: string;
  description?: string;
  kbType?: 'document' | 'faq';
  count?: number;
  kbName?: string;
  kbId?: string;
  orgName?: string;
  serviceId?: string;
  serviceName?: string;
  skillName?: string;
  isAgentConfigured?: boolean;
}

export interface MentionRequestItem {
  id: string;
  name: string;
  type: MentionItemType;
  kb_type?: 'document' | 'faq';
  kb_id?: string;
  kb_name?: string;
  service_id?: string;
  skill_name?: string;
}
