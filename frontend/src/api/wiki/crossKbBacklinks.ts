// v0.7.25 Build #28 — Cross-KB Backlinks API client.
//
// Fetches pages that link to the current wiki page from any other
// knowledge base owned by the caller's tenant. Used by the extended
// WikiBacklinksPanel to render a per-KB accordion alongside the
// existing direct / indirect / related / broken sections.

import { get } from "../../utils/request";

export interface WikiBacklinkCrossKB {
  slug: string;
  title: string;
  page_type: string;
  status: string;
  updated_at: string;
  knowledge_base_id: string;
}

export interface WikiBacklinkCrossKBGroup {
  knowledge_base_id: string;
  kb_name: string;
  backlinks: WikiBacklinkCrossKB[];
  total: number;
}

export interface WikiBacklinkCrossKBResponse {
  groups: WikiBacklinkCrossKBGroup[];
  total: number;
}

export interface CrossKbBacklinksOptions {
  limit?: number;
}

export async function listCrossKbBacklinks(
  kbId: string,
  slug: string,
  options: CrossKbBacklinksOptions = {},
): Promise<WikiBacklinkCrossKBResponse> {
  const params: Record<string, string> = {};
  if (options.limit !== undefined) params.limit = String(options.limit);
  return get<WikiBacklinkCrossKBResponse>(
    `/api/v1/knowledgebase/${encodeURIComponent(kbId)}/wiki/pages/${encodeURIComponent(slug)}/backlinks/cross-kb`,
    { params },
  );
}
