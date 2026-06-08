# DocInsight — Lessons Learned

Persistent tracking file for bugs found, decisions made, and pitfalls to avoid.
Updated as the project evolves; survives conversation compaction.

> **⚠️ Architecture changed (2026-06): web app → Wails desktop app.** Everything
> below the "Desktop migration" section was written while DocInsight was a
> 3-process web app (Next.js + chi HTTP API + SSE + Python sidecar). The Go
> *core* lessons (SQLite triggers, RE2 regex, worker pool, FTS5, RRF, snippets,
> folders, the agent loop, LLM SSE parsing, OCR, crawling) are **still accurate
> and still apply** — that code was reused unchanged. But anything about the
> **transport or shell** is historical: there is no longer a Next.js frontend,
> no chi router, no REST endpoints, no CORS, no SSE endpoint, no API-key
> auth/login, and no `NEXT_PUBLIC_API_URL`. Read the **Desktop migration**
> section next for the current model; treat the older transport/auth notes as a
> record of what *was*. The per-phase log is in [`MIGRATION_LOG.md`](MIGRATION_LOG.md).

---

## Desktop migration: web app → Wails v2 (2026-06)

DocInsight was converted from a 3-terminal web app into a **single Wails v2
(v2.12.0) desktop binary** (`build/bin/docinsight.exe`, native WebView2 window
on Windows). The Go core under `internal/` was reused **unchanged**; what
changed is the transport, the entry point, and the frontend toolchain.

### What was removed vs the old app
- **Next.js** frontend → Vite + React 19 + Tailwind 4 + Zustand + react-router-dom (HashRouter) under `frontend/`.
- **chi HTTP server + REST endpoints + CORS** → **Wails bound methods** on `*App` (typed Go methods exposed to JS), split by domain across `app_documents.go`, `app_ingest.go`, `app_search.go`, `app_tags.go`, `app_folders.go`, `app_agent.go`.
- **Server-Sent Events** → **Wails runtime events** (`runtime.EventsEmit` → JS `EventsOn`). `app.go`'s `forwardEvents` bridges the existing `events.Broker` to the runtime, so the Go core's event-publishing code didn't change.
- **API-key auth + login page** → a single implicit **local user** (`local@docinsight.app`), provisioned once on first run. The multi-tenant schema is retained (records carry `user_id`) but there is **no auth**; `a.userID` is threaded through every store call exactly as before.
- **3-terminal startup** → one process. `main.go` embeds `frontend/dist` and binds `App`; `app.go` `startup` wires deps, spawns the sidecar, provisions the user, recovers jobs; `shutdown` tears it all down.
- **`NEXT_PUBLIC_API_URL`** and the whole HTTP base-URL concept → gone; the frontend calls Go in-process.

### Go-native ONNX embeddings: evaluated and dropped
- The tempting "fully self-contained, no Python" path was to run `all-MiniLM-L6-v2` in-process via a Go ONNX runtime. It was **rejected**: the viable Go ONNX bindings pull in a **CGo/native** dependency (onnxruntime), which reintroduces a C toolchain requirement — the exact thing the desktop build was trying to avoid (modernc SQLite is pure Go specifically so there's **no C compiler** in the build).
- **Decision:** keep the **Python sidecar**, but have the app **own its lifecycle**. On startup `app_sidecar.go` picks a free localhost port, spawns the venv's `python -m uvicorn main:app`, polls `/health` (up to 60s — first start loads the model), points `embedder.HTTPEmbedder` at it, and kills it on shutdown. To the user it's invisible; to the build it's C-free. Same model, same 384-dim vectors, zero core changes.

### Sidecar supervision gotchas (`app_sidecar.go`)
- **Locate, don't assume.** `locateSidecarDir` tries `EMBEDDING_SIDECAR_DIR`, then dirs next to the executable (`embedding-sidecar`, `backend/embedding-sidecar`), then the dev tree relative to cwd. This is what lets `wails dev` (cwd = repo) and a future bundled exe (sidecar next to the binary) both work.
- **Free port, not 8000.** Hardcoding `:8000` would collide with a stray dev sidecar or another app. `freePort()` binds `127.0.0.1:0`, reads the assigned port, closes the listener, and passes it to uvicorn — then `cfg.EmbeddingSidecarURL` is overwritten with the real URL before the embedder is constructed.
- **Non-fatal startup.** If the sidecar fails (no venv, missing deps), the window **still opens**; the app emits a `sidecar.error` runtime event and logs "run setup.ps1". Ingest/search simply stay broken until it's up, rather than the whole app refusing to launch.
- **127.0.0.1 only.** uvicorn is bound to loopback, so the sidecar is never reachable off-host — important for the "local-only, no server" privacy story.

### Bindings ↔ frontend type mismatch (the `time.Time` / `uuid.UUID` trap)
- Wails' TS generator emits `number[]` for `uuid.UUID` and `any` for `time.Time`, and prints a harmless `Not found: time.Time` during generation. But at **runtime** those fields marshal to JSON **strings**.
- **Fix:** the frontend ignores the generated models for these shapes. `frontend/src/lib/types.ts` hand-types every id and date as `string`, and `frontend/src/lib/api.ts` casts binding results to those types. Don't try to "fix" the generator — the runtime JSON is correct, only the generated *types* are wrong.
- `api.ts` deliberately re-exposes the **same function names and return shapes** the old fetch-based Zustand store had (`fetchDocuments`, `searchDocuments`, `sendAgentMessage`, …). That's why the ported pages/components needed only mechanical edits (see `MIGRATION_LOG.md` Phase 3), not rewrites.

### Binding-method conventions (held across all `app_*.go`)
- IDs are `string` params parsed with `uuid.Parse`; empty string → `nil` UUID = "none" (e.g. empty `folderID` lists across all folders).
- `a.userID` passed to every store call that accepts a `userID`; **tags are global** and intentionally take none (mirrors the original handler).
- Wrong-user and not-found **collapse to a generic "not found"** so existence isn't leaked.
- No HTTP/JSON envelopes — methods return typed values or domain-prefixed in-file structs (`DocumentsPage`, `DocumentDetail`, `SearchResponse`, …). Errors wrapped with `fmt.Errorf` (`%w` where there's a cause).
- The snippet builder and `searchStopwords` were **re-implemented** in `app_search.go` (copied, not imported) because the originals are unexported in `internal/handler`, which is off-limits to the root `main` package. Kept byte-for-byte faithful.

### Native file picker replaces multipart upload
- `AddDocuments` (in `app_documents.go`) uses `runtime.OpenMultipleFilesDialog` (PDF filter) instead of an HTTP multipart upload. A cancelled dialog returns an empty result + nil error (the frontend treats a 0-doc result as a no-op). Per file: `.pdf` check, size check vs `MaxUploadSizeMB`, copy into the data dir as `<uuid>.pdf`, insert, enqueue. The contract ingests **and** enqueues, so the frontend dropped its separate `processDocument` trigger.

### Data lives in `%APPDATA%\DocInsight`, set at startup
- `config.Load()` still defaults `SQLitePath=./docinsight.db` / `UploadDir=./uploads`, but `app.go` `startup` **overrides** both to live under `os.UserConfigDir()/DocInsight` before opening the store — so the installed app writes to the user's profile, not next to the exe (which may be read-only / Program Files). The old config defaults remain only as the bare-process fallback.

### Tooling / build
- **`setup.ps1`** is the one-time, idempotent bootstrap (PowerShell): sidecar venv + requirements, `go mod download`, install the **Wails CLI pinned to v2.12.0**, `npm install` in `frontend/`. Dev: `wails dev`. Build: `wails build` → `build/bin/docinsight.exe`.
- **Tests split:** `go test ./internal/...` (core) + `go test .` (the binding layer at the repo root); `go vet ./internal/... .`. Frontend: `cd frontend; npm run build` + `npx tsc --noEmit`.
- **Frontend unit tests are not yet re-ported.** The old Vitest suite (happy-dom, `vitest.config.mts`) was removed with the old `src/`. The covered hooks/components were copied into `frontend/` verbatim, so re-porting is mostly path edits + picking a Vitest version compatible with Vite 6 / React 19.
- **Dead code:** `cmd/server` (old HTTP entry) and `internal/server` (chi routes) still exist but are unused by the desktop app — safe to remove later.

---

## Architecture (historical — pre-desktop; see "Desktop migration" above)

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

---

## Review remediation: tests + robustness (2026-06-03)

A deep review of the already-committed voice + agent-tools work (4 parallel
read-only review agents, every load-bearing finding independently re-verified
against source) found **no critical / security / data-leak issues** — tenant
scoping, arg validation, clamps, the nested-summarize key forwarding + bounding,
XSS safety, and citation parsing are all sound. What it surfaced: a few
under-asserting tests, some latent fragilities, and one silently-dropped feature.
This pass fixed the test gaps + robustness items (no new features).

### Test count progression
- Pre: backend **231** / 0 / 0, frontend **14** / 0 / 0
- After: backend **233** / 0 / 0 (+`TestGetDocument_TruncationMultibyte`,
  +`TestListDocuments_DefaultLimit`); frontend **14** / 0 / 0 (3 tests
  *strengthened*, none added). `go vet ./...` clean; `next build` clean.

### Byte-vs-rune truncation — the documented rule, finally enforced
- `get_document`, `summarize_document`, and both snippet builders sliced content
  by **bytes** (`s[:8000]`), which can split a multibyte rune. Mitigated in
  practice only because `json.Marshal` coerces invalid UTF-8 to U+FFFD (no crash,
  no invalid JSON) — but it violated the rune-clamp rule this file already
  documents for snippets.
- Fixed with one helper `capRunes(s, n) (string, bool)` in `tools.go`, used at all
  four sites. Caps by **rune count**, never splits a rune; the ASCII path is
  allocation-free (`len(s) <= n` short-circuit, since byte length bounds rune
  count).
- **Test trap I hit:** my first multibyte test used "é" (2 bytes). The cap (8000)
  is even, so a byte slice lands *exactly* on a rune boundary — that test would
  have passed against the buggy code. Use a **3-byte** rune ("世"): `8000 % 3 ≠ 0`
  guarantees a mid-rune split, so the test actually discriminates. Assert exact
  equality to `strings.Repeat(wide, cap)`, not just `utf8.ValidString` (which is
  tautologically true once json has coerced the split bytes to U+FFFD).

### Tests that passed even when the code was broken
- `TestListDocuments_PaginationCap` seeded **zero** docs and only checked "no
  error" — the 100-cap was dead-code-removable with the test still green. Now
  seeds `maxLimit+1` and asserts exactly `maxLimit` returned + `total` reflects
  the full matching count. Added `TestListDocuments_DefaultLimit` for the
  `limit<=0 → default` branch.
- `TestGetDocument_Truncation` asserted `len <= cap` (an over-truncation to 0
  would pass). Now asserts exact `== cap` (rune count and byte length).
- `TestAgent_MultiToolFlow` asserted tool_result **counts** only. Now asserts
  tool_result **names in order** + each event's `display_label` (the Phase-4 SSE
  contract the frontend renders). Added an `apiKeys []string` recorder to
  `scriptedLLM` so the integration test proves the user's key threads through
  `Run → Dispatch → dispatchSummarizeDocument` (previously only the isolated
  `capturingLLM` unit test observed key forwarding).

### Speech-hook unmount cleanup
- The unmount effect called `r.stop()` (async — defers `onend`, can still deliver
  a final result) and never detached handlers, so the deferred `onend` ran
  `setState` on the unmounted hook. Now detaches `onresult/onerror/onend/onstart`
  **before** calling `r.abort()` (synchronous, discards pending results).
- The old unmount test couldn't catch this: `FakeRecognition.abort()` bumped the
  same `stopCalls` counter as `stop()`. Gave `abort()` a distinct `abortCalls`
  counter; the test now asserts `abortCalls===1 && stopCalls===0` and that all
  four handlers are null post-unmount.

### streamingTools session-switch leak
- `/agent` `page.tsx` reset `streamingText`/`streamingCitations` on a session
  change but not `streamingTools` — switching conversations mid-stream left stale
  tool rows rendering under the new thread. Now resets all three streaming states
  at the top of the session-change effect (covers the `→ null` case too) and
  clears `streamingTools` at the start of each send. No Vitest added: `page.tsx`
  has no unit harness and standing one up is disproportionate; covered by the
  Claude Preview smoke pattern, consistent with the rest of the page.

### Still open — reviewed, deliberately deferred (not in this scope)
- **"Tools used (N)" persisted footer never built.** Phase 4 of the plan
  specified a collapsible footer on each completed assistant message + a Vitest
  test for it. It was silently descoped — tool history is ephemeral and vanishes
  on the `agent.complete` refetch. Would need tool metadata persisted on
  `AgentMessage` (backend + `agent-message.tsx` + the promised test).
- **Positional `tool_result`→row matching** (`page.tsx`): flips "most recent
  not-done" with no name/id correlation. Fine under serial dispatch; mismatches
  if an SSE `tool_result` drops (which the broker tolerates by design). Fix = a
  stable `tool_call_id` carried on both events, matched on it.
- **Permission-error auto-clear** ("clears after 5s" in the plan) is unimplemented
  — the error persists until the next `start()`.
- **Hook mock fidelity**: `FakeRecognition.emitResult` hard-codes `resultIndex:0`,
  so it doesn't model Chrome's cumulative-result/`resultIndex` advance.
- **Unreachable `dispatchErr` branch** (`agent.go`) skips the `agent.tool_result`
  event; harmless today (all tools return nil err) but would strand a UI row if a
  tool ever returned a real error.

