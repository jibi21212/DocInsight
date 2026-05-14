# DocInsight E2E Test Playbook

Executable via Claude Preview MCP tools. No npm test dependencies required.

## Prerequisites

- `.claude/launch.json` defines `frontend` (port 3000) and `backend` (port 8080)
- Backend logs are captured to `test-logs/backend.log` via `tee`
- Run `bash e2e/reset-test-db.sh` before each test session

## Execution Order

```
1. Reset environment
2. Start backend  -> gate on "server starting" in logs
3. Start frontend -> gate on "Ready" in logs
4. Suite 1: Health Check
5. Suite 2: Dashboard
6. Suite 6: Navigation & Header
7. Suite 3: Upload Page (creates a document)
8. Suite 5: Document Detail (uses doc from Suite 3)
9. Suite 4: Search Page
10. Suite 7: Login Page
11. Suite 8: SSE Events
12. Suite 9: Responsive Layout
13. Post-test log analysis
14. Stop servers
15. Report results
```

---

## Suite 1: Server Startup & Health Check

**Goal:** Both servers boot and respond.

| Step | Tool | Action | Assert |
|------|------|--------|--------|
| 1.1 | `preview_logs(backend)` | Check startup | Contains `server starting` and `addr=:8080` |
| 1.2 | `preview_logs(backend, search:"SQLite")` | Check DB | Contains `using SQLite` |
| 1.3 | `preview_eval(frontend)` | `fetch('http://localhost:8080/health').then(r=>r.json())` | Returns `{"status":"ok"}` |
| 1.4 | `preview_logs(frontend)` | Check Next.js ready | Contains `Ready` or `Local:` |

---

## Suite 2: Dashboard Page (`/`)

**Goal:** Dashboard loads with correct empty state.

| Step | Tool | Action | Assert |
|------|------|--------|--------|
| 2.1 | `preview_screenshot(frontend)` | Visual baseline | Page renders without errors |
| 2.2 | `preview_snapshot(frontend)` | Accessibility tree | Contains "DocInsight" branding |
| 2.3 | `preview_snapshot(frontend)` | Check stat cards | Contains "Total Documents" with "0" |
| 2.4 | `preview_snapshot(frontend)` | Check empty state | Contains "No documents yet" |
| 2.5 | `preview_snapshot(frontend)` | Check search bar | Contains placeholder "Ask anything about your documents..." |
| 2.6 | `preview_logs(backend, search:"GET")` | Backend log | Contains `path=/api/documents` with `status=200` |

---

## Suite 3: Upload Page (`/upload`)

**Goal:** Both PDF and URL tabs work, upload reaches backend.

| Step | Tool | Action | Assert |
|------|------|--------|--------|
| 3.1 | `preview_click(frontend, "a[href='/upload']")` | Navigate | URL changes to /upload |
| 3.2 | `preview_snapshot(frontend)` | Check page | Contains "Upload PDF" tab (active) |
| 3.3 | `preview_eval(frontend)` | `document.querySelector('input[type=file]') !== null` | Returns `true` |
| 3.4 | `preview_eval(frontend)` | Synthetic upload via fetch + FormData (see below) | Returns 200 status |
| 3.5 | `preview_logs(backend, search:"upload")` | Backend log | Contains `path=/api/documents/upload` |
| 3.6 | `preview_snapshot(frontend)` | Switch to URL tab | Click "Add URLs" button, verify textarea appears |
| 3.7 | `preview_fill(frontend, "textarea", "https://example.com")` | Fill URL | Textarea populated |
| 3.8 | `preview_snapshot(frontend)` | Check crawl option | Contains "Crawl linked pages" |

**Synthetic upload script (Step 3.4):**
```javascript
(async () => {
  const fd = new FormData();
  const blob = new Blob(['%PDF-1.4 test content for DocInsight E2E'], {type: 'application/pdf'});
  fd.append('file', blob, 'e2e-test-document.pdf');
  const r = await fetch('http://localhost:8080/api/documents/upload', {method:'POST', body:fd});
  window.__e2eDocUpload = await r.json();
  return r.status;
})()
```

---

## Suite 4: Search Page (`/search`)

**Goal:** Search UI renders, mode switching works, search executes.

| Step | Tool | Action | Assert |
|------|------|--------|--------|
| 4.1 | `preview_click(frontend, "a[href='/search']")` | Navigate | URL is /search |
| 4.2 | `preview_snapshot(frontend)` | Check heading | Contains "Semantic Search" |
| 4.3 | `preview_snapshot(frontend)` | Check search bar | Contains placeholder text |
| 4.4 | `preview_fill(frontend, "input[placeholder*='Ask']", "test query")` | Fill search | Input populated |
| 4.5 | `preview_click(frontend, "button[type='submit']")` | Execute search | Search fires |
| 4.6 | `preview_snapshot(frontend)` | Check results | Contains "0 results" or empty state |
| 4.7 | `preview_logs(backend, search:"search")` | Backend log | Contains `path=/api/search` |
| 4.8 | `preview_console_logs(frontend, level:"error")` | JS errors | No unexpected errors |

---

## Suite 5: Document Detail (`/documents/[id]`)

**Goal:** Detail page renders for the uploaded document.

| Step | Tool | Action | Assert |
|------|------|--------|--------|
| 5.1 | `preview_eval(frontend)` | `fetch('http://localhost:8080/api/documents?page=1&pageSize=1').then(r=>r.json()).then(d=>d.data[0]?.id ?? 'NO_DOCS')` | Returns a UUID |
| 5.2 | `preview_eval(frontend)` | `window.location.href = 'http://localhost:3000/documents/{ID}'` | Navigates |
| 5.3 | `preview_snapshot(frontend)` | Check structure | Contains "Back to Dashboard" link |
| 5.4 | `preview_snapshot(frontend)` | Check doc name | Contains "e2e-test-document.pdf" |
| 5.5 | `preview_snapshot(frontend)` | Check status | Contains a status badge (pending/processing/completed/failed) |
| 5.6 | `preview_logs(backend, search:"{ID}")` | Backend log | Contains `path=/api/documents/{ID} status=200` |

---

## Suite 6: Navigation & Header

**Goal:** All nav links route correctly, dark mode toggles.

| Step | Tool | Action | Assert |
|------|------|--------|--------|
| 6.1 | `preview_eval(frontend, "window.location.href='http://localhost:3000/'")` | Go home | At / |
| 6.2 | `preview_snapshot(frontend)` | Check header | Contains "DocInsight", nav links, dark mode button |
| 6.3 | `preview_click(frontend, "a[href='/upload']")` | Nav to upload | `window.location.pathname` === "/upload" |
| 6.4 | `preview_click(frontend, "a[href='/search']")` | Nav to search | `window.location.pathname` === "/search" |
| 6.5 | `preview_click(frontend, "a[href='/']")` | Nav to home | `window.location.pathname` === "/" |
| 6.6 | `preview_screenshot(frontend)` | Light mode baseline | Visual check |
| 6.7 | `preview_eval(frontend)` | Click dark mode toggle button | `document.documentElement.classList.contains('dark')` === true |
| 6.8 | `preview_screenshot(frontend)` | Dark mode visual | Background is dark |
| 6.9 | `preview_eval(frontend)` | Toggle back | `document.documentElement.classList.contains('dark')` === false |

---

## Suite 7: Login Page (`/login`)

**Goal:** Auth UI renders, registration hits backend.

| Step | Tool | Action | Assert |
|------|------|--------|--------|
| 7.1 | `preview_eval(frontend, "window.location.href='http://localhost:3000/login'")` | Navigate | At /login |
| 7.2 | `preview_snapshot(frontend)` | Sign In tab | Contains "API Key" label, input with placeholder "di_..." |
| 7.3 | `preview_snapshot(frontend)` | Find Register tab | Click Register button/tab |
| 7.4 | `preview_snapshot(frontend)` | Register form | Contains "Email" and "Name" labels |
| 7.5 | `preview_fill(frontend, "input[type='email']", "e2e@test.com")` | Fill email | Input populated |
| 7.6 | `preview_fill(frontend, "input[type='text']", "E2E Tester")` | Fill name | Input populated |
| 7.7 | `preview_click(frontend, "button")` | Submit register | Click the Register button |
| 7.8 | `preview_snapshot(frontend)` | Check response | Contains generated API key (di_...) or error message |
| 7.9 | `preview_logs(backend, search:"auth")` | Backend log | Contains `path=/api/auth/register` |

---

## Suite 8: SSE Events

**Goal:** SSE endpoint responds with correct content type.

| Step | Tool | Action | Assert |
|------|------|--------|--------|
| 8.1 | `preview_eval(frontend)` | `(async()=>{const r=await fetch('http://localhost:8080/api/events');return r.status+' '+r.headers.get('content-type')})()` | Contains `200` and `text/event-stream` |
| 8.2 | `preview_logs(backend, search:"events")` | Backend log | Contains `path=/api/events` |

---

## Suite 9: Responsive Layout

**Goal:** No layout breakage across viewports.

| Step | Tool | Action | Assert |
|------|------|--------|--------|
| 9.1 | `preview_eval(frontend, "window.location.href='http://localhost:3000/'")` | Go home | At dashboard |
| 9.2 | `preview_resize(frontend, preset:"desktop")` | Desktop | 1280x800 |
| 9.3 | `preview_screenshot(frontend)` | Desktop shot | Full nav, multi-column grid |
| 9.4 | `preview_resize(frontend, preset:"tablet")` | Tablet | 768x1024 |
| 9.5 | `preview_screenshot(frontend)` | Tablet shot | Layout adjusts, no overflow |
| 9.6 | `preview_resize(frontend, preset:"mobile")` | Mobile | 375x812 |
| 9.7 | `preview_screenshot(frontend)` | Mobile shot | Single column, nav collapses |
| 9.8 | `preview_resize(frontend, preset:"desktop")` | Reset | Back to desktop |

---

## Post-Test Log Analysis

| Check | Tool | What to look for |
|-------|------|-----------------|
| Backend errors | `preview_logs(backend, level:"error")` | Any level=ERROR lines |
| 5xx responses | `preview_logs(backend, search:"status=5")` | Unexpected server errors |
| Frontend JS errors | `preview_console_logs(frontend, level:"error")` | Unexpected exceptions |
| Log volume | `bash: wc -l < test-logs/backend.log` | Total request count |
| Failed network | `preview_network(frontend, filter:"failed")` | Failed API calls |

**Expected errors (not failures):**
- Embedding sidecar connection refused (if sidecar not running)
- Document processing failures at embedding step
- Search errors in semantic mode without sidecar

---

## Results Report Format

```
=== DocInsight E2E Test Results ===

Suite 1: Server Startup & Health     [PASS/FAIL]
Suite 2: Dashboard Page              [PASS/FAIL]
Suite 3: Upload Page                 [PASS/FAIL]
Suite 4: Search Page                 [PASS/FAIL]
Suite 5: Document Detail             [PASS/FAIL]
Suite 6: Navigation & Header         [PASS/FAIL]
Suite 7: Login Page                  [PASS/FAIL]
Suite 8: SSE Events                  [PASS/FAIL]
Suite 9: Responsive Layout           [PASS/FAIL]

=== Backend Log Summary ===
Total log lines:     <N>
Error entries:       <N>
5xx responses:       <N>
Expected errors:     <list>
Unexpected errors:   <list or "none">

=== Frontend Console Summary ===
JS errors:           <N>
Unexpected errors:   <list or "none">
```
