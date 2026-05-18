# DocInsight — Agent / Developer Handoff

**Purpose:** if you're starting a fresh session (Claude Code, Codex, Copilot CLI, or a new human contributor) and want to pick up DocInsight work without losing context, read this file first.

> **Tooling note** — the project's [`AGENTS.md`](AGENTS.md) calls out that Next.js 16.2.3 has breaking changes from training-data Next.js. Before writing frontend code, consult `node_modules/next/dist/docs/` for the current API.

---

## What DocInsight is

DocInsight is a **semantic document search + AI agent** web app.

- **Frontend:** Next.js 16.2.3 (App Router), React 19, TypeScript, Tailwind 4, zustand. Routes: `/`, `/upload`, `/search`, `/agent`, `/documents/[id]`, `/login`.
- **Backend:** Go (chi router, `modernc.org/sqlite`), SSE event broker, brute-force cosine + FTS5 hybrid search via RRF (k=60).
- **Embedding sidecar:** small Python FastAPI service (`backend/embedding-sidecar/`) producing embeddings. **Required** for ingest/search to function.
- **BYO-LLM agent:** the user supplies their own Anthropic or OpenAI API key (stored in `localStorage`, forwarded per request as `X-LLM-API-Key`). Never persisted server-side.

---

## Current state (as of this commit)

| Metric | Value |
|---|---|
| Backend tests | **231 passing / 0 skipped / 0 failed** |
| Frontend tests (Vitest + happy-dom) | **14 passing / 0 skipped / 0 failed** |
| `go vet ./...` | clean |
| `npx next build` | clean, 8 routes |
| Branch | `master` |

### Implemented features

**Core (commit `837a6f3`):** bulk PDF upload, export, doc refresh, tags, hybrid search (FTS5 + cosine + RRF), OCR via Tesseract, web-page crawling, SSE for processing/search events, API-key auth.

**E2E + chi fix (`28c582a`):** `.claude/launch.json`, E2E playbook in `e2e/E2E_TEST_PLAN.md`, fix for chi middleware ordering panic.

**Search snippets, folders, agent (`a91b39b`):**
- Snippet extraction with stopword-aware tokenization, earliest-match windowing, `…` ellipsis, frontend `<mark>` highlighting.
- Hierarchical folders with recursive CTE descendants; sidebar tree + move-to-folder picker.
- BYO-LLM agent (`/agent` route) with Anthropic + OpenAI streaming, tool-calling loop (max 5 iterations), `<cite chunk="UUID"/>` citation extraction, expandable source list per message.

**CORS for agent (`d3d08ca`):** allowed `X-LLM-API-Key` header through CORS preflight.

**Agent tools expansion (`7204281`):** 4 new tools added to the agent — `get_document`, `summarize_document`, `list_documents`, `get_chunk_context`. Dispatcher extracted into [`backend/internal/agent/tools.go`](backend/internal/agent/tools.go). Added store method `GetChunkByID(chunkID, userID)`.

**Vitest setup (`8f093de`):** first frontend test framework. **happy-dom** environment (jsdom 27.x has an unfixable ESM/CJS deadlock with Vitest 4's CJS worker pool). Config is `vitest.config.mts` (the `.mts` extension is required — see [LESSONS_LEARNED.md](LESSONS_LEARNED.md#vitest-setup-jsdom--happy-dom-swap)).

**Voice input (`295d093`):** Web Speech API (browser-native `SpeechRecognition`), zero backend services. Hook at [`src/hooks/use-speech-recognition.ts`](src/hooks/use-speech-recognition.ts), `MicButton` at [`src/components/mic-button.tsx`](src/components/mic-button.tsx). Slotted into the search bar and agent composer.

**Inline tool status (`38aea0f`):** [`ToolStatus`](src/components/tool-status.tsx) renders friendly "Searching for…", "Reading…", "Summarizing…" rows inline as the agent invokes tools. Flips to a check mark on tool_result.

---

## What's NOT implemented yet

Categorized by likely effort.

### Low effort, high value
- **TTS for agent replies** (`SpeechSynthesis` Web API) — symmetric to the input voice feature, ~half a day.
- **`compare_documents` agent tool** — prompt-engineering work; pattern already exists in `summarize_document`.
- **Faceted search sidebar** — tag/folder/date/source filters; data is already present in the model.
- **Search analytics dashboard** — top queries, zero-result queries; needs a small `search_log` table + dashboard route.

### Medium effort, high value
- **Audio/video transcription** — would expand DocInsight from "PDF search" to "everything search". Requires a Whisper sidecar (the user explicitly said they don't want to handle audio yet — re-confirm before starting).
- **Cross-encoder re-ranking** — retrieve top-50 hybrid, re-rank with a small cross-encoder; meaningful quality jump.
- **Scheduled re-crawl** — re-fetch indexed URLs on a cron; diff content; surface changes.
- **Saved searches + alerts** — store a query; fire SSE/email when new docs match.
- **Comments + highlights** on documents — most-requested feature for any doc tool.

### Larger, structural
- **Shared workspaces / multi-user orgs** — schema-wise it's an `org_id` column on users + role table, but it touches every query. Big lift, unlocks team usage.
- **Public share links** — share a doc or saved search read-only.
- **MCP server mode for the backend** — expose `search_documents`, `get_document` as MCP tools so external agents can search a user's DocInsight library. **Note:** this is distinct from the docinsight-cli MCP server (different repo, different data — code instead of PDFs).
- **Browser extension** — one-click page clip into DocInsight.
- **Layout-aware PDF extraction** — current extractor loses 30%+ of structure on academic/financial PDFs. Could integrate Marker or a vision-LLM pass.

### Out-of-scope / explicitly deferred
- Backend-driven STT (Whisper sidecar) — user explicitly declined this path for voice input
- Continuous dictation mode for very long speech — hold-to-talk is enough
- `sqlite-vec` for vector search — brute force handles current scale
- Tree-sitter chunking — too heavy a cgo dep for what we get

---

## Related repo: docinsight-cli

A **separate** repo at `C:\Users\jibkh\Personal_Projects\docinsight-cli\` indexes git repos for semantic **code** search and exposes an MCP server for AI CLI agents (Claude Code, Codex, Copilot CLI). Patterns reused from DocInsight — code is not. See the "Spinoff" section in [LESSONS_LEARNED.md](LESSONS_LEARNED.md#spinoff-docinsight-cli-2026-05-14).

The two repos are independent. Changes to one don't require changes to the other.

---

## How to run DocInsight locally

### Prereqs
- Go 1.23+ in PATH (on Windows, `"C:\Program Files\Git\bin\bash.exe" -c 'export PATH="/c/Program Files/Go/bin:$PATH" && ...'`)
- Node 20+, npm
- Python 3.11+ (for the embedding sidecar)
- Tesseract (optional, for OCR fallback)

### First-run setup
```bash
# Frontend deps
npm install

# Backend builds in-place
cd backend && go mod download && cd ..

# Embedding sidecar (one-time)
cd backend/embedding-sidecar
python -m venv .venv
.venv/Scripts/activate    # or `source .venv/bin/activate`
pip install -r requirements.txt
cd ../..
```

### Dev loop (three terminals)
```bash
# Terminal A — embedding sidecar
cd backend/embedding-sidecar && .venv/Scripts/python -m uvicorn main:app --port 8000

# Terminal B — Go backend (auth enabled)
AUTH_ENABLED=true go run ./backend/cmd/server/main.go

# Terminal C — Next.js frontend
npm run dev
```

### Test
```bash
# Backend (Go)
cd backend && go test ./... -count=1 -timeout 180s
# Expected: 231 passing, 0 skipped, 0 failed

# Frontend (Vitest)
npm run test:run
# Expected: 14 passing, 0 skipped, 0 failed

# Frontend build
npx next build
# Expected: 8 routes, no errors
```

### Reset E2E state
```bash
bash e2e/reset-test-db.sh
```

### Claude Preview MCP smoke test
The launch configs in `.claude/launch.json` define `frontend` and `backend` for the [Claude Preview MCP](https://docs.anthropic.com/en/docs/claude-code/mcp). Use `preview_start` / `preview_snapshot` / `preview_eval` etc. for browser-side verification.

---

## Architecture cheat sheet

```
┌─────────────────┐    HTTP     ┌──────────────────┐
│  Next.js 16     │────────────▶│  Go backend      │
│  (port 3000)    │             │  chi router      │
│                 │◀────────────│  port 8080       │
│  - /agent       │   SSE       │                  │
│  - /search      │             │  ┌────────────┐  │
│  - /upload      │             │  │ embedder   │──┼──HTTP──▶ Python sidecar (port 8000)
│  - /          │               │  │ chunker    │  │
└─────────────────┘             │  │ store      │──┼─── SQLite (./docinsight.db)
                                │  │ agent loop │  │
                                │  │ llm client │──┼──HTTPS─▶ Anthropic / OpenAI
                                │  └────────────┘  │
                                └──────────────────┘
```

### Key files
| Path | What |
|---|---|
| [`backend/internal/agent/agent.go`](backend/internal/agent/agent.go) | Tool-calling LLM loop (max 5 iterations) |
| [`backend/internal/agent/tools.go`](backend/internal/agent/tools.go) | Tool dispatcher + 5 tool implementations |
| [`backend/internal/handler/agent.go`](backend/internal/handler/agent.go) | HTTP handlers for `/api/agent/sessions/*` |
| [`backend/internal/llm/{anthropic,openai}.go`](backend/internal/llm) | Streaming LLM clients (SSE parsers) |
| [`backend/internal/store/sqlite.go`](backend/internal/store/sqlite.go) | SQLite + FTS5 + cosine search + RRF |
| [`backend/internal/events/broker.go`](backend/internal/events/broker.go) | Non-blocking SSE event broker |
| [`src/app/agent/page.tsx`](src/app/agent/page.tsx) | Agent chat UI (sessions sidebar + composer) |
| [`src/components/mic-button.tsx`](src/components/mic-button.tsx) | Voice input button (Web Speech API) |
| [`src/hooks/use-speech-recognition.ts`](src/hooks/use-speech-recognition.ts) | SpeechRecognition state-machine hook |
| [`src/components/tool-status.tsx`](src/components/tool-status.tsx) | Inline tool-call status rows in chat |
| [`LESSONS_LEARNED.md`](LESSONS_LEARNED.md) | Running log of gotchas, decisions, file references |

### Auth model
- API-key auth, header `Authorization: Bearer di_<hex>`
- Set `AUTH_ENABLED=true` env var to enforce
- `/api/auth/register` accepts `{email, name}` and returns `{user, api_key}` — the key is shown once and only once
- All multi-tenant tables carry `user_id` (NOT NULL)

### Agent SSE event types
| Event | Payload | When |
|---|---|---|
| `agent.delta` | `{text, session_id}` | Streamed text chunk from LLM |
| `agent.tool_call` | `{name, args, display_label, session_id}` | LLM invokes a tool |
| `agent.tool_result` | `{name, citations, display_label, session_id}` | Tool returns a value |
| `agent.complete` | `{session_id, message_id}` | Final assistant message persisted |
| `agent.error` | `{error, session_id}` | LLM/tool error surfaced to client |

Frontend consumes via [`src/hooks/use-sse.ts`](src/hooks/use-sse.ts) and re-fetches messages from the DB on `agent.complete` to capture the authoritative persisted state (SSE deltas are best-effort and can be dropped under backpressure).

---

## Conventions worth knowing

1. **Test discipline:** every PR keeps `backend ≥ 231` passing, **0 skipped**, **0 failed**. Frontend ≥ 14. If you're tempted to skip a test, fix the underlying issue instead.
2. **`LESSONS_LEARNED.md` is a running log** — append a section per feature with: test-count delta, gotchas encountered, design decisions made, file-reference table for new code.
3. **No commits without explicit user request** when working as an AI agent.
4. **chi middleware ordering:** `r.Use(...)` MUST come before any route definitions on the same mux. chi panics at runtime if violated. See [LESSONS_LEARNED.md](LESSONS_LEARNED.md#bug-found-during-e2e-chi-middleware-ordering).
5. **Vitest config is `vitest.config.mts`, NOT `.ts`** — the `.mts` extension is required to avoid `ERR_REQUIRE_ESM` in Vitest 4.x.
6. **Tool-call SSE events carry `display_label`** — frontend uses this to render friendly status rows. New tools should set a sensible label in the dispatcher.

---

## How to continue from a new session

If you are an AI agent picking this up cold:

1. **Read this file fully.**
2. **Read [`LESSONS_LEARNED.md`](LESSONS_LEARNED.md)** end-to-end. It contains all the "why we chose X" context that prevents re-litigating settled decisions.
3. **Skim [`AGENTS.md`](AGENTS.md)** — single-line warning about Next.js 16.
4. **Verify baseline tests pass:**
   ```bash
   cd backend && go test ./... -count=1
   cd .. && npm run test:run
   npx next build
   ```
5. **Pick a feature from the "What's NOT implemented yet" list above** (or wait for the user to point at one) and use the same pattern that's worked across all prior features:
   - EnterPlanMode → write plan → ExitPlanMode for approval
   - Implement per-phase with `TodoWrite` tracking
   - Tests as you go (unit + integration; smoke via Claude Preview MCP)
   - Update `LESSONS_LEARNED.md`
   - One commit per phase, message style matches existing log

If you are a human contributor: the same flow works without the planning ceremony. Tests + LESSONS_LEARNED updates still apply.

---

## Open questions to confirm with the user

If anything below changes the design, ask before starting:
- TTS for agent replies — confirm whether to ship now (user explicitly deferred earlier)
- Whisper transcription — explicitly declined for voice input; may still be wanted for ingest
- Multi-tenant orgs — large schema migration; needs explicit greenlight
- Public REST API — would require versioning, rate-limit, and docs work
