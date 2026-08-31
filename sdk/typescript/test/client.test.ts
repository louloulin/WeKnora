import { describe, it, expect } from 'vitest';
import { WeKnoraClient } from '../src/client.js';

class FakeResponse {
  constructor(public status: number, public body: string) {}
  get ok() { return this.status >= 200 && this.status < 300; }
  async text() { return this.body; }
  async json() { return JSON.parse(this.body); }
  async arrayBuffer() { return new TextEncoder().encode(this.body).buffer; }
}

function makeFetch(responses: FakeResponse[]): typeof fetch {
  let i = 0;
  return (async (_url: any, _init?: any) => {
    const r = responses[Math.min(i++, responses.length - 1)];
    return r as any;
  });
}

describe('WeKnoraClient', () => {
  it('requires baseURL', () => {
    expect(() => new WeKnoraClient({ baseURL: '', auth: { kind: 'bearer', token: 'x' } })).toThrow(/baseURL/);
  });
  it('requires auth', () => {
    expect(() => new WeKnoraClient({ baseURL: 'https://x', auth: undefined as any })).toThrow(/auth/);
  });
  it('creates a knowledge base', async () => {
    const c = new WeKnoraClient({
      baseURL: 'https://api.test/v1',
      auth: { kind: 'bearer', token: 't' },
      fetch: makeFetch([new FakeResponse(201, JSON.stringify({ id: 'kb-1', name: 'Eng', type: 'rag' }))]),
    });
    const kb = await c.knowledgeBase.create({ name: 'Eng', type: 'rag' });
    expect(kb.id).toBe('kb-1');
  });
  it('evaluates a formula', async () => {
    const c = new WeKnoraClient({
      baseURL: 'https://api.test/v1',
      auth: { kind: 'apiKey', key: 'k' },
      fetch: makeFetch([new FakeResponse(200, JSON.stringify({ value: 110, type: 'number' }))]),
    });
    const r = await c.formula.eval('kb-1', { expression: 'price * 1.1', context: { price: 100 } });
    expect(r.value).toBe(110);
  });
});
