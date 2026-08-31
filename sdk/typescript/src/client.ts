import { WeKnoraError } from './errors.js';
import type {
  KnowledgeBase, KnowledgeBaseInput, KnowledgeBasePatch, KnowledgeBasePage,
  SearchRequest, SearchResponse, AskRequest, AskResponse,
  ChatRequest, ChatChunk,
  Database, DatabaseInput, Row, Row as RowType, RowInput,
  FormulaEvalRequest, FormulaEvalResponse,
  Automation, AutomationInput, CollabDoc, CollabDocInput, CollabDocFile,
  Agent, AgentInput, AgentRun,
  Connector, ConnectorInput,
  VerificationRequest, VerificationReport,
} from './types.js';

export type Auth = { kind: 'bearer'; token: string } | { kind: 'apiKey'; key: string };

export interface WeKnoraClientOptions {
  baseURL: string;
  auth: Auth;
  fetch?: typeof fetch;
  maxRetries?: number;
}

const DEFAULT_MAX_RETRIES = 3;

function applyAuth(headers: Headers, auth: Auth): void {
  if (auth.kind === 'bearer') {
    headers.set('Authorization', `Bearer ${auth.token}`);
  } else {
    headers.set('X-API-Key', auth.key);
  }
}

async function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * WeKnoraClient is the typed entry point for the WeKnora REST API. It
 * exposes one sub-service per REST surface (knowledge bases, search,
 * chat, ...).
 */
export class WeKnoraClient {
  readonly baseURL: string;
  private readonly auth: Auth;
  private readonly fetchImpl: typeof fetch;
  private readonly maxRetries: number;

  // Sub-services (lazy-instantiated to keep NewClient cheap).
  readonly knowledgeBase: KnowledgeBaseAPI;
  readonly search: SearchAPI;
  readonly chat: ChatAPI;
  readonly database: DatabaseAPI;
  readonly formula: FormulaAPI;
  readonly automation: AutomationAPI;
  readonly collabDoc: CollabDocAPI;
  readonly agentStudio: AgentStudioAPI;
  readonly connector: ConnectorAPI;
  readonly verification: VerificationAPI;

  constructor(opts: WeKnoraClientOptions) {
    if (!opts.baseURL) throw new Error('WeKnora: baseURL is required');
    if (!opts.auth) throw new Error('WeKnora: auth is required');
    this.baseURL = opts.baseURL.replace(/\/$/, '');
    this.auth = opts.auth;
    this.fetchImpl = opts.fetch ?? globalThis.fetch;
    if (!this.fetchImpl) {
      throw new Error('WeKnora: fetch is unavailable; pass opts.fetch or use Node 18+');
    }
    this.maxRetries = opts.maxRetries ?? DEFAULT_MAX_RETRIES;
    this.knowledgeBase = new KnowledgeBaseAPI(this);
    this.search = new SearchAPI(this);
    this.chat = new ChatAPI(this);
    this.database = new DatabaseAPI(this);
    this.formula = new FormulaAPI(this);
    this.automation = new AutomationAPI(this);
    this.collabDoc = new CollabDocAPI(this);
    this.agentStudio = new AgentStudioAPI(this);
    this.connector = new ConnectorAPI(this);
    this.verification = new VerificationAPI(this);
  }

  /** Internal: execute a request and decode JSON, with retries. */
  async request<T>(method: string, path: string, body?: unknown, query?: Record<string, string>): Promise<T> {
    const url = new URL(this.baseURL + path);
    if (query) {
      for (const [k, v] of Object.entries(query)) {
        url.searchParams.set(k, v);
      }
    }
    let lastErr: unknown;
    for (let i = 0; i < this.maxRetries; i++) {
      try {
        const headers = new Headers({ 'Content-Type': 'application/json' });
        applyAuth(headers, this.auth);
        const init: RequestInit = { method, headers };
        if (body !== undefined) init.body = JSON.stringify(body);
        const resp = await this.fetchImpl(url, init);
        if (resp.status >= 200 && resp.status < 300) {
          if (resp.status === 204) return undefined as T;
          const text = await resp.text();
          return text ? (JSON.parse(text) as T) : (undefined as T);
        }
        const errBody = await resp.text();
        lastErr = new WeKnoraError(errBody || resp.statusText, resp.status, resp.statusText);
        if (resp.status >= 400 && resp.status < 500 && resp.status !== 429) {
          throw lastErr;
        }
      } catch (err) {
        lastErr = err;
      }
      await sleep(200 * Math.pow(2, i));
    }
    throw lastErr;
  }
}

class KnowledgeBaseAPI {
  constructor(private readonly c: WeKnoraClient) {}
  async list(pageSize = 20, pageToken?: string): Promise<KnowledgeBasePage> {
    return this.c.request<KnowledgeBasePage>('GET', '/knowledge-bases', undefined, {
      page_size: String(pageSize),
      ...(pageToken ? { page_token: pageToken } : {}),
    });
  }
  async get(kbId: string): Promise<KnowledgeBase> {
    return this.c.request<KnowledgeBase>('GET', `/knowledge-bases/${kbId}`);
  }
  async create(input: KnowledgeBaseInput): Promise<KnowledgeBase> {
    return this.c.request<KnowledgeBase>('POST', '/knowledge-bases', input);
  }
  async update(kbId: string, patch: KnowledgeBasePatch): Promise<KnowledgeBase> {
    return this.c.request<KnowledgeBase>('PATCH', `/knowledge-bases/${kbId}`, patch);
  }
  async delete(kbId: string): Promise<void> {
    await this.c.request<void>('DELETE', `/knowledge-bases/${kbId}`);
  }
}

class SearchAPI {
  constructor(private readonly c: WeKnoraClient) {}
  async search(kbId: string, req: SearchRequest): Promise<SearchResponse> {
    return this.c.request<SearchResponse>('POST', `/knowledge-bases/${kbId}/search`, req);
  }
}

class ChatAPI {
  constructor(private readonly c: WeKnoraClient) {}
  async ask(kbId: string, req: AskRequest): Promise<AskResponse> {
    return this.c.request<AskResponse>('POST', `/knowledge-bases/${kbId}/ask`, req);
  }
  async *stream(kbId: string, req: ChatRequest): AsyncIterable<ChatChunk> {
    const headers = new Headers({ 'Content-Type': 'application/json', Accept: 'application/x-ndjson' });
    applyAuth(headers, this.c['auth']);
    const resp = await this.c['fetchImpl'](`${this.c.baseURL}/knowledge-bases/${kbId}/chat`, {
      method: 'POST', headers, body: JSON.stringify(req),
    });
    if (!resp.body) throw new Error('WeKnora: stream response has no body');
    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      let nl = buf.indexOf('\n');
      while (nl >= 0) {
        const line = buf.slice(0, nl).trim();
        buf = buf.slice(nl + 1);
        if (line) yield JSON.parse(line) as ChatChunk;
        nl = buf.indexOf('\n');
      }
    }
  }
}

class DatabaseAPI {
  constructor(private readonly c: WeKnoraClient) {}
  async list(kbId: string): Promise<Database[]> {
    return this.c.request<Database[]>('GET', `/knowledge-bases/${kbId}/databases`);
  }
  async create(kbId: string, input: DatabaseInput): Promise<Database> {
    return this.c.request<Database>('POST', `/knowledge-bases/${kbId}/databases`, input);
  }
  async insertRows(kbId: string, databaseId: string, rows: RowInput[]): Promise<Row[]> {
    return this.c.request<Row[]>('POST', `/knowledge-bases/${kbId}/databases/${databaseId}/rows`, { rows });
  }
  async queryRows(kbId: string, databaseId: string, filter: string): Promise<Row[]> {
    return this.c.request<Row[]>('GET', `/knowledge-bases/${kbId}/databases/${databaseId}/rows`, undefined, { filter });
  }
}

class FormulaAPI {
  constructor(private readonly c: WeKnoraClient) {}
  async eval(kbId: string, req: FormulaEvalRequest): Promise<FormulaEvalResponse> {
    return this.c.request<FormulaEvalResponse>('POST', `/knowledge-bases/${kbId}/formula/eval`, req);
  }
}

class AutomationAPI {
  constructor(private readonly c: WeKnoraClient) {}
  async list(kbId: string, databaseId: string): Promise<Automation[]> {
    return this.c.request<Automation[]>('GET', `/knowledge-bases/${kbId}/databases/${databaseId}/automations`);
  }
  async create(kbId: string, input: AutomationInput): Promise<Automation> {
    return this.c.request<Automation>('POST', `/knowledge-bases/${kbId}/databases/${input.database_id}/automations`, input);
  }
  async run(kbId: string, automationId: string, payload?: Record<string, unknown>): Promise<Record<string, unknown>> {
    return this.c.request<Record<string, unknown>>('POST', `/knowledge-bases/${kbId}/automations/${automationId}/run`, payload);
  }
}

class CollabDocAPI {
  constructor(private readonly c: WeKnoraClient) {}
  async list(): Promise<CollabDoc[]> {
    return this.c.request<CollabDoc[]>('GET', '/collaborative-docs');
  }
  async create(input: CollabDocInput): Promise<CollabDoc> {
    return this.c.request<CollabDoc>('POST', '/collaborative-docs', input);
  }
  async uploadBytes(docId: string, data: Uint8Array, contentType = 'application/octet-stream'): Promise<CollabDocFile> {
    const form = new FormData();
    const file = new Blob([data], { type: contentType });
    form.append('file', file, 'blob');
    const headers = new Headers();
    applyAuth(headers, this.c['auth']);
    const resp = await this.c['fetchImpl'](`${this.c.baseURL}/collaborative-docs/${docId}/upload`, {
      method: 'POST', headers, body: form,
    });
    if (!resp.ok) throw new WeKnoraError(await resp.text(), resp.status, resp.statusText);
    return resp.json() as Promise<CollabDocFile>;
  }
  async downloadBytes(docId: string): Promise<Uint8Array> {
    const headers = new Headers();
    applyAuth(headers, this.c['auth']);
    const resp = await this.c['fetchImpl'](`${this.c.baseURL}/collaborative-docs/${docId}/download`, { headers });
    if (!resp.ok) throw new WeKnoraError(await resp.text(), resp.status, resp.statusText);
    const buf = await resp.arrayBuffer();
    return new Uint8Array(buf);
  }
}

class AgentStudioAPI {
  constructor(private readonly c: WeKnoraClient) {}
  async list(): Promise<Agent[]> {
    return this.c.request<Agent[]>('GET', '/agents');
  }
  async create(input: AgentInput): Promise<Agent> {
    return this.c.request<Agent>('POST', '/agents', input);
  }
  async run(agentId: string, input: Record<string, unknown>): Promise<AgentRun> {
    return this.c.request<AgentRun>('POST', `/agents/${agentId}/runs`, { input });
  }
}

class ConnectorAPI {
  constructor(private readonly c: WeKnoraClient) {}
  async list(): Promise<Connector[]> {
    return this.c.request<Connector[]>('GET', '/connectors');
  }
  async install(input: ConnectorInput): Promise<Connector> {
    return this.c.request<Connector>('POST', '/connectors', input);
  }
  async sync(connectorId: string): Promise<void> {
    await this.c.request<void>('POST', `/connectors/${connectorId}/sync`);
  }
}

class VerificationAPI {
  constructor(private readonly c: WeKnoraClient) {}
  async verify(kbId: string, req: VerificationRequest): Promise<VerificationReport> {
    return this.c.request<VerificationReport>('POST', `/knowledge-bases/${kbId}/verify`, req);
  }
}
