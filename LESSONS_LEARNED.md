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
