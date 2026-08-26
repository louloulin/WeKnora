<template>
  <TDrawer
    v-model:visible="visible"
    :header="headerTitle"
    :size="`720px`"
    :close-on-esc-key="true"
    :footer="false"
    destroy-on-close
    @open="onOpen"
    @close="onClose"
  >
    <div class="wiki-audit-drawer">
      <div class="wiki-audit-drawer__chips">
        <TCheckbox.Group
          v-model:value="selectedSources"
          theme="primary"
          @change="onSourceChange"
        >
          <TCheckbox
            v-for="source in AllAuditSources"
            :key="source"
            :value="source"
            :label="sourceLabel(source)"
          >
            <span class="wiki-audit-drawer__chip">
              {{ sourceLabel(source) }}
              <span class="wiki-audit-drawer__chip-count">
                {{ sourceCounts[source] ?? 0 }}
              </span>
            </span>
          </TCheckbox>
        </TCheckbox.Group>
      </div>

      <div class="wiki-audit-drawer__filters">
        <TInput
          v-model="filter.actor"
          :placeholder="t('wiki.audit.filterActor')"
          size="small"
          clearable
          style="width: 160px"
          @change="reload"
        />
        <TInput
          v-model="filter.op"
          :placeholder="t('wiki.audit.filterOp')"
          size="small"
          clearable
          style="width: 160px"
          @change="reload"
        />
        <TInput
          v-model="filter.correlation_id"
          :placeholder="t('audit.filterCorrelationId')"
          size="small"
          clearable
          style="width: 200px"
          @change="reload"
        />
        <TButton size="small" theme="default" @click="reload">
          {{ t('common.refresh') }}
        </TButton>
      </div>

      <div v-if="loading" class="wiki-audit-drawer__loading">
        <TSkeleton :row="6" />
      </div>

      <div v-else-if="!events.length" class="wiki-audit-drawer__empty">
        {{ t('wiki.audit.empty') }}
      </div>

      <table v-else class="wiki-audit-drawer__table">
        <thead>
          <tr>
            <th>{{ t('wiki.audit.table.timestamp') }}</th>
            <th>{{ t('wiki.audit.table.source') }}</th>
            <th>{{ t('wiki.audit.table.op') }}</th>
            <th>{{ t('wiki.audit.table.actor') }}</th>
            <th>{{ t('wiki.audit.table.slug') }}</th>
            <th>{{ t('audit.table.correlation') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="ev in events" :key="ev.id">
            <td>{{ formatTimestamp(ev.timestamp) }}</td>
            <td>
              <span :class="['wiki-audit-drawer__source', `is-${badgeKind(ev.op, ev.source)}`]">
                {{ sourceLabel(ev.source) }}
              </span>
            </td>
            <td>{{ opLabel(ev.op) }}</td>
            <td>
              <span :class="['wiki-audit-drawer__actor', `is-${ev.actor_kind}`]">
                {{ shortActor(ev.actor) }}
              </span>
            </td>
            <td>{{ ev.slug ?? '—' }}</td>
            <td>
              <button
                v-if="ev.source_event_id"
                type="button"
                :class="['wiki-audit-drawer__chip-btn', correlationKind(ev.source_event_id)]"
                :title="ev.source_event_id"
                @click="copyCorrelationId(ev.source_event_id)"
              >
                {{ shortCorrelationId(ev.source_event_id) }}
              </button>
              <span v-else class="wiki-audit-drawer__chip-empty">—</span>
            </td>
          </tr>
        </tbody>
      </table>

      <div class="wiki-audit-drawer__pagination">
        <TPagination
          v-model:current="filter.page"
          v-model:page-size="filter.page_size"
          :total="total"
          :page-size-options="[20, 50, 100]"
          size="small"
          show-page-size
          @change="reload"
        />
      </div>
    </div>
  </TDrawer>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  Button as TButton,
  Checkbox as TCheckbox,
  Drawer as TDrawer,
  Input as TInput,
  MessagePlugin,
  Pagination as TPagination,
  Skeleton as TSkeleton,
} from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'

import { useWikiAuditStore } from '../../stores/wikiAudit'
import type { WikiAuditSource } from '../../api/wiki/auditTypes'
import {
  AllAuditSources,
  actorKindLabelSuffix,
  formatAuditTimestamp as formatTimestamp,
  opBadgeKind as badgeKind,
  opI18nKey,
  shortActorId as shortActor,
  sourceLabelI18nKey as sourceLabelKey,
  sourceLabelSuffix,
} from './wikiAuditDrawerLogic'

const props = defineProps<{
  kbId: string
}>()

const { t } = useI18n()
const store = useWikiAuditStore()

const visible = defineModel<boolean>('visible', { default: false })

const events = computed(() => store.eventsFor(props.kbId))
const total = computed(() => store.totalFor(props.kbId))
const sourceCounts = computed(() => store.sourceCountsFor(props.kbId))
const loading = computed(() => store.isLoading(props.kbId))

// selectedSources mirrors the checkbox-group state. Empty selection
// means "all sources"; the server receives no `source` filter so the
// envelope's per-source counts stay accurate.
const selectedSources = ref<WikiAuditSource[]>([...AllAuditSources])

const filter = reactive({
  actor: '',
  op: '',
  correlation_id: '',
  page: 1,
  page_size: 50,
})

const headerTitle = computed(() => t('wiki.audit.drawerTitle'))

function sourceLabel(source: WikiAuditSource): string {
  return t(sourceLabelKey(source))
}

function opLabel(op: string): string {
  const key = opI18nKey(op)
  // vue-i18n returns the key when missing; fall back to the raw op so
  // the row isn't blank.
  const resolved = t(key)
  return resolved === key ? op : resolved
}

// shortCorrelationId trims a long X-Request-ID down to 12 chars + an
// ellipsis. The full id is the button title attribute and is what
// gets copied to the clipboard, so the user sees the truncated form
// in the table but the full value when they paste. 12 keeps the chip
// narrow while staying readable.
function shortCorrelationId(id: string): string {
  if (!id) return ''
  if (id.length <= 14) return id
  return `${id.slice(0, 12)}…`
}

// correlationKind decides the chip colour based on the correlation
// prefix. Build #25 stamps three background prefixes:
//   - sweep:... — cleanup sweeper rows
//   - batch:... — batch worker rows
//   - admin:... — admin scripts / cron
//   Anything else (X-Request-ID, middleware-generated UUID) renders
//   as the default chip colour.
function correlationKind(id: string): string {
  if (id.startsWith('sweep:')) return 'is-sweep'
  if (id.startsWith('batch:')) return 'is-batch'
  if (id.startsWith('admin:')) return 'is-admin'
  return 'is-http'
}

// copyCorrelationId writes the full id to the clipboard via the
// modern Clipboard API and shows a localised success toast. Falls
// back to a deprecated textarea path when the secure context is
// missing (very old browsers, http://localhost in some sandboxes).
async function copyCorrelationId(id: string): Promise<void> {
  if (!id) return
  let copied = false
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(id)
      copied = true
    } else {
      const ta = document.createElement('textarea')
      ta.value = id
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      copied = document.execCommand('copy')
      document.body.removeChild(ta)
    }
  } catch (err) {
    copied = false
  }
  if (copied) {
    MessagePlugin.success(t('audit.chip.copied'))
  } else {
    MessagePlugin.warning(t('audit.chip.copyFailed'))
  }
}

async function reload(): Promise<void> {
  if (!props.kbId) return
  const singleSource =
    selectedSources.value.length === 1 ? selectedSources.value[0] : undefined
  await store.loadAudit(props.kbId, {
    actor: filter.actor || undefined,
    op: filter.op || undefined,
    correlation_id: filter.correlation_id.trim() || undefined,
    source: singleSource,
    page: filter.page,
    page_size: filter.page_size,
  })
}

function onSourceChange(): void {
  filter.page = 1
  reload()
}

function onOpen(): void {
  reload()
}

function onClose(): void {
  // Keep the cache — re-opening the drawer is a no-op until reload.
}

watch(
  () => props.kbId,
  () => {
    if (visible.value) reload()
  },
)

// Expose helpers for unit tests / future i18n callers without
// triggering a re-export chain.
export type { WikiAuditSource }

// Suppress unused-export lint for actorKindLabelSuffix — the chip
// class names are computed inline; the suffix is reserved for the
// next iteration that adds an actor-kind legend.
void actorKindLabelSuffix
void sourceLabelSuffix
</script>

<style scoped>
.wiki-audit-drawer {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding-bottom: 16px;
}

.wiki-audit-drawer__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.wiki-audit-drawer__chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
}

.wiki-audit-drawer__chip-count {
  display: inline-block;
  min-width: 18px;
  padding: 0 6px;
  border-radius: 8px;
  background: var(--td-bg-color-component, #f3f3f3);
  color: var(--td-text-color-secondary, #555);
  font-size: 11px;
  text-align: center;
}

.wiki-audit-drawer__filters {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.wiki-audit-drawer__loading,
.wiki-audit-drawer__empty {
  padding: 32px 0;
  text-align: center;
  color: var(--td-text-color-secondary, #888);
}

.wiki-audit-drawer__table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.wiki-audit-drawer__table th,
.wiki-audit-drawer__table td {
  text-align: left;
  padding: 6px 8px;
  border-bottom: 1px solid var(--td-component-stroke, #e7e7e7);
}

.wiki-audit-drawer__source {
  display: inline-block;
  padding: 0 8px;
  border-radius: 6px;
  font-size: 11px;
  font-weight: 500;
}

.wiki-audit-drawer__source.is-activity {
  background: rgba(28, 122, 255, 0.12);
  color: #1c7aff;
}

.wiki-audit-drawer__source.is-batch {
  background: rgba(255, 152, 0, 0.14);
  color: #c87300;
}

.wiki-audit-drawer__source.is-acl {
  background: rgba(220, 56, 80, 0.14);
  color: #b3243b;
}

.wiki-audit-drawer__source.is-invalidation {
  background: rgba(102, 102, 102, 0.16);
  color: #444;
}

.wiki-audit-drawer__actor.is-system {
  color: #6c757d;
  font-style: italic;
}

.wiki-audit-drawer__actor.is-sweep {
  color: #198754;
}

/* Build #25 — correlation-id chip column. The button is a real
   <button> so screen readers + keyboard nav get focus + enter
   behaviour for free; the onClick handler copies the full id. */
.wiki-audit-drawer__chip-btn {
  display: inline-flex;
  align-items: center;
  padding: 1px 8px;
  border: 1px solid transparent;
  border-radius: 6px;
  background: rgba(28, 122, 255, 0.08);
  color: #1c7aff;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  line-height: 1.6;
  cursor: pointer;
  transition: background-color 0.15s, border-color 0.15s;
}

.wiki-audit-drawer__chip-btn:hover {
  background: rgba(28, 122, 255, 0.16);
  border-color: rgba(28, 122, 255, 0.4);
}

.wiki-audit-drawer__chip-btn:focus-visible {
  outline: 2px solid #1c7aff;
  outline-offset: 1px;
}

.wiki-audit-drawer__chip-btn.is-sweep {
  background: rgba(25, 135, 84, 0.1);
  color: #198754;
}

.wiki-audit-drawer__chip-btn.is-sweep:hover {
  background: rgba(25, 135, 84, 0.18);
  border-color: rgba(25, 135, 84, 0.4);
}

.wiki-audit-drawer__chip-btn.is-batch {
  background: rgba(255, 152, 0, 0.14);
  color: #c87300;
}

.wiki-audit-drawer__chip-btn.is-batch:hover {
  background: rgba(255, 152, 0, 0.24);
  border-color: rgba(255, 152, 0, 0.5);
}

.wiki-audit-drawer__chip-btn.is-admin {
  background: rgba(102, 102, 102, 0.16);
  color: #444;
}

.wiki-audit-drawer__chip-btn.is-admin:hover {
  background: rgba(102, 102, 102, 0.24);
  border-color: rgba(102, 102, 102, 0.4);
}

.wiki-audit-drawer__chip-empty {
  color: var(--td-text-color-placeholder, #aaa);
}

.wiki-audit-drawer__pagination {
  display: flex;
  justify-content: flex-end;
}
</style>