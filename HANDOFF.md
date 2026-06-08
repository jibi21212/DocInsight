# DocInsight — Agent / Developer Handoff

**Purpose:** if you're starting a fresh session (Claude Code, Codex, Copilot CLI, or a new human contributor) and want to pick up DocInsight work without losing context, read this file first.

> **Heads-up:** DocInsight was recently converted from a 3-process web app (Next.js + Go API + Python sidecar) into a **single Wails v2 desktop app**. If you have training-data or memory context describing the old Next.js / chi-HTTP / SSE architecture, **discard it** — none of that is current. The per-phase conversion is logged in [`MIGRATION_LOG.md`](MIGRATION_LOG.md).

---

## What DocInsight is

DocInsight is a **semantic document search + AI agent desktop application**, built with **Wails v2 (v2.12.0)**: a Go core and a React/Tailwind UI rendered in a native **WebView2** window. It ships as a single binary, `build/bin/docinsight.exe`. There is no web server, no browser, and no login.

- **Shell:** Wails v2.12.0. `main.go` embeds `frontend/dist`, sets up the window, and binds the `App` struct. The frontend talks to Go via **generated bindings**, not HTTP.
- **Go core:** reused unchanged under `internal/` (module `github.com/docinsight/backend`, rooted at the repo). modernc.org/sqlite (pure Go, **no CGo**), FTS5 + brute-force cosine + RRF (k=60), a channel-based worker pool/queue with crash recovery, scraper/crawler, OCR, LLM clients, the agent loop, and an in-process event broker.
- **Bindings layer:** `app_*.go` files at the repo root (package `main`) expose typed methods on `*App`. `app.go` owns startup/shutdown + DI wiring + local-user provisioning + the event bridge. `app_sidecar.go` supervises the Python sidecar.
- **Frontend:** `frontend/` — Vite + React 19 + Tailwind 4 + Zustand + react-router-dom (HashRouter). `frontend/src/lib/api.ts` wraps the generated bindings under the same function names the old fetch store used; `frontend/src/hooks/use-events.ts` consumes Wails runtime events.
- **Embedding sidecar:** `backend/embedding-sidecar/` — FastAPI/uvicorn + sentence-transformers `all-MiniLM-L6-v2` (384-dim). The app spawns it on a free localhost port at startup, health-checks `/health`, and kills it on shutdown. **Required** for ingest/search.
- **BYO-LLM agent:** the user supplies their own Anthropic or OpenAI key (stored in the WebView's `localStorage`, forwarded per request as the `llmApiKey` arg to `SendAgentMessage`). Never persisted server-side.
- **Single local user:** a `local@docinsight.app` user is provisioned on first run. The multi-tenant schema is retained (every record carries `user_id`) but there is **no auth** — `a.userID` is threaded through every store call.
- **Data dir:** `%APPDATA%\DocInsight` (`docinsight.db` + `uploads/`), resolved via `os.UserConfigDir()`.

---

## Current state

| Metric | Value |
|---|---|
| Go core tests (`go test ./internal/...`) | passing |
| Binding-layer tests (`go test .`) | passing |
| `go vet ./internal/... .` | clean |
| Frontend (`cd frontend; npm run build` + `npx tsc --noEmit`) | clean |
| Frontend unit tests (Vitest) | **not yet re-ported** to `frontend/` (see below) |
| Branch | `wails-desktop-migration` |

### Implemented (carried over the migration)

All previously-shipped features work in the desktop app: bulk PDF import (now via a native file picker), web-page ingestion + recursive crawl, hybrid/semantic/keyword search with highlighted snippets, hierarchical folders + tags, OCR fallback, the BYO-LLM agent with 5 tools and inline citations, live tool-status rows, voice input, and JSON/CSV export. The migration replaced the transport (HTTP→bindings, SSE→runtime events) and the entry point (chi server→Wails), not the feature set.

The screens live in `frontend/src/pages/` — **Dashboard** (`/`, document grid + folder tree + inline search), **AddContent** (`/upload`, native PDF picker + URL ingest/crawl), **Search** (`/search`), **Agent** (`/agent`, sessions sidebar + streaming thread + LLM-key settings), and **DocumentDetail** (`/documents/:id`). The nav header lives in `frontend/src/components/header.tsx`.

---

## What's left / known gaps

### Should do
- **Re-port the frontend unit tests.** The old Vitest suite (happy-dom, `vitest.config.mts`) was removed along with the old `src/` tree. The hooks/components it covered (`use-speech-recognition`, `tool-status`, etc.) were copied into `frontend/` verbatim, so the tests should re-port with mostly path edits. Pick a Vitest version compatible with Vite 6 / React 19; re-add a `test`/`test:run` script to `frontend/package.json`.
- **Finish distribution packaging.** The app locates `backend/embedding-sidecar` relative to cwd/exe (and honours `EMBEDDING_SIDECAR_DIR`), and `setup.ps1` covers run-from-source. Running on a machine **without the repo** isn't packaged yet: the venv/sidecar needs to be bundled next to the exe (e.g. a Wails NSIS installer step that ships `embedding-sidecar/` + a prebuilt venv, or a frozen sidecar binary). This is the main remaining item in "Phase 5 — packaging & polish".

### Cleanup (safe, low-risk)
- **Remove the dead HTTP path.** `cmd/server` (old entry point) and `internal/server` (chi routes) still exist but are unused by the desktop app. They compile but nothing references them at runtime. Safe to delete once you're confident nothing else imports them.
- The Wails TS-binding generator prints a harmless `Not found: time.Time` — handled by `frontend/src/lib/types.ts`, which hand-types ids and dates as `string` to match the real runtime JSON (Wails would otherwise emit `number[]` for `uuid.UUID` and `any` for `time.Time`). Don't "fix" the generator; the adapter in `api.ts` casts binding results to these types.

### Feature ideas (from the web version, still valid)
These are unchanged by the migration — the pattern lives in the Go core, not the transport:
- TTS for agent replies (`SpeechSynthesis`), symmetric to voice input.
- `compare_documents` agent tool (pattern exists in `summarize_document`).
- Faceted search sidebar (tag/folder/date/source); data is already on the model.
- Cross-encoder re-ranking of the top-N hybrid hits.
- Audio/video transcription (would need a Whisper sidecar — user previously deferred; re-confirm).
- Comments + highlights on documents; saved searches; scheduled re-crawl.

### Explicitly deferred / out of scope
- Backend-driven STT (Whisper) for voice input — user declined this path.
- `sqlite-vec` for vector search — brute force handles current scale.
- Tree-sitter chunking — too heavy a dependency.
- Re-introducing any HTTP server / REST API — the app is intentionally local-only now.

---

## How to run / test

### Prereqs
- Go 1.23+, Node 20+, Python 3.11+
- WebView2 runtime (preinstalled on Windows 11)
- Tesseract (optional, OCR fallback)
- **No C compiler** — modernc SQLite is pure Go, and there's no ONNX/CGo path.

### One-time setup
```powershell
./setup.ps1
```
Idempotent. Provisions the sidecar venv + requirements, downloads Go deps, installs the **Wails v2 CLI (v2.12.0)**, and runs `npm install` in `frontend/`. If `wails` isn't on `PATH` afterwards, add `GOPATH\bin` (the script prints the path).

### Dev loop
```powershell
wails dev        # recompiles Go + serves the Vite dev frontend inside the window, hot-reload
```

### Build
```powershell
wails build      # → build\bin\docinsight.exe
```

### Tests
```powershell
go test ./internal/...      # Go core
go test .                   # Wails binding layer (app_*.go)
go vet ./internal/... .     # static analysis

cd frontend
npm run build               # Vite production build
npx tsc --noEmit            # type-check
```

> **Windows / Go-not-on-PATH:** if `go` isn't found, it's typically at `C:\Program Files\Go\bin\go.exe`. `setup.ps1` already falls back to that path; for ad-hoc commands prepend it to `$env:PATH`.

---

## Architecture cheat sheet

```
                  docinsight.exe  (single process)
   ┌──────────────────────────┐                 ┌──────────────────────┐
   │ WebView2 window           │   App.*()       │  Go core (internal/) │
   │ React 19 / Tailwind 4     │ ──────────────▶ │  store · chunker     │
   │ Zustand · HashRouter      │   bindings      │  worker · queue      │
   │                           │                 │  scraper · crawler   │
   │ pages/  components/        │ ◀────────────── │  ocr · agent loop    │
   │ lib/api.ts (binding wrap) │  runtime events │  llm clients         │
   │ hooks/use-events.ts       │                 │  events.Broker       │
   └──────────────────────────┘                 └───┬───────────┬──────┘
                                       embeddings    │      data │   chat
                                  (spawned+supervised)│           │
                            ┌────────────────────────▼┐ ┌────────▼──────┐ ┌──────────────┐
                            │ Python sidecar          │ │ SQLite        │ │ Anthropic /  │
                            │ FastAPI/uvicorn          │ │ %APPDATA%\    │ │ OpenAI       │
                            │ MiniLM-L6-v2 (384d)     │ │ DocInsight    │ │ (BYO key)    │
                            └─────────────────────────┘ └───────────────┘ └──────────────┘
```

### Key files
| Path | What |
|---|---|
| [`main.go`](main.go) | Wails entry: `//go:embed all:frontend/dist`, window options, `Bind: []interface{}{app}` |
| [`app.go`](app.go) | `startup`/`shutdown`, DI wiring, `forwardEvents` (broker→runtime), `ensureLocalUser`, `appDataDir` |
| [`app_sidecar.go`](app_sidecar.go) | `startSidecar` (locate dir, pick free port, spawn uvicorn, poll `/health`), `stop` |
| [`app_documents.go`](app_documents.go) | `ListDocuments`/`GetDocument`/`DeleteDocument`/`MoveDocument`/`ProcessDocument`/`RefreshDocument`/`AddDocuments` |
| [`app_ingest.go`](app_ingest.go) | `IngestURLs(urls, crawl, maxDepth, maxPages)` |
| [`app_search.go`](app_search.go) | `Search(query, topK, threshold, searchMode, folderID)` + ported snippet builder |
| [`app_tags.go`](app_tags.go) / [`app_folders.go`](app_folders.go) | tag + folder CRUD, document move |
| [`app_agent.go`](app_agent.go) | agent session/message CRUD; `SendAgentMessage` runs `agent.Run` in a goroutine (120s timeout) |
| [`internal/agent/agent.go`](internal/agent/agent.go) | Tool-calling LLM loop (max 5 iterations) + citation extraction |
| [`internal/agent/tools.go`](internal/agent/tools.go) | Tool dispatcher + 5 tool implementations |
| [`internal/store/sqlite.go`](internal/store/sqlite.go) | modernc SQLite + FTS5 + cosine + RRF |
| [`internal/events/broker.go`](internal/events/broker.go) | Non-blocking in-process event broker |
| [`frontend/src/lib/api.ts`](frontend/src/lib/api.ts) | Adapter over generated bindings (same names the old fetch store used) |
| [`frontend/src/lib/types.ts`](frontend/src/lib/types.ts) | Hand-written runtime types (ids/dates as `string`) |
| [`frontend/src/hooks/use-events.ts`](frontend/src/hooks/use-events.ts) | `EventsOn` subscription hook (replaces `useSSE`) |
| [`frontend/src/pages/AgentPage.tsx`](frontend/src/pages/AgentPage.tsx) | Agent chat UI (sessions sidebar + streaming thread) |

---

## The binding + event model (read this before touching the boundary)

**Calling Go from the UI.** Every exported method on `*App` in an `app_*.go` file becomes a JS function under `frontend/wailsjs/go/main/App`. Don't import those generated files directly in screens — go through `frontend/src/lib/api.ts`, which keeps the *same function names and return shapes* the old fetch-based Zustand store exposed (`fetchDocuments`, `searchDocuments`, `sendAgentMessage`, …). That adapter is why the ported page/component code didn't need rewriting.

**Conventions in the binding methods** (held across all `app_*.go`):
- **IDs are `string` params**, parsed with `uuid.Parse`; an empty string means "none" (→ `nil` UUID), e.g. an empty `folderID` lists across all folders.
- **Tenant scoping:** pass `a.userID` to every store call that accepts a `userID`. (Tags are global and intentionally take no `userID` — mirrors the original handler.)
- **Not-found vs wrong-user collapse to a generic "not found"** so existence isn't leaked across users.
- **No HTTP envelopes** — methods return typed values (or domain-prefixed in-file structs like `DocumentsPage`, `DocumentDetail`, `SearchResponse`). Errors are wrapped with `fmt.Errorf` (`%w` where there's a cause).
- **Type mismatch:** Wails generates `number[]` for `uuid.UUID` and `any` for `time.Time`. The frontend ignores the generated models and casts to `frontend/src/lib/types.ts`, which types ids/dates as `string` to match the real JSON.

**Receiving push updates.** `app.go`'s `forwardEvents` subscribes to the internal `events.Broker` and re-emits each event via `runtime.EventsEmit(ctx, evt.Type, evt.Data)`. The frontend subscribes with `useEvents([...types], handler)` (wrapping Wails `EventsOn`). This is the replacement for the old Server-Sent Events stream.

### Runtime event types
| Event | Payload | When |
|---|---|---|
| `document.completed` | document info | A document finishes processing (→ toast) |
| `document.failed` | document info + error | Processing failed (→ toast) |
| `agent.delta` | `{text, session_id}` | Streamed text chunk from the LLM |
| `agent.tool_call` | `{name, args, display_label, session_id}` | Agent invokes a tool |
| `agent.tool_result` | `{name, citations, display_label, session_id}` | Tool returns a value |
| `agent.complete` | `{session_id, message_id}` | Final assistant message persisted |
| `agent.error` | `{error, session_id}` | LLM/tool error surfaced to the UI |
| `sidecar.error` | `{error}` | Emitted at startup if the embedding sidecar didn't come up |

The agent UI consumes the `agent.*` events optimistically, then **refetches** the persisted message on `agent.complete` — the broker drops events under backpressure by design, so the refetch is the source of truth.

### Sidecar lifecycle (`app_sidecar.go`)
`locateSidecarDir` checks `EMBEDDING_SIDECAR_DIR`, then dirs next to the exe, then the dev tree (`backend/embedding-sidecar`). It runs the venv's interpreter (`.venv/Scripts/python.exe` on Windows) as `python -m uvicorn main:app --host 127.0.0.1 --port <free>`, polls `/health` for up to 60s (first start loads the model), and `stop()` kills the process on shutdown. Failure is **non-fatal**: the window still opens, `sidecar.error` is emitted, and ingest/search stay disabled until it's up.

---

## Conventions worth knowing

1. **Read [`AGENTS.md`](AGENTS.md) and [`MIGRATION_LOG.md`](MIGRATION_LOG.md) first** — the codebase is a desktop app; the migration log records *why* each web→desktop decision was made (e.g. the dropped Go-native ONNX path).
2. **Don't reach into generated bindings from UI code** — extend `frontend/src/lib/api.ts` instead, preserving the existing function names/shapes.
3. **New runtime event?** Publish it through the `events.Broker` in the Go core; it's auto-forwarded by `forwardEvents`. Add the type string to the relevant `useEvents([...])` call in the frontend.
4. **New bound method?** Add it to the right `app_*.go`, follow the ID/tenant/not-found conventions above, and re-run `wails dev`/`wails build` so the TS bindings regenerate; then expose it through `api.ts`.
5. **`LESSONS_LEARNED.md` is a running log** — append a section per feature/change (decisions, gotchas, file references). It survives conversation compaction.
6. **No commits without explicit user request** when working as an AI agent.
7. **Tool-call events carry `display_label`** — the frontend renders friendly status rows from it; set a sensible label in the dispatcher for any new tool.

---

## Related repo: docinsight-cli

A **separate** repo at `C:\Users\jibkh\Personal_Projects\docinsight-cli\` indexes git repos for semantic **code** search and exposes an MCP server for AI CLI agents. Patterns reused from DocInsight (FTS5 + cosine + RRF, BYO-key) — code is not. The two repos are independent. See the "Spinoff" section in [`LESSONS_LEARNED.md`](LESSONS_LEARNED.md).

---

## How to continue from a new session

1. **Read this file fully**, then [`MIGRATION_LOG.md`](MIGRATION_LOG.md) and [`LESSONS_LEARNED.md`](LESSONS_LEARNED.md).
2. **Verify the baseline:**
   ```powershell
   go test ./internal/...
   go test .
   cd frontend; npm run build; npx tsc --noEmit
   ```
3. **Smoke-test the app:** `wails dev`, add a PDF / ingest a URL, run a search, open the agent.
4. **Pick an item from "What's left"** (re-port Vitest, or distribution packaging) or wait for the user to point at one. Implement per-phase, test as you go, update `LESSONS_LEARNED.md`, one commit per phase.

---

## Open questions to confirm with the user

- **Distribution target:** which packaging route for the bundled sidecar (ship a venv vs a frozen sidecar binary vs a Wails NSIS installer step)?
- **Remove `cmd/server` + `internal/server`** now, or keep them around as reference for a bit?
- **TTS / Whisper transcription** — previously deferred; still wanted?
- **Cross-platform** — anything beyond Windows/WebView2 in scope (macOS WKWebView, Linux WebKitGTK)?
