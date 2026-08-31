// Smoke test for parseSSEMessage. The test runner is tsx --test
// (see package.json scripts.test).

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { parseSSEMessage } from './index'

test('metadata', () => {
  const out = parseSSEMessage('metadata', '{"conversation_id":"c-1","answer_id":"a-1","tenant_id":7}')
  assert.equal(out?.type, 'metadata')
  if (out?.type === 'metadata') {
    assert.equal(out.conversationId, 'c-1')
    assert.equal(out.answerId, 'a-1')
    assert.equal(out.tenantId, 7)
  }
})

test('citation', () => {
  const out = parseSSEMessage(
    'citation',
    '{"index":2,"citation":{"type":"kb","id":"k3","title":"Deploy","snippet":"Run kubectl apply.","score":0.91}}',
  )
  assert.equal(out?.type, 'citation')
  if (out?.type === 'citation') {
    assert.equal(out.index, 2)
    assert.equal(out.citation.title, 'Deploy')
    assert.equal(out.citation.type, 'kb')
  }
})

test('token', () => {
  const out = parseSSEMessage('token', '{"text":"hello "}')
  assert.deepEqual(out, { type: 'token', text: 'hello ' })
})

test('done', () => {
  const out = parseSSEMessage('done', '{"prompt_tokens":7,"completion_tokens":3,"finish_reason":"stop"}')
  assert.equal(out?.type, 'done')
  if (out?.type === 'done') {
    assert.equal(out.promptTokens, 7)
    assert.equal(out.completionTokens, 3)
    assert.equal(out.finishReason, 'stop')
  }
})

test('error', () => {
  const out = parseSSEMessage('error', '{"error":"upstream blew up"}')
  assert.deepEqual(out, { type: 'error', error: 'upstream blew up' })
})

test('unknown event returns null', () => {
  assert.equal(parseSSEMessage('whatever', '{}'), null)
})

test('empty data returns null', () => {
  assert.equal(parseSSEMessage('token', ''), null)
})

test('malformed JSON returns null', () => {
  assert.equal(parseSSEMessage('token', 'not-json'), null)
})
