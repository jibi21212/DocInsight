# DocInsight

**A private, offline-first desktop app for semantic document search with a
bring-your-own-key AI agent.**

DocInsight ingests your PDFs and web pages, indexes them for meaning-based search
(not just keywords), and lets an AI agent answer questions grounded in your own
library — with inline citations back to the source chunk. It runs as a **single
native desktop application** (Wails v2 + WebView2): one window, one binary, your
data on your machine. You supply your own Anthropic or OpenAI API key; it is
forwarded per request and **never stored**.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
![Wails](https://img.shields.io/badge/Wails-v2.12-DF0000?logo=wails&logoColor=white)
![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-19-149ECA?logo=react&logoColor=white)
![Platform](https://img.shields.io/badge/platform-Windows-0078D6?logo=windows&logoColor=white)

---

## Table of contents

- [Features](#features)
- [Screenshots](#screenshots)
- [Architecture](#architecture)
- [Tech stack](#tech-stack)
- [Getting started](#getting-started)
- [Testing](#testing)
- [How the agent works](#how-the-agent-works)
- [Project structure](#project-structure)
- [Privacy](#privacy)
- [License](#license)

---

## Features

### Ingestion & search
- **Bulk PDF import** via a native file picker, with a six-stage processing
  pipeline (extract → chunk → store → embed → index → complete) and an **OCR
  fallback** (Tesseract) that kicks in automatically when a PDF's extracted text
  is too sparse.
- **Web-page ingestion** — paste URLs to fetch and index article content, with
  optional **recursive same-domain crawling**.
- **Hybrid search** — full-text (SQLite FTS5/BM25) and semantic (cosine
  similarity) results fused with **Reciprocal Rank Fusion** (RRF, _k_ = 60).
  Search by keyword, by meaning, or both.
- **Highlighted snippets** with stopword-aware tokenization and earliest-match
  windowing.
- **Hierarchical folders** (recursive-CTE subtree scoping) and **tags** for
  organizing a library.

### AI agent
- **Bring-your-own-LLM** chat over your documents — streaming responses from
  **Anthropic** or **OpenAI**.
- A tool-calling loop with **five grounded tools**: `search_documents`,
  `get_document`, `summarize_document`, `list_documents`, and
  `get_chunk_context`.
- **Inline citations** — answers cite the exact source chunk
  (`<cite chunk="…"/>`), rendered as numbered, expandable sources.
- **Live tool status** — the UI shows "Searching for…", "Reading…",
  "Summarizing…" as the agent works, streamed over Wails runtime events.
- **Voice input** — dictate queries and prompts via the Web Speech API
  (handled by the WebView2 layer; no audio reaches DocInsight).

### Desktop platform
- **Single native window** — a Go core and a React UI rendered in WebView2. No
  browser, no localhost server to start, no ports to manage.
- **Self-contained startup** — on launch the app spawns its embedding sidecar,
  health-checks it, provisions a local user, and recovers any in-flight jobs;
  on close it tears everything down.
- **Local-first storage** — everything lives under `%APPDATA%\DocInsight`
  (a `modernc.org/sqlite` database — pure Go, no CGo — plus an `uploads`
  folder). Nothing is uploaded anywhere.
- **Real-time updates** via Wails runtime events for processing and agent
  streaming.
- **Export** search results to JSON or CSV.

---

## Screenshots

| | |
|:--:|:--:|
| ![Document library](docs/screenshots/dashboard.jpg) | ![Semantic search](docs/screenshots/search.jpg) |
| **Document library** — mixed PDF & web sources, processed and ready to search | **Hybrid search** — ranked results with highlighted matches and citations |
| ![Grounded AI agent](docs/screenshots/agent.jpg) | ![Add content](docs/screenshots/upload.jpg) |
| **AI agent** — grounded in your library, bring-your-own key | **Add content** — import PDFs or ingest &amp; crawl web pages |

---

## Architecture

DocInsight is a **Wails v2 desktop app**: a Go core and a React/Tailwind UI
rendered in a native **WebView2** window. The frontend never makes HTTP calls —
it invokes Go directly through **generated Wails bindings** and receives push
updates through **Wails runtime events**. The only network process is a small,
locally-spawned Python sidecar that produces embeddings.

```
        ┌────────────────────────────────────────────────────────────┐
        │                    docinsight.exe  (one process)             │
        │                                                              │
        │   ┌──────────────────────────┐    bindings   ┌───────────┐  │
        │   │  WebView2 window          │  ◀─────────▶  │  Go core  │  │
        │   │  React 19 · Tailwind 4    │   (App.*())   │           │  │
        │   │  Zustand · react-router   │               │ ingest    │  │
        │   │                           │  ◀──────────  │ chunk·OCR │  │
        │   │  Dashboard · Search       │   runtime     │ hybrid    │  │
        │   │  Agent · Add · Detail     │   events      │ search    │  │
        │   └──────────────────────────┘               │ agent loop│  │
        │                                               └─────┬─────┘  │
        └─────────────────────────────────────────────────── │ ───────┘
                              embeddings  │            data │  │  chat completions
                            (spawned + supervised)          │  │  (your API key)
                          ┌───────────────▼──┐  ┌───────────▼┐ │ ┌────────────────────┐
                          │ Python sidecar   │  │  SQLite    │ └▶│ Anthropic / OpenAI │
                          │ FastAPI·uvicorn  │  │ %APPDATA%\ │   │  (forwarded only,  │
                          │ MiniLM-L6-v2 384d│  │ DocInsight │   │   never persisted) │
                          └──────────────────┘  └────────────┘   └────────────────────┘
```

- **Go core** (`internal/`, module `github.com/docinsight/backend` at the repo
  root) — the chunker, worker pool, channel job queue (with crash recovery),
  scraper/crawler, OCR, the `modernc.org/sqlite` store (FTS5 + brute-force
  cosine + RRF), the LLM clients, and the agent loop. **Reused unchanged** from
  the previous web version.
- **Bindings layer** (`app_*.go` at the repo root) — exported methods on the
  `App` struct that Wails exposes to JavaScript, split by domain
  (`app_documents.go`, `app_ingest.go`, `app_search.go`, `app_tags.go`,
  `app_folders.go`, `app_agent.go`). `app.go` does startup/shutdown, dependency
  wiring, local-user provisioning, and bridges the internal event broker to
  `runtime.EventsEmit`. `app_sidecar.go` supervises the Python process.
- **Frontend** (`frontend/`) — Vite + React 19 + Tailwind 4 + Zustand +
  react-router-dom (HashRouter). It calls Go through the generated bindings
  (`frontend/wailsjs/go/main/App`) wrapped by `frontend/src/lib/api.ts`, and
  consumes streaming/processing events via `frontend/src/hooks/use-events.ts`.
- **Embedding sidecar** (`backend/embedding-sidecar/`) — a FastAPI/uvicorn
  service serving `all-MiniLM-L6-v2` (384-dim) embeddings. The app picks a free
  localhost port, spawns the venv's `python -m uvicorn main:app`, waits for
  `/health`, and kills it on shutdown. **Required** for ingestion and search.

> **Why a Python sidecar and not pure Go?** A CGo-free, Go-native ONNX
> embedding path was evaluated and dropped during the desktop migration — the
> sidecar keeps the build C-compiler-free and reuses the exact model the
> original app shipped. See [`MIGRATION_LOG.md`](MIGRATION_LOG.md).

---

## Tech stack

| Layer | Technologies |
| --- | --- |
| Shell | Wails v2.12.0 · WebView2 (native window) |
| Frontend | React 19, TypeScript, Tailwind CSS 4, Zustand, react-router-dom, lucide-react, Vite 6 |
| Core | Go 1.23, `modernc.org/sqlite` (pure Go, no CGo), channel-based worker pool |
| Search | SQLite FTS5 (BM25) + brute-force cosine, fused with RRF |
| Embeddings | Python FastAPI/uvicorn sidecar · `all-MiniLM-L6-v2` (384-dim) |
| LLM | Anthropic Messages API · OpenAI Chat Completions (streaming, BYO key) |
| Storage | SQLite under `%APPDATA%\DocInsight` |
| OCR | Tesseract (optional) |
| Tests | Go `testing` (core + binding layer) |

---

## Getting started

### Prerequisites
- **Go** 1.23+
- **Node.js** 20+ and npm
- **Python** 3.11+ (for the embedding sidecar)
- **WebView2** runtime — preinstalled on Windows 11
- **Tesseract** (optional, for OCR of scanned PDFs)

No C compiler is required.

### One-time setup

From the repo root, in PowerShell:

```powershell
./setup.ps1
```

This is idempotent. It provisions the Python sidecar's virtualenv + requirements
(the first run downloads the embedding model's dependencies and can take a few
minutes), downloads Go module dependencies, installs the **Wails v2 CLI**
(`v2.12.0`), and installs the frontend's npm dependencies.

> If `wails` isn't on your `PATH` afterwards, add your `GOPATH\bin` to it (the
> setup script prints the exact path), or invoke it by its full path.

### Run it

```powershell
# Hot-reload development (recompiles Go + serves the Vite dev frontend in the window)
wails dev

# Production build → a single binary
wails build      # produces build\bin\docinsight.exe
```

Launch the binary (or `wails dev`) and the app opens its own window. There is no
URL to visit and no login: a local user is provisioned automatically on first
run. To use the agent, open its settings and paste your Anthropic or OpenAI API
key — it is kept only in this app's local storage and forwarded per request.

---

## Testing

```powershell
# Go core
go test ./internal/...

# Wails binding layer (the app_*.go methods)
go test .

# Static analysis
go vet ./internal/... .

# Frontend build + type-check
cd frontend
npm run build
npx tsc --noEmit
```

> Frontend unit tests (Vitest) are not yet re-set-up for the new `frontend/`
> tree — see [`HANDOFF.md`](HANDOFF.md) for status.

---

## How the agent works

When you send a message, the Go core runs a bounded tool-calling loop:

1. The user message and tool schemas are streamed to your chosen LLM.
2. If the model calls a tool, the core's **dispatcher** executes it (all
   tenant-scoped to your local user), emits a friendly status to the UI, and
   feeds the result back to the model.
3. Tools that return chunks attach **citations**; `summarize_document` issues a
   bounded nested LLM call using your key.
4. The loop repeats (up to 5 iterations) until the model produces a final
   answer, whose `<cite chunk="…"/>` markers are resolved into an expandable
   source list.

Streaming `agent.delta` events drive the live UI optimistically; on
`agent.complete` the client refetches the authoritative persisted message, so
the final state is always correct even if streamed events are dropped under
backpressure.

---

## Project structure

```
.
├── main.go                       # Wails entry point (embeds frontend/dist, binds App)
├── app.go                        # startup/shutdown, DI wiring, event bridge, local user
├── app_sidecar.go                # spawn + health-check + stop the Python sidecar
├── app_documents.go              # bound methods: list/get/delete/move/process/refresh/add
├── app_ingest.go                 # bound methods: URL ingestion (+ optional crawl)
├── app_search.go                 # bound methods: hybrid/semantic/keyword search
├── app_tags.go  app_folders.go   # bound methods: tags + folder CRUD
├── app_agent.go                  # bound methods: agent sessions/messages
├── wails.json                    # Wails project config
├── setup.ps1                     # one-time idempotent setup
├── internal/                     # Go core (reused unchanged)
│   ├── agent/                    # LLM tool-calling loop + tool dispatcher
│   ├── store/                    # modernc SQLite store, FTS5 + cosine + RRF
│   ├── llm/                      # Anthropic + OpenAI streaming clients
│   ├── chunker/ embedder/        # text chunking + sidecar client
│   ├── worker/ queue/            # job pool + channel queue
│   ├── scraper/ crawler/ ocr/    # web ingestion + OCR
│   ├── events/                   # in-process event broker
│   ├── pdf/ model/ config/       # extraction · domain types · config
├── frontend/
│   ├── src/pages/                # Dashboard, AddContent, Search, Agent, DocumentDetail
│   ├── src/components/           # UI: search bar, agent message, mic button, ...
│   ├── src/hooks/                # use-events (Wails events), use-speech-recognition
│   ├── src/lib/                  # api.ts (binding adapter), types.ts
│   └── wailsjs/                  # generated Go bindings + runtime
├── backend/embedding-sidecar/    # Python FastAPI embedding service
├── HANDOFF.md                    # full developer handoff
├── LESSONS_LEARNED.md            # running log of decisions & gotchas
└── MIGRATION_LOG.md              # web → desktop conversion log
```

For deeper context — what's implemented, conventions, and design rationale — see
[`HANDOFF.md`](HANDOFF.md), [`LESSONS_LEARNED.md`](LESSONS_LEARNED.md), and
[`MIGRATION_LOG.md`](MIGRATION_LOG.md).

---

## Privacy

- **Your documents never leave your machine.** They are stored locally under
  `%APPDATA%\DocInsight` (SQLite + an uploads folder).
- **No server, no account, no telemetry.** DocInsight runs as a single local
  process. The embedding sidecar is spawned on `127.0.0.1` and is not reachable
  off-host.
- **Your LLM API key is never persisted by the core** — it lives in the app's
  local storage and is forwarded per request to your chosen provider, then
  dropped. Document text is sent to that provider only when *you* chat with the
  agent.
- **Voice input is handled by the WebView2 layer** via the Web Speech API.
  DocInsight never receives audio bytes.

---

## License

Released under the [MIT License](LICENSE). © 2026 jibkh.
