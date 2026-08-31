# @weknora/sdk

Official TypeScript SDK for the WeKnora Enterprise Knowledge Platform.

## Install

```bash
npm install @weknora/sdk
```

## Usage

```typescript
import { WeKnoraClient } from '@weknora/sdk';

const client = new WeKnoraClient({
  baseURL: process.env.WEKNORA_BASE_URL ?? 'https://api.weknora.com/v1',
  auth: { kind: 'bearer', token: process.env.WEKNORA_TOKEN! },
});

const kb = await client.knowledgeBase.create({ name: 'Engineering', type: 'rag' });
const hits = await client.search.search(kb.id!, { query: 'vector search', top_k: 5 });
const ask = await client.chat.ask(kb.id!, { question: 'What is pgvector?' });
console.log(ask.answer, ask.citations);
```

## Streaming chat

```typescript
for await (const chunk of client.chat.stream('kb-1', { messages: [{ role: 'user', content: 'hello' }] })) {
  if (chunk.type === 'delta') process.stdout.write(chunk.content ?? '');
}
```

## Auth modes

- `bearer` — JWT access token (interactive sessions).
- `apiKey` — service-to-service `X-API-Key`.

## License

Apache-2.0
