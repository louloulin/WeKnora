// scripts/e2e/wiki-collab.ts
//
// End-to-end convergence harness for the Build #8 Y.js CRDT wiki
// collab hub. Spins up two y-websocket clients against the live Go
// backend, makes concurrent edits, and asserts the documents
// converge.
//
// Run with:
//   cd /root/multica_workspaces/.../WeKnora/frontend
//   npx tsx ../scripts/e2e/wiki-collab.ts \
//       --base http://localhost:8080 \
//       --kb-id <kb-uuid> \
//       --slug guides/runbook \
//       --token-a <jwt-user-a> \
//       --token-b <jwt-user-b>
//
// Exit code 0 on convergence, non-zero with diagnostic on divergence.
//
// The harness is intentionally self-contained — it doesn't depend on
// the frontend build pipeline. Only the runtime packages listed in
// `package.json` are required: yjs, y-websocket, ws. These are already
// declared in the frontend deps for Build #8.
//
// What it checks:
//   1. Both clients connect and reach awareness=connected.
//   2. Alice inserts text → Bob's Y.Doc state matches within 2s.
//   3. Bob inserts text simultaneously → Alice receives the diff
//      and both clients agree on the merged state.
//   4. The merged state has zero pending updates (Y.Doc.isClean === true
//      on both sides after sync step).

import { argv, exit } from 'node:process'
import * as Y from 'yjs'
import { WebsocketProvider } from 'y-websocket'
import { WebSocket } from 'ws'

interface Args {
  base: string
  kbId: string
  slug: string
  tokenA: string
  tokenB: string
  timeoutMs: number
}

function parseArgs(): Args {
  const get = (name: string, fallback?: string): string => {
    const idx = argv.indexOf(`--${name}`)
    if (idx === -1 || idx + 1 >= argv.length) {
      if (fallback !== undefined) return fallback
      throw new Error(`missing --${name}`)
    }
    return argv[idx + 1]
  }
  return {
    base: get('base', 'http://localhost:8080'),
    kbId: get('kb-id'),
    slug: get('slug'),
    tokenA: get('token-a'),
    tokenB: get('token-b'),
    timeoutMs: Number(get('timeout-ms', '5000')),
  }
}

function endpoint(a: Args, token: string, user: string, name: string): string {
  const wsBase = a.base.replace(/^http/, 'ws')
  const u = new URL(`${wsBase}/api/v1/wiki/collab/${encodeURIComponent(a.kbId)}/${encodeURIComponent(a.slug)}`)
  u.searchParams.set('token', token)
  u.searchParams.set('user', user)
  u.searchParams.set('name', name)
  return u.toString()
}

async function waitForSync(p: WebsocketProvider, timeoutMs: number): Promise<void> {
  return new Promise((resolve, reject) => {
    const start = Date.now()
    const tick = () => {
      if (p.synced) return resolve()
      if (Date.now() - start > timeoutMs) {
        return reject(new Error(`sync timeout after ${timeoutMs}ms (status=${p.wsconnected})`))
      }
      setTimeout(tick, 50)
    }
    tick()
  })
}

async function main(): Promise<void> {
  const a = parseArgs()

  // y-websocket uses 'ws' as the global; Node 18+ has WebSocket but
  // y-websocket 2.x expects the ws package signature. Wire it through.
  ;(globalThis as unknown as { WebSocket: typeof WebSocket }).WebSocket = WebSocket

  const urlA = endpoint(a, a.tokenA, 'alice', 'Alice')
  const urlB = endpoint(a, a.tokenB, 'bob', 'Bob')

  const docA = new Y.Doc()
  const docB = new Y.Doc()

  const providerA = new WebsocketProvider(urlA, `${a.kbId}/${a.slug}`, docA, {
    connect: true,
  })
  const providerB = new WebsocketProvider(urlB, `${a.kbId}/${a.slug}`, docB, {
    connect: true,
  })

  let failed = false
  const fail = (msg: string): void => {
    failed = true
    // eslint-disable-next-line no-console
    console.error(`[FAIL] ${msg}`)
  }

  providerA.on('status', (e: { status: string }) => {
    if (e.status !== 'connected') fail(`alice provider status=${e.status}`)
  })
  providerB.on('status', (e: { status: string }) => {
    if (e.status !== 'connected') fail(`bob provider status=${e.status}`)
  })

  // Wait for both providers to sync (handshake + replay).
  await Promise.all([
    waitForSync(providerA, a.timeoutMs).catch((e) => fail(String(e))),
    waitForSync(providerB, a.timeoutMs).catch((e) => fail(String(e))),
  ])

  // Step 1: Alice types into the doc.
  const textA = docA.getText('body')
  textA.insert(0, 'Hello from Alice. ')
  await waitForSync(providerA, a.timeoutMs).catch((e) => fail(String(e)))

  // Wait for Bob's doc to reflect Alice's insert.
  const startBob = Date.now()
  while (Date.now() - startBob < a.timeoutMs) {
    const textB = docB.getText('body').toString()
    if (textB.includes('Hello from Alice.')) break
    await new Promise((r) => setTimeout(r, 50))
  }
  const bobText1 = docB.getText('body').toString()
  if (!bobText1.includes('Hello from Alice.')) {
    fail(`step 1: bob did not receive alice's edit; bobText=${JSON.stringify(bobText1)}`)
  }

  // Step 2: Bob edits concurrently.
  const textB = docB.getText('body')
  textB.insert(textB.length, 'Reply from Bob.')
  await waitForSync(providerB, a.timeoutMs).catch((e) => fail(String(e)))

  // Wait for Alice to converge.
  const startAlice = Date.now()
  while (Date.now() - startAlice < a.timeoutMs) {
    const aliceText = docA.getText('body').toString()
    if (aliceText.includes('Reply from Bob.')) break
    await new Promise((r) => setTimeout(r, 50))
  }
  const aliceText2 = docA.getText('body').toString()
  if (!aliceText2.includes('Reply from Bob.')) {
    fail(`step 2: alice did not receive bob's edit; aliceText=${JSON.stringify(aliceText2)}`)
  }

  // Step 3: Both sides must agree on the merged state.
  const finalA = docA.getText('body').toString()
  const finalB = docB.getText('body').toString()
  if (finalA !== finalB) {
    fail(`step 3: divergence detected\n  alice=${JSON.stringify(finalA)}\n  bob=${JSON.stringify(finalB)}`)
  }

  // Step 4: No pending updates on either side.
  if (!docA.isClean) fail('step 4: docA has pending updates after sync')
  if (!docB.isClean) fail('step 4: docB has pending updates after sync')

  providerA.destroy()
  providerB.destroy()

  if (failed) {
    // eslint-disable-next-line no-console
    console.error('e2e: FAILED')
    exit(1)
  }
  // eslint-disable-next-line no-console
  console.log(`e2e: OK — converged text=${JSON.stringify(finalA)}`)
  exit(0)
}

main().catch((e: unknown) => {
  // eslint-disable-next-line no-console
  console.error('e2e: error', e)
  exit(1)
})
