/**
 * Async iterator helper that lazily fetches every page of a paginated
 * list endpoint. Mirrors the Notion SDK pattern.
 */
export interface PageEnvelope<T> {
  items: T[];
  next_page_token?: string;
}

export interface PageFetcher<T> {
  (pageToken: string | undefined): Promise<PageEnvelope<T>>;
}

export async function* iteratePages<T>(fetcher: PageFetcher<T>): AsyncIterable<T> {
  let token: string | undefined;
  while (true) {
    const page = await fetcher(token);
    for (const item of page.items) yield item;
    if (!page.next_page_token) return;
    token = page.next_page_token;
  }
}
