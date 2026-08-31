import { ref, computed, type Ref, type ComputedRef } from 'vue';
import { listAgents } from '@/api/agent';
import type { CustomAgent } from '@/api/agent';
import type { MentionItem } from '@/types/mention';

/**
 * Load agents as mention items for the @-mention selector.
 *
 * Mirrors the kb/file/skill loaders already in Input-field.vue but stays
 * self-contained: returns reactive items + a search helper so it can be
 * composed into the existing mention loader without coupling. The
 * Sprint 1 task backend will reuse the same shape for `task` items.
 */
export interface AgentMentionSource {
  items: Ref<MentionItem[]>;
  loading: Ref<boolean>;
  error: Ref<string | null>;
  load: () => Promise<void>;
  search: (query: string) => MentionItem[];
  total: ComputedRef<number>;
}

function toAgentMentionItem(agent: CustomAgent): MentionItem {
  const mode = (agent as any).config?.agent_mode as 'quick-answer' | 'smart-reasoning' | undefined;
  const type = (agent as any).config?.agent_type as string | undefined;
  const builtin = Boolean((agent as any).is_builtin);
  return {
    id: String(agent.id),
    name: agent.name || String(agent.id),
    type: 'agent',
    description: agent.description || '',
    agentMode: mode,
    agentType: type,
    isBuiltinAgent: builtin,
  };
}

export function useAgentMention(): AgentMentionSource {
  const items = ref<MentionItem[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);

  const load = async () => {
    if (loading.value) return;
    loading.value = true;
    error.value = null;
    try {
      const res: any = await listAgents();
      const list: CustomAgent[] = Array.isArray(res?.data) ? res.data : [];
      items.value = list.map(toAgentMentionItem);
    } catch (e: any) {
      error.value = e?.message || 'Failed to load agents';
      items.value = [];
    } finally {
      loading.value = false;
    }
  };

  const search = (query: string): MentionItem[] => {
    const q = (query || '').trim().toLowerCase();
    if (!q) return items.value;
    return items.value.filter((item) => {
      const haystack = [item.name, item.description || '', item.agentType || '', item.agentMode || '']
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  };

  const total = computed(() => items.value.length);

  return { items, loading, error, load, search, total };
}
