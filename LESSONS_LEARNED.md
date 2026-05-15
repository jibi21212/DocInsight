# DocInsight — Lessons Learned

Persistent tracking file for bugs found, decisions made, and pitfalls to avoid.
Updated as the project evolves; survives conversation compaction.

---

## Architecture

| Decision | Why |
|---|---|
| Next.js (frontend only) + Go backend (`:8080`) | Decouples UI from processing; Go gives real concurrency via goroutines |
| SQLite for local dev, PostgreSQL/Supabase for prod | Zero-setup local testing; auto-selected by checking `DATABASE_URL` |
| Python FastAPI embedding sidecar (`:8000`) | Same `all-MiniLM-L6-v2` model as the original `@xenova/transformers`; batches efficiently |
| In-memory channel-based job queue | Simplest approach; DB-backed recovery on restart for crash resilience |
| Frontend uses `NEXT_PUBLIC_API_URL=http://localhost:8080` | All fetch calls prefixed with `API_BASE`; old Next.js API routes deleted |

---

## Critical Bugs Found & Fixed

### 1. SQLite `updated_at` trigger causes infinite recursion (CRITICAL)
- **File:** `backend/internal/store/sqlite.go`
- **Bug:** `AFTER UPDATE` trigger did `UPDATE documents SET updated_at = datetime('now') WHERE id = OLD.id` — this fires itself in an infinite loop, crashing the server on any document update.
- **Fix:** Removed the trigger entirely. Added `updated_at = datetime('now')` directly to each UPDATE SQL statement (`UpdateDocumentStatus`, `UpdateDocumentPageCount`).
- **Lesson:** SQLite `AFTER UPDATE` triggers that modify the same table/row will re-fire themselves. Use `BEFORE UPDATE` with `NEW.updated_at` or handle timestamps in application code.

### 2. Go regexp engine doesn't support lookbehinds (CRITICAL — server won't start)
- **File:** `backend/internal/chunker/chunker.go`
- **Bug:** `regexp.MustCompile("(?<=[.!?])\\s+")` panics at init time because Go's RE2 engine doesn't support lookbehinds. Server crashes immediately on startup.
- **Fix:** Changed pattern to `([.!?])\s+` (capture the punctuation) and rewrote `splitIntoSentences()` to use `FindAllStringIndex` and manually re-attach punctuation to each sentence.
- **Lesson:** Go's `regexp` package uses RE2 syntax — no lookbehinds, lookaheads, or backreferences. Always validate regex patterns from other languages (JS/Python) when porting to Go.

### 3. Worker pool never closes job channel — shutdown hangs forever
- **File:** `backend/internal/worker/pool.go`
- **Bug:** `Shutdown()` called `cancel()` but never closed the channel. Workers block on `<-channel` and `wg.Wait()` hangs indefinitely.
- **Fix:** Added `q.Close()` method (with `sync.Once` to prevent double-close panics) and call it in `Shutdown()` before waiting.

### 4. Retry off-by-one error — last retry never happens
- **File:** `backend/internal/worker/processor.go`
- **Bug:** `if job.Attempts < job.MaxRetries-1` skips the final retry. With `MaxRetries=3`, only 2 retries happen.
- **Fix:** Changed to `if job.Attempts < job.MaxRetries`.

### 5. Ignored error on GetDocument after upload
- **File:** `backend/internal/handler/documents.go`
- **Bug:** `doc, _ = h.store.GetDocument(...)` — if this fails, `doc` becomes nil and the response contains null fields.
- **Fix:** Check error; fall back to the original `doc` struct if re-fetch fails.

---

## Web Page Ingestion Feature

### Design Decision: Reuse Existing Pipeline
- Web pages are treated as another document source type — only Stage 1 (extraction) changes.
- Stages 2-6 (chunking, embedding, storage, search) are reused identically for PDFs and web pages.
- `source_type` column (`"pdf"` or `"web"`) added to documents table; defaults to `"pdf"` for backward compatibility.
- Raw HTML saved to disk (like PDF uploads), enabling re-processing without re-fetching.

### Key Implementation Details
- **Scraper:** Uses `go-shiori/go-readability` (Go port of Mozilla Readability.js) to extract article content from web pages. Strips nav, footer, scripts, ads.
- **`go-readability` deprecation:** The `github.com/go-shiori/go-readability` module shows a deprecation notice pointing to a Codeberg fork. Still works fine and compiles cleanly.
- **Processor branching:** `processor.go` checks `doc.SourceType` — if `"web"`, calls `scraper.ExtractFromHTML()` instead of `extractor.Extract()`. Nil-safe: falls back to PDF path if scraper is nil.
- **Ingest endpoint:** `POST /api/documents/ingest` accepts `{"urls": [...]}`. Validates URL schemes (http/https only), respects `MaxIngestURLs` limit.
- **Text sectioning:** `splitIntoSections()` accumulates paragraph blocks into ~2000 char sections to produce meaningful page-equivalent units for the chunker.

### Pitfall: SQLite ALTER TABLE idempotency
- `ALTER TABLE ... ADD COLUMN` fails if column already exists. Wrapped in error check: `if !strings.Contains(err.Error(), "duplicate column")` for safe re-runs.

---

## Hybrid Search (FTS5 + Semantic)

### Design
- SQLite FTS5 virtual table (`chunks_fts`) mirrors chunk content for full-text search
- Three search modes: `hybrid` (default), `semantic`, `keyword`
- Hybrid uses **Reciprocal Rank Fusion** (RRF): `score = 1/(60+rank_semantic) + 1/(60+rank_keyword)` to merge rankings
- FTS5 BM25 returns **negative** scores — must `math.Abs()` before displaying

### FTS5 Sync
- `InsertChunks` inserts into both `chunks` and `chunks_fts` in the same transaction
- `DeleteChunksByDocumentID` deletes from `chunks_fts` before deleting from `chunks`
- Backfill on first migration: inserts existing chunks into FTS5 if count is 0

### Pitfall: FTS5 MATCH syntax
- FTS5 `MATCH` can fail on special characters (e.g., colons, hyphens). HybridSearch falls back to semantic-only if keyword search errors.

---

## Environment & Tooling Notes

### Go not in PATH (Windows)
- Go is installed at `C:\Program Files\Go\bin\go.exe` but not in the default bash PATH.
- Must prepend: `export PATH="/c/Program Files/Go/bin:$PATH"` before any `go` command.

### Running the Go server
- Start: `cd backend && go run ./cmd/server/`
- Logs appear in the terminal (human-readable `slog.TextHandler`)
- Default port: 8080; default SQLite path: `./docinsight.db`
- Kill orphaned servers: `taskkill //IM server.exe //F` (Windows)

### Next.js version
- Uses Next.js 16.2.3 with breaking changes from training data.
- Must read guides in `node_modules/next/dist/docs/` before writing Next.js code.
- pdf-parse v2 uses class-based API: `new PDFParse({data})`, `.getText()`, `.getInfo()`.

---

## Test Coverage

| Package | Tests | Status |
|---|---|---|
| `internal/config` | 5 | PASS |
| `internal/queue` | 7 | PASS |
| `internal/chunker` | 9 | PASS |
| `internal/store` | 31 | PASS |
| `internal/handler` | 44 | PASS |
| `internal/embedder` | 8 | PASS |
| `internal/worker` | 7 | PASS |
| `internal/scraper` | 8 | PASS |
| `internal/ocr` | 7 | PASS |
| `internal/crawler` | 10 | PASS |
| `internal/events` | 6 | PASS |
| **Total** | **155** | **ALL PASS** |

Run all: `go test ./... -timeout 60s`

---

## OCR Pipeline

- **Tesseract** binary required as system dependency — optional, detected at startup via `Available()`.
- OCR fallback triggers when `totalChars / fileSize < OCRMinTextRatio` (default 0.02).
- If Tesseract is not found, `ocrProc` is set to `nil` — processor skips OCR gracefully.
- Config: `OCR_ENABLED`, `TESSERACT_PATH`, `OCR_MIN_TEXT_RATIO`.

## Recursive Web Crawling

- BFS crawler with depth and page count limits.
- Same-domain only — external links are filtered out.
- Static asset URLs (`.css`, `.js`, `.png`, `.pdf`, etc.) are skipped via `shouldSkipURL`.
- URL normalization removes fragments and trailing slashes to avoid duplicates.
- Frontend shows "Crawl linked pages" checkbox when a single URL is entered.

## SSE Notifications

- Event broker uses Go channels with buffer of 16 per subscriber.
- Slow consumers have events dropped (non-blocking publish).
- Processor publishes `document.completed` and `document.failed` events.
- Frontend `useSSE` hook uses `EventSource` with 5-second reconnect on error.
- Toast notifications auto-dismiss after 5 seconds, max 5 visible.

## User Auth + Multi-tenancy

- Auth is opt-in: `AUTH_ENABLED=true` (default `false` for backward compatibility).
- API keys generated via `crypto/rand` with `di_` prefix (64 hex chars).
- Auth middleware extracts Bearer token, looks up user, injects into context.
- SSE endpoint and `/api/auth/register` are exempt from auth checks.
- Frontend stores API key in `localStorage`, includes in all fetch calls.
- `/me` endpoint returns user info without the API key for security.

---

## Remaining Work / Known Issues

- [ ] Frontend: URL ingestion UI (tab/toggle on upload page, textarea for URLs)
- [ ] Frontend: Globe icon for web sources on document cards, source type badges on search results
- [ ] End-to-end test: Next.js UI → Go backend → upload → process → search → delete
- [ ] Embedding sidecar must be running for document processing to succeed
- [ ] No health check of sidecar at startup — workers fail silently if it's down
- [ ] Retry goroutines during processor backoff are not tracked — potential leak on shutdown
- [ ] No upper bound on `pageSize` query param (could request huge pages)
- [ ] `process.go` response uses camelCase `documentId` while models use snake_case `document_id`
- [x] Frontend integration testing (does the React app talk to Go correctly?) — verified via E2E test suites
- [ ] PostgreSQL `match_embeddings` RPC function not yet updated for `source_type`/`source_url` fields

---

## File Reference

Key files and what they do:

| File | Purpose |
|---|---|
| `backend/cmd/server/main.go` | Entry point, DI wiring, graceful shutdown, startup recovery |
| `backend/internal/config/config.go` | Env-based config with defaults |
| `backend/internal/store/store.go` | Store interface (15 methods) |
| `backend/internal/store/sqlite.go` | SQLite implementation with in-memory embedding index |
| `backend/internal/store/postgres.go` | PostgreSQL/pgvector implementation |
| `backend/internal/handler/documents.go` | Upload, List, GetByID, Delete handlers |
| `backend/internal/handler/process.go` | Enqueue processing job |
| `backend/internal/handler/search.go` | Semantic search handler |
| `backend/internal/chunker/chunker.go` | Paragraph-aware text chunking |
| `backend/internal/worker/pool.go` | N-goroutine worker pool |
| `backend/internal/worker/processor.go` | 6-stage document processing pipeline |
| `backend/internal/queue/queue.go` | Channel-based job queue |
| `backend/internal/embedder/http_embedder.go` | HTTP client to Python sidecar |
| `backend/internal/pdf/ledongthuc.go` | PDF text extraction |
| `backend/internal/scraper/scraper.go` | Scraper interface + ScrapeResult type |
| `backend/internal/scraper/readability.go` | Web scraping via go-readability |
| `backend/internal/handler/ingest.go` | URL ingestion endpoint with optional crawl mode |
| `backend/internal/handler/tags.go` | Tag CRUD handler |
| `backend/internal/handler/search.go` | Hybrid/semantic/keyword search handler |
| `backend/internal/handler/auth.go` | User registration, auth middleware, `/me` endpoint |
| `backend/internal/handler/sse.go` | Server-Sent Events stream handler |
| `backend/internal/ocr/ocr.go` | Tesseract OCR processor with sparse text detection |
| `backend/internal/crawler/crawler.go` | BFS web crawler with depth/page limits |
| `backend/internal/events/broker.go` | Pub/sub event broker for SSE |
| `backend/internal/model/tag.go` | Tag model |
| `backend/internal/model/user.go` | User model |
| `backend/embedding-sidecar/main.py` | FastAPI embedding server |
| `src/store/app-store.ts` | Frontend Zustand store with auth headers on all API calls |
| `src/lib/auth-context.tsx` | React auth context (login, register, logout) |
| `src/hooks/use-sse.ts` | SSE EventSource hook with auto-reconnect |
| `src/components/sse-provider.tsx` | SSE → toast notification bridge |
| `src/components/toast-notification.tsx` | Toast notification UI |
| `src/components/tag-manager.tsx` | Document tag management dropdown |
| `src/components/export-toolbar.tsx` | JSON/CSV export for search results |
| `src/app/login/page.tsx` | Login/registration page |
| `.env.local` | `NEXT_PUBLIC_API_URL=http://localhost:8080` |
| `.claude/launch.json` | MCP preview server configs (frontend + backend) |
| `e2e/reset-test-db.sh` | Test environment reset script |
| `e2e/E2E_TEST_PLAN.md` | E2E test playbook (9 suites, Claude-executable) |

---

## E2E Testing

### Setup
- Uses **Claude Preview MCP tools** (`preview_start`, `preview_snapshot`, `preview_click`, `preview_fill`, `preview_eval`, `preview_logs`, `preview_network`) — no npm test dependencies
- Backend started via Bash (`go build` + background process), not MCP `preview_start` (WSL bash conflict on Windows)
- `.claude/launch.json` defines frontend (npm run dev, port 3000) and backend (Git Bash + Go, port 8080) configs
- Backend logs captured to `test-logs/backend.log` via `tee` and verified with `grep`

### Bug Found During E2E: chi middleware ordering
- **File:** `backend/internal/server/routes.go`
- **Bug:** `r.Get("/health", ...)` was defined before `r.Use(handler.AuthMiddleware(...))`. chi panics with `"all middlewares must be defined before routes on a mux"`.
- **Fix:** Moved all `r.Use()` calls above the first route definition.
- **Lesson:** In chi, `Use()` MUST come before any `Get()`/`Post()`/`Route()` calls on the same mux. Unlike Express.js, chi enforces this at runtime with a panic.

### Key E2E Findings
- All 9 suites pass (startup, dashboard, upload, search, doc detail, navigation, login, SSE, responsive)
- Backend: 0 ERROR-level logs, 0 5xx responses across 33 log lines
- Frontend: only stale "Failed to fetch" errors from before backend was started (expected)
- SSE connections log `status=0` — correct for long-lived streaming connections
- Auth `/api/auth/me` returns 401 before login (expected, no token)
- Registration creates `di_`-prefixed API key, header updates to show user name

---

## Snippets, Folders & BYO-LLM Agent (2026-05-14)

### Test count progression
- Baseline: **155** passing, 0 skipped
- After pre-work (user_id enforcement on documents): **155** (no new tests added, existing tests updated with nil userID)
- After Snippets: **166** (+11 snippet extraction + handler tests)
- After Folders: **194** (+28 folder CRUD + recursive descendant + scoped-search tests)
- After BYO-LLM Agent: **209** (+15 llm streaming parser + agent loop + handler tests)
- **Final: 209 passing / 0 skipped / 0 failed**

### Snippet extraction algorithm gotchas
- File: `backend/internal/handler/snippet.go`
- Tokenize query: lowercase, drop tokens shorter than 2 chars, drop a small stopword set (`the`, `and`, `for`, `with`, ...). If all tokens are stopwords, **fall back** to the leading window — don't return empty.
- Earliest-match wins: scan content case-insensitively for any query token; pick the lowest offset across all tokens.
- Window: `content[max(0, offset-50) : min(len, offset+windowSize-50)]` — centers the match with ~50 chars of leading context.
- Edge ellipsis: prepend `…` (U+2026, **one char** not three dots) iff the window starts after byte 0; append `…` iff window ends before end of content. Using three ASCII dots inflates length and confuses downstream highlight tokenizers.
- Tokens for the UI: return the deduped, non-stopword query tokens as `highlight_tokens` so the frontend can wrap each occurrence with `<mark>` — the frontend never re-tokenizes.
- Byte vs rune indexing: SQLite text is UTF-8; we use byte offsets throughout. A multibyte char split at a window boundary produces invalid UTF-8 — clamp to nearest rune boundary before slicing.

### Recursive CTE pitfalls (folders)
- SQLite's `WITH RECURSIVE tree(id) AS (SELECT id FROM folders WHERE id = ? UNION ALL SELECT f.id FROM folders f JOIN tree t ON f.parent_id = t.id)` returns the seed folder plus all descendants in a single query — used by `FolderDescendants()` to scope document lists and search to a folder subtree.
- **Cycle detection**: we don't currently enforce a DAG. `CreateFolder` only validates `parent_id` exists and belongs to the same user; if we ever expose a "move folder" API that rewrites `parent_id`, a malicious or buggy client could create A→B→A. SQLite's recursive CTE has no built-in cycle break, so the query would loop until SQLITE_LIMIT_DEPTH (default 1000) — slow but not catastrophic. **If folder reparenting ships, add cycle check**: walk ancestor chain before update and reject if target descendant of source.
- `ON DELETE CASCADE` on `folders(parent_id)` cleans up the subtree; `ON DELETE SET NULL` on `documents.folder_id` keeps documents alive but "unfiled" — the dashboard's "All documents" view still shows them.

### SSE backpressure (agent streaming)
- `backend/internal/events/broker.go` uses **non-blocking sends** (`select { case ch <- evt: default: drop }`) — slow consumers silently lose events.
- Agent text deltas can arrive at >100 events/sec when the LLM is streaming a long response. If the EventSource client (browser tab in background, throttled JS event loop) reads slower, deltas get dropped and the streamed text in the UI looks truncated — but on `agent.complete` we **refetch** the full message from `/api/agent/sessions/{id}/messages`, so the final state is always correct.
- The UI shows mid-stream text optimistically (best-effort) and replaces it with the authoritative persisted message on complete. Don't try to "fix" backpressure by adding per-client buffers — drops are acceptable because we have a refetch fallback.

### API key transport: header vs cookie
- Chose `X-LLM-API-Key` header per-request over a cookie or session-bound storage.
- **Why header**: keys never need to persist server-side. No DB column for an encrypted key, no rotation endpoint, no "leaked key" incident response — if the user clears their browser, the key is gone. The header only lives in memory for the duration of one request; the LLM client uses it then we drop the reference.
- **Why not cookie**: cookies imply server lifecycle (set/clear endpoints, expiry policy), and `HttpOnly` would force a roundtrip just to populate the agent request — defeating the simplicity.
- **Why not URL param**: keys would land in server access logs, proxy logs, and browser history. Headers don't.
- Client side: `src/lib/llm-key-storage.ts` reads/writes `localStorage["docinsight_llm_key_anthropic"|"docinsight_llm_key_openai"]`. The settings modal warns explicitly that the key is stored only in this browser and forwarded per-request.

### LLM streaming parser notes
- Anthropic SSE: events are `event: <name>\ndata: <json>\n\n`. Parser switches on event name (`content_block_start`, `content_block_delta`, `content_block_stop`, `message_stop`). Tool calls arrive as `content_block_start` with `type=tool_use` followed by `input_json_delta` chunks that **must be concatenated** before JSON-parsing.
- OpenAI SSE: events are `data: <json>\n\n` with a terminal `data: [DONE]`. Tool calls arrive as `delta.tool_calls[i].function.arguments` chunks indexed by `i` — we accumulate into a map keyed by `index`, not by `id` (id only arrives on the first chunk).
- Both clients expose a `BaseURL` field overridable in tests so `httptest.NewServer` can stub the upstream API.

### chi router additions
- `/api/folders` (GET/POST), `/api/folders/{id}` (DELETE)
- `/api/documents/{id}/move` (POST `{folder_id}`)
- `/api/agent/sessions` (GET/POST), `/api/agent/sessions/{id}` (DELETE)
- `/api/agent/sessions/{id}/messages` (GET/POST — POST returns 202, agent runs in goroutine and publishes SSE)

### File reference additions
| File | Purpose |
| --- | --- |
| `backend/internal/handler/snippet.go` | Query tokenizer + earliest-match windowing for search snippets |
| `backend/internal/handler/folders.go` | Folder CRUD + move-document endpoint |
| `backend/internal/handler/agent.go` | Agent session CRUD + SendMessage (validates X-LLM-API-Key header) |
| `backend/internal/model/folder.go` | Folder struct |
| `backend/internal/model/agent.go` | AgentSession, AgentMessage, Citation + JSON marshal helpers |
| `backend/internal/llm/client.go` | Client interface, Message/Tool/StreamEvent types, NewClient factory |
| `backend/internal/llm/anthropic.go` | Anthropic Messages API SSE client |
| `backend/internal/llm/openai.go` | OpenAI Chat Completions SSE client |
| `backend/internal/agent/agent.go` | Tool-calling agent loop with citation extraction |
| `src/lib/llm-key-storage.ts` | localStorage wrapper for per-provider API keys |
| `src/components/folder-tree.tsx` | Sidebar recursive folder tree with inline create/delete |
| `src/components/folder-picker.tsx` | Modal for "Move document to folder" |
| `src/components/settings-llm-keys.tsx` | Settings modal for Anthropic/OpenAI key entry |
| `src/components/agent-message.tsx` | Renders `<cite chunk="..."/>` markers as numbered superscripts with expandable source list |
| `src/app/agent/page.tsx` | Full agent chat UI (sessions sidebar + streaming message thread) |

---

## Spinoff: docinsight-cli (2026-05-14)

A separate repo `C:\Users\jibkh\Personal_Projects\docinsight-cli\` indexes
git repos for semantic code search and exposes an **MCP stdio server** for
AI CLI agents (Claude Code, Codex, Copilot CLI).

**Patterns reused from DocInsight:**
- modernc/sqlite + FTS5 + brute-force cosine in Go
- RRF hybrid fusion at k=60
- BYO API-key model (key in env / per-request header; never persisted)
- Stopword-aware query tokenization for snippets/baseline

**Code NOT reused** — too tightly coupled to the document/HTTP-sidecar
world:
- DocInsight chunker is PDF-page-aware; CLI chunker is symbol-aware
  (Go `go/parser` for `.go` files, line-window fallback for others)
- DocInsight embedder is an HTTP client to a Python sidecar; CLI embedder
  is an in-process OpenAI client
- DocInsight schema is user/folder-indexed; CLI schema is branch-indexed

**Headline metric: token efficiency for AI agents.** Claim: a single
`semantic_search_code` MCP call (~2–3 KB out) replaces 5–10 grep + read
iterations (~20–50 KB) — measurable as a 10–25× reduction. The CLI ships
with a Phase 7.5 eval framework (`docinsight-cli eval`) that computes
Precision@1/3/5 + output bytes per query against a labeled benchmark
([`docinsight-cli/eval/queries.jsonl`](../docinsight-cli/eval/queries.jsonl)
is a 25-query benchmark targeting **this** DocInsight repo).

See [`../docinsight-cli/LESSONS_LEARNED.md`](../docinsight-cli/LESSONS_LEARNED.md)
for CLI-specific gotchas (MCP notification handling, FTS5 sync triggers,
modernc/sqlite quirks, etc.).

### MCP wire-up

To use the CLI against this repo, from this directory:
```bash
cd ../docinsight-cli && go install ./cmd/docinsight-cli
cd -                   # back to DocInsight
docinsight-cli init    # one-time per repo
OPENAI_API_KEY=sk-... docinsight-cli index
```
Then add to `.claude/mcp.json` here:
```jsonc
{
  "mcpServers": {
    "code-search": {
      "command": "docinsight-cli",
      "args": ["mcp"],
      "env": { "OPENAI_API_KEY": "sk-..." }
    }
  }
}
```
Claude Code will then call `semantic_search_code(query)` instead of running
its own grep loops, surfacing ranked chunks from this codebase.

---

## Voice input + agent tools expansion (2026-05-15)

### Test count progression
- Pre: backend **209** / 0 / 0 (last verified d3d08ca)
- After Phase 1 (4 new agent tools + GetChunkByID in store): backend **231** / 0 / 0
- After Phase 2 (Vitest setup, smoke tests): + **2** frontend tests
- After Phase 3 (speech-recognition hook + tests): + **8** frontend tests (10 total)
- After Phase 4 (ToolStatus component + test): + **4** frontend tests (**14** total)
- **Final: backend 231 / 0 / 0, frontend 14 / 0 / 0**

### Agent tools dispatcher pattern
- New file [backend/internal/agent/tools.go](backend/internal/agent/tools.go) owns tool spec registration AND dispatch in one place. `agent.go` is now focused on the LLM loop; adding tool #6 means editing one switch case and one `Specs()` slice.
- The dispatcher returns a `displayLabel` per call; the agent publishes it on both `agent.tool_call` and `agent.tool_result` SSE events. Frontend consumes via `streamingTools` state in `/agent/page.tsx`.
- `summarize_document` does a **nested LLM call** with the user's API key, no tools, length-mapped system prompt. Bounded sub-step — doesn't recurse into the main agent loop. Each summarize call adds one round-trip and a few hundred output tokens.
- Cross-tenant access: we treat wrong-user-id as **not-found**, not 403. Matches the existing search path and avoids leaking existence across users.

### Citation behavior decision
- `Citation.ChunkID` kept non-nullable. `summarize_document` doesn't emit citations — its return payload carries `source_document_id` + `source_title` instead, which the agent narrates in prose. Document-level citations would require schema change + UI rework, deferred until there's a real consumer.

### Web Speech API gotchas
- `SpeechRecognition` is **not** in the default `lib.dom.d.ts` (still marked experimental upstream). Added [src/types/speech-recognition.d.ts](src/types/speech-recognition.d.ts) with the minimal subset used.
- Chrome/Edge/Safari support; Firefox needs a polyfill we don't ship. Hook returns `isSupported: false` and the MicButton renders disabled with a tooltip explaining why.
- Audio is processed inside the browser-vendor layer (Chrome forwards to Google servers; Safari is on-device). DocInsight never sees audio bytes. The tooltip and lessons doc are explicit about this.
- `no-speech` and `aborted` errors fire after ~2s silence — treat as graceful stop, NOT as errors. Surfacing them would spam the user.
- `onresult` fires many times with growing `transcript[i].isFinal=false` chunks; only commit on `isFinal=true`. The hook tracks `transcript` (final) and `interimTranscript` (in-progress) separately.

### MicButton commit pattern
- The hook accumulates final transcripts; the button uses a `lastCommitted` ref to compute the *delta* and forward only new text to `onTranscript(text)`. Without the diff, every utterance would re-emit everything previously said.
- On `isListening === false`, the button calls `reset()` to clear both the hook's internal transcript and the committed cache — so the next start session begins fresh.

### Vitest setup: jsdom → happy-dom swap
- Vitest 4.1 + jsdom 27.x ships an ESM/CJS mismatch: jsdom's CSS subsystem (`@asamuzakjp/css-color`) is CJS but `require()`s ESM-only `@csstools/css-calc`, blowing up Vitest's CJS worker pool with `ERR_REQUIRE_ESM`. Switching the test environment to **happy-dom** sidesteps the entire CSS dependency tree (we don't need full CSS in tests anyway).
- Config file is `vitest.config.mts` (note `.mts`) — `.ts` config files cause Vitest's own config loader to hit the same ERR_REQUIRE_ESM when reading certain ESM-only deps. The `.mts` extension forces Node to treat it as ESM at load time.
- happy-dom is also ~3× faster than jsdom for component tests — net win.

### chi router additions: none
This batch did not add new HTTP endpoints. The new agent tools all dispatch through the existing `/api/agent/sessions/{id}/messages` flow; the SSE event payload gained a `display_label` field, no schema change.

### File reference additions
| File | Purpose |
| --- | --- |
| `backend/internal/agent/tools.go` | Tool registration + dispatcher for all 5 agent tools |
| `backend/internal/agent/tools_test.go` | Per-tool unit tests + scriptedLLM patterns |
| `src/types/speech-recognition.d.ts` | Minimal TS typings for the Web Speech API |
| `src/hooks/use-speech-recognition.ts` | React hook wrapping `SpeechRecognition` with state machine + cleanup |
| `src/hooks/use-speech-recognition.test.ts` | Vitest unit tests for the hook (8 cases) |
| `src/components/mic-button.tsx` | Pulsing-while-listening mic button used in search + agent |
| `src/components/tool-status.tsx` | Inline status rows for in-flight + completed agent tool calls |
| `src/components/tool-status.test.tsx` | Vitest component tests |
| `vitest.config.mts` | happy-dom environment, `pool: "vmThreads"`, `@/*` alias |
| `src/test/setup.ts` | Loads `@testing-library/jest-dom/vitest` matchers |
| `src/test/smoke.test.ts` | Sanity check that the runner + jest-dom matchers work |

### Known smoke-test limitation
- Claude Preview MCP runs headless Chrome without microphone permission, so clicking the mic button does not transition to a listening state in automated smoke. We verified: button renders, has correct aria-label, `'webkitSpeechRecognition' in window` is true, click doesn't throw or break the page. **Real-browser manual verification required** to see the full listening flow.

### Commits
- `7204281` — Phase 1: 4 new agent tools
- `8f093de` — Phase 2: Vitest + happy-dom setup
- `295d093` — Phase 3: voice input via Web Speech API
- `38aea0f` — Phase 4: render tool calls inline in agent UI

