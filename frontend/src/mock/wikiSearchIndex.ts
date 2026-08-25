/**
 * Local mock wiki index for the Build #9-A toolbar search. This is a
 * TEMPORARY scaffold so the sandbox (no Go toolchain) can render the
 * search UI end-to-end. Once the backend exposes a real
 * `GET /api/v1/knowledgebase/:kb_id/wiki/search` route, this file
 * should be deleted and `searchWikiPagesFullText` will hit the
 * network first.
 *
 * 20 sample pages across a fictional product KB. The shape mirrors
 * `WikiSearchResult` so the UI never branches on the data source.
 */

import {
  buildSnippet,
  scoreMatch,
  splitKeywords,
  type ScoredPage,
} from '../utils/wikiSearch'

export interface WikiSearchIndexEntry {
  pageId: string
  slug: string
  title: string
  path: string[]
  content: string
}

export const WIKI_SEARCH_MOCK_PAGES: WikiSearchIndexEntry[] = [
  {
    pageId: 'p-001',
    slug: 'getting-started',
    title: 'Getting Started with WeKnora',
    path: ['Onboarding'],
    content:
      'Welcome to WeKnora. This page walks you through the first 10 minutes: creating a tenant, importing your first knowledge base, and inviting teammates. The meeting cadence is weekly.',
  },
  {
    pageId: 'p-002',
    slug: 'team/weekly-meeting-notes',
    title: 'Weekly Meeting Notes',
    path: ['Team', 'Meetings'],
    content:
      'Rolling notes for the weekly meeting every Monday at 10:00. Attendees discuss roadmap progress, blockers, and share meeting recordings on the wiki.',
  },
  {
    pageId: 'p-003',
    slug: 'team/quarterly-review',
    title: 'Quarterly Review Template',
    path: ['Team', 'Meetings'],
    content:
      'Use this template for the quarterly review. Each section lists the meeting agenda, decisions, and follow-up items. Quarterly review is the highest-signal meeting of the season.',
  },
  {
    pageId: 'p-004',
    slug: 'product/wiki-roadmap',
    title: 'Wiki Module Roadmap',
    path: ['Product', 'Wiki'],
    content:
      'The wiki module roadmap covers full-text search, comments, share links, and ACL. Each milestone has its own tracking issue and meeting recap.',
  },
  {
    pageId: 'p-005',
    slug: 'product/search-design',
    title: 'Search System Design',
    path: ['Product', 'Wiki'],
    content:
      'How the wiki full-text search ranks pages. Title matches weight ten times higher than body matches. Multiple keywords are joined with AND semantics.',
  },
  {
    pageId: 'p-006',
    slug: 'engineering/deployment-guide',
    title: 'Deployment Guide',
    path: ['Engineering', 'Ops'],
    content:
      'Deploy WeKnora to Kubernetes with the bundled Helm chart. The deployment guide covers staging, production rollout, and rollback procedures.',
  },
  {
    pageId: 'p-007',
    slug: 'engineering/api-reference',
    title: 'Wiki REST API Reference',
    path: ['Engineering', 'API'],
    content:
      'Reference for the wiki REST API: list pages, fetch page, search pages, manage folders. All endpoints require the bearer token in the Authorization header.',
  },
  {
    pageId: 'p-008',
    slug: 'engineering/troubleshooting',
    title: 'Troubleshooting Common Issues',
    path: ['Engineering', 'Ops'],
    content:
      'A wiki of troubleshooting recipes for the most common deployment issues. If you cannot find your issue here, open a support ticket and link to a meeting with the on-call engineer.',
  },
  {
    pageId: 'p-009',
    slug: 'kb/onboarding-checklist',
    title: 'New Hire Onboarding Checklist',
    path: ['Onboarding'],
    content:
      'Day-one checklist for new hires. Read the wiki onboarding page, set up the local dev environment, and join the weekly new-hire meeting on Friday.',
  },
  {
    pageId: 'p-010',
    slug: 'kb/code-of-conduct',
    title: 'Code of Conduct',
    path: ['Policy'],
    content:
      'Our code of conduct applies to every meeting, channel, and wiki page. Be respectful, share context, and surface disagreements early.',
  },
  {
    pageId: 'p-011',
    slug: 'product/comments-design',
    title: 'Comments Subsystem Design',
    path: ['Product', 'Wiki'],
    content:
      'Wiki pages support threaded comments with @mentions. The comments subsystem stores one row per comment and resolves mentions against the user directory.',
  },
  {
    pageId: 'p-012',
    slug: 'product/share-links-design',
    title: 'Public Share Links',
    path: ['Product', 'Wiki'],
    content:
      'Share wiki pages publicly via a tokenized URL. Owners can revoke the share link at any time and configure an expiry. The wiki never logs the token in analytics.',
  },
  {
    pageId: 'p-013',
    slug: 'product/acl-design',
    title: 'Page-level Access Control',
    path: ['Product', 'Wiki'],
    content:
      'Each wiki page can be marked private or restricted to an allow list. ACL decisions are evaluated after KB-level membership checks.',
  },
  {
    pageId: 'p-014',
    slug: 'engineering/testing-strategy',
    title: 'Frontend Testing Strategy',
    path: ['Engineering', 'Quality'],
    content:
      'Unit tests cover pure logic with vitest. Component tests mount single-file components with @vue/test-utils. E2E tests run nightly via Playwright.',
  },
  {
    pageId: 'p-015',
    slug: 'engineering/i18n-workflow',
    title: 'Internationalization Workflow',
    path: ['Engineering', 'Frontend'],
    content:
      'Frontend strings live in TypeScript locale files under src/i18n/locales. The i18n completeness check fails the build if any key is missing in zh-CN / en-US / ko-KR / ru-RU.',
  },
  {
    pageId: 'p-016',
    slug: 'product/real-time-collab',
    title: 'Real-time Collaboration (CRDT)',
    path: ['Product', 'Wiki'],
    content:
      'Wiki editing uses Y.js to merge concurrent edits from multiple users. A WebSocket hub broadcasts awareness so the wiki shows who else is in the page.',
  },
  {
    pageId: 'p-017',
    slug: 'team/remote-work-policy',
    title: 'Remote Work Policy',
    path: ['Team', 'Policy'],
    content:
      'Engineers may work remotely up to two days a week. Sync meetings stay in-person when possible; async updates go to the wiki meeting recap.',
  },
  {
    pageId: 'p-018',
    slug: 'kb/glossary',
    title: 'Product Glossary',
    path: ['Reference'],
    content:
      'Glossary of wiki terms: KB, page, folder, revision, ACL, share link. Used by new hires during onboarding and by the weekly all-hands meeting.',
  },
  {
    pageId: 'p-019',
    slug: 'product/release-notes',
    title: 'Release Notes Archive',
    path: ['Product', 'Releases'],
    content:
      'Archived release notes for every shipped version. Each release ships a wiki update, a public changelog, and an internal post-mortem meeting.',
  },
  {
    pageId: 'p-020',
    slug: 'engineering/security-overview',
    title: 'Security Overview',
    path: ['Engineering', 'Security'],
    content:
      'WeKnora enforces tenant isolation, role-based access at the API gateway, and audit logging for every wiki write. Security incidents trigger a war-room meeting.',
  },
]

/**
 * searchWikiIndex is what the API client falls back to when the
 * backend is unreachable. It runs the same scoring / snippet
 * pipeline as the production path so the UI cannot tell whether the
 * data is real or mocked.
 */
export function searchWikiIndex(query: string, limit: number) {
  const keywords = splitKeywords(query)
  if (keywords.length === 0) return []
  const scored: Array<WikiSearchIndexEntry & { score: number; snippet: string }> = []
  for (const entry of WIKI_SEARCH_MOCK_PAGES) {
    const page: ScoredPage = {
      title: entry.title,
      content: entry.content,
      path: entry.path,
    }
    const result = scoreMatch(page, keywords)
    if (result === null) continue
    scored.push({
      ...entry,
      score: result.score,
      snippet: buildSnippet(entry.content, keywords),
    })
  }
  scored.sort((a, b) => b.score - a.score || a.title.localeCompare(b.title))
  return scored.slice(0, limit)
}