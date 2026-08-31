export type MentionItemType = 'kb' | 'file' | 'tag' | 'mcp' | 'skill' | 'agent' | 'task';

/** TDesign icon for Skills in the agent editor, @-mention menu, and chat chips. */
export const SKILL_ICON = 'system-code';

/** TDesign icon for Agent mentions in the @-mention menu and chat chips. */
export const AGENT_ICON = 'app';

/** TDesign icon for Task mentions in the @-mention menu and chat chips. */
export const TASK_ICON = 'task-checked';

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
  agentMode?: 'quick-answer' | 'smart-reasoning';
  agentType?: string;
  isBuiltinAgent?: boolean;
  taskStatus?: 'open' | 'in_progress' | 'done' | 'cancelled';
  taskAssigneeName?: string;
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
  agent_mode?: 'quick-answer' | 'smart-reasoning';
  task_status?: 'open' | 'in_progress' | 'done' | 'cancelled';
}

/** Returns true if the mention type can launch or open a runtime surface on click. */
export function isRuntimeMention(type: MentionItemType): boolean {
  return type === 'agent' || type === 'skill' || type === 'mcp';
}
