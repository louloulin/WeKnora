# Wiki CRDT Collab — End-to-End Harness

Build #8 ships a Y.js CRDT real-time collaboration hub on the Go
backend. The frontend prototype is committed on branch
`lumos0826-collab`. Before merging back to `lumos0826`, run this
harness to confirm two clients converge on the same document state
through the live WebSocket.

## Prerequisites

1. **Go toolchain** (≥ 1.22) — for the backend WS hub under
   `internal/handler/wiki_collab.go`.
2. **Node.js** (≥ 18) and **tsx** — for the harness script. Install
   with `npm install -g tsx` if you don't have it.
3. **A running WeKnora instance** with the `lumos0826-collab` branch
   checked out. The simplest path is:
   ```bash
   ./scripts/dev.sh
   ```
4. **Migration `000094_wiki_collab_snapshots` applied**. The dev
   script runs migrations on boot. If you boot manually:
   ```bash
   ./scripts/migrate.sh up
   ```

## Backend unit tests (sanity check)

```bash
cd /path/to/WeKnora
go test ./internal/handler/ -run TestWikiCollab -count=1 -v
```

Expected: four tests pass — `TestWikiCollabHubFanout`,
`TestWikiCollabRoomGCAfterLastLeave`, `TestWikiCollabRoomIsolation`,
`TestWikiCollabSnapshotReplay`.

## E2E harness

The harness connects two y-websocket clients to the live Go hub,
makes concurrent edits, and asserts the documents converge.

### 1. Mint two JWTs

The hub requires a valid Bearer token in `?token=`. Easiest path:

```bash
# Log in two test users via the auth API, capture access_token.
TOKEN_A=$(curl -sX POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"<password>"}' | jq -r .access_token)

TOKEN_B=$(curl -sX POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"bob","password":"<password>"}' | jq -r .access_token)
```

### 2. Find a wiki KB + slug

```bash
KB_ID=$(curl -sX GET http://localhost:8080/api/v1/knowledgebase \
  -H "Authorization: Bearer $TOKEN_A" | jq -r '.[] | select(.wiki_enabled) | .id' | head -1)
```

Pick any existing page slug, e.g. `guides/runbook`. If the page
doesn't exist yet, create one through the wiki REST API or via the
frontend.

### 3. Run the harness

```bash
cd /path/to/WeKnora/frontend
npx tsx ../scripts/e2e/wiki-collab.ts \
  --base http://localhost:8080 \
  --kb-id "$KB_ID" \
  --slug guides/runbook \
  --token-a "$TOKEN_A" \
  --token-b "$TOKEN_B" \
  --timeout-ms 8000
```

### 4. Expected output

```
e2e: OK — converged text="Hello from Alice. Reply from Bob."
```

Exit code `0`. Non-zero exit + diagnostics on divergence.

## What the harness verifies

| Step | Assertion |
| ---- | --------- |
| 1    | Both providers reach `status=connected` |
| 2    | Both providers reach `synced=true` after the initial handshake (replay of any persisted snapshot) |
| 3    | Alice's edit propagates to Bob within `--timeout-ms` |
| 4    | Bob's concurrent edit propagates back to Alice within `--timeout-ms` |
| 5    | `Y.Doc.getText('body').toString()` is byte-identical on both sides |
| 6    | `Y.Doc.isClean === true` on both sides (no pending local updates) |

## Known limitations (Build #8 prototype)

- **No snapshot compaction.** The server keeps an append-only buffer
  per room. Long-running sessions accumulate frames. Build #9+ will
  replace this with `Y.encodeStateAsUpdate` snapshots once we have a
  Go port of yjs encoders (or a sidecar Node worker).
- **In-memory store by default.** The default snapshot store is
  process-local. Multi-instance deployments will see divergent
  snapshot buffers per replica. Switch to `SQLWikiCollabSnapshotStore`
  via the wiring layer (see `internal/handler/wiki_collab_snapshot.go`)
  when running multi-replica.
- **No `Origin` enforcement.** y-protocol uses an `origin` byte at the
  start of each message; we forward them opaquely. CRDT convergence
  is unaffected because all clients see the same updates, but
  per-origin policy (e.g. block the awareness channel) is not yet
  implemented.
- **No horizontal scaling.** The hub is single-process. For HA, the
  future SQL-backed store + a Redis pub/sub fan-out channel are the
  intended path.

## Cleanup

After the harness completes, the CRDT buffer remains in the hub's
in-memory map until the last client disconnects. To clear it
explicitly:

```bash
# Wait 30s for the debounce timer to flush the snapshot, then
# manually delete the row (if you used SQL store):
sqlite3 ./data/weknora.db "DELETE FROM wiki_collab_snapshots WHERE room_key = '...';"
```

Or simply restart the backend.
