import { create } from "zustand";
import type {
  Document,
  Tag,
  Folder,
  AgentSession,
  AgentMessage,
  LLMProvider,
  SearchMode,
  SearchResult,
  SearchResponse,
  PaginatedResponse,
} from "@/lib/types";

interface AppState {
  // Documents
  documents: Document[];
  documentsLoading: boolean;
  totalDocuments: number;
  currentPage: number;

  // Search
  searchQuery: string;
  searchResults: SearchResult[];
  searchLoading: boolean;
  searchTookMs: number;
  selectedDocumentIds: string[];

  // Folders
  selectedFolderId: string | null;

  // UI
  darkMode: boolean;

  // Actions
  setDocuments: (docs: Document[], total: number) => void;
  setDocumentsLoading: (loading: boolean) => void;
  setCurrentPage: (page: number) => void;
  updateDocumentStatus: (
    id: string,
    status: Document["status"],
    errorMessage?: string
  ) => void;
  removeDocument: (id: string) => void;
  addDocument: (doc: Document) => void;

  setSearchQuery: (query: string) => void;
  setSearchResults: (results: SearchResult[], tookMs: number) => void;
  setSearchLoading: (loading: boolean) => void;
  setSelectedDocumentIds: (ids: string[]) => void;
  clearSearch: () => void;

  setSelectedFolderId: (id: string | null) => void;

  toggleDarkMode: () => void;
}

export const useAppStore = create<AppState>((set) => ({
  documents: [],
  documentsLoading: false,
  totalDocuments: 0,
  currentPage: 1,

  searchQuery: "",
  searchResults: [],
  searchLoading: false,
  searchTookMs: 0,
  selectedDocumentIds: [],

  selectedFolderId: null,

  darkMode: false,

  setDocuments: (docs, total) => set({ documents: docs, totalDocuments: total }),
  setDocumentsLoading: (loading) => set({ documentsLoading: loading }),
  setCurrentPage: (page) => set({ currentPage: page }),
  updateDocumentStatus: (id, status, errorMessage) =>
    set((state) => ({
      documents: state.documents.map((d) =>
        d.id === id
          ? { ...d, status, error_message: errorMessage ?? d.error_message }
          : d
      ),
    })),
  removeDocument: (id) =>
    set((state) => ({
      documents: state.documents.filter((d) => d.id !== id),
      totalDocuments: state.totalDocuments - 1,
    })),
  addDocument: (doc) =>
    set((state) => ({
      documents: [doc, ...state.documents],
      totalDocuments: state.totalDocuments + 1,
    })),

  setSearchQuery: (query) => set({ searchQuery: query }),
  setSearchResults: (results, tookMs) =>
    set({ searchResults: results, searchTookMs: tookMs }),
  setSearchLoading: (loading) => set({ searchLoading: loading }),
  setSelectedDocumentIds: (ids) => set({ selectedDocumentIds: ids }),
  clearSearch: () =>
    set({ searchQuery: "", searchResults: [], searchTookMs: 0 }),

  setSelectedFolderId: (id) => set({ selectedFolderId: id }),

  toggleDarkMode: () =>
    set((state) => {
      const newMode = !state.darkMode;
      if (typeof window !== "undefined") {
        document.documentElement.classList.toggle("dark", newMode);
        localStorage.setItem("darkMode", String(newMode));
      }
      return { darkMode: newMode };
    }),
}));

// API base URL - points to Go backend when set, otherwise same-origin
const API_BASE = process.env.NEXT_PUBLIC_API_URL || "";

function authHeaders(): Record<string, string> {
  if (typeof window === "undefined") return {};
  const key = localStorage.getItem("docinsight_api_key");
  return key ? { Authorization: `Bearer ${key}` } : {};
}

// API helper functions
export async function fetchDocuments(
  page: number = 1,
  pageSize: number = 20,
  folderId?: string | null
): Promise<PaginatedResponse<Document>> {
  const params = new URLSearchParams({
    page: String(page),
    pageSize: String(pageSize),
  });
  if (folderId) params.set("folder_id", folderId);
  const res = await fetch(`${API_BASE}/api/documents?${params.toString()}`, {
    headers: authHeaders(),
  });
  if (!res.ok) throw new Error("Failed to fetch documents");
  return res.json();
}

export async function uploadDocument(file: File): Promise<{ document: Document }> {
  const formData = new FormData();
  formData.append("file", file);
  const res = await fetch(`${API_BASE}/api/documents/upload`, {
    method: "POST",
    headers: authHeaders(),
    body: formData,
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Upload failed");
  }
  return res.json();
}

export async function processDocument(documentId: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/documents/process`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ documentId }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Processing failed");
  }
}

export async function deleteDocument(documentId: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/documents/${documentId}`, {
    method: "DELETE",
    headers: authHeaders(),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Delete failed");
  }
}

export async function searchDocuments(
  query: string,
  topK?: number,
  threshold?: number,
  documentIds?: string[],
  searchMode?: SearchMode,
  folderId?: string | null
): Promise<SearchResponse> {
  const res = await fetch(`${API_BASE}/api/search`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({
      query,
      topK,
      threshold,
      documentIds,
      searchMode,
      folder_id: folderId ?? undefined,
    }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Search failed");
  }
  return res.json();
}

export async function uploadDocuments(
  files: File[]
): Promise<{ documents: Document[]; message: string }> {
  const formData = new FormData();
  for (const file of files) {
    formData.append("files", file);
  }
  const res = await fetch(`${API_BASE}/api/documents/upload-bulk`, {
    method: "POST",
    headers: authHeaders(),
    body: formData,
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Upload failed");
  }
  return res.json();
}

export async function refreshDocument(
  documentId: string
): Promise<{ document: Document; message: string }> {
  const res = await fetch(`${API_BASE}/api/documents/${documentId}/refresh`, {
    method: "POST",
    headers: authHeaders(),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Refresh failed");
  }
  return res.json();
}

export async function ingestURLs(
  urls: string[],
  crawl?: boolean,
  maxDepth?: number,
  maxPages?: number
): Promise<{ documents: Document[]; message: string }> {
  const res = await fetch(`${API_BASE}/api/documents/ingest`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ urls, crawl, maxDepth, maxPages }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Ingestion failed");
  }
  return res.json();
}

export async function fetchDocumentDetail(id: string): Promise<{
  document: Document;
  chunks: Array<{
    id: string;
    content: string;
    page_number: number;
    chunk_index: number;
    metadata: Record<string, unknown>;
  }>;
  chunkCount: number;
}> {
  const res = await fetch(`${API_BASE}/api/documents/${id}`, {
    headers: authHeaders(),
  });
  if (!res.ok) throw new Error("Failed to fetch document");
  return res.json();
}

// --- Tag API ---

export async function fetchTags(): Promise<{ tags: Tag[] }> {
  const res = await fetch(`${API_BASE}/api/tags`, { headers: authHeaders() });
  if (!res.ok) throw new Error("Failed to fetch tags");
  return res.json();
}

export async function createTag(
  name: string,
  color?: string
): Promise<{ tag: Tag }> {
  const res = await fetch(`${API_BASE}/api/tags`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ name, color }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Failed to create tag");
  }
  return res.json();
}

export async function deleteTag(tagId: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/tags/${tagId}`, {
    method: "DELETE",
    headers: authHeaders(),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Failed to delete tag");
  }
}

export async function addTagToDocument(
  documentId: string,
  tagId: string
): Promise<{ tags: Tag[] }> {
  const res = await fetch(`${API_BASE}/api/documents/${documentId}/tags`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ tagId }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Failed to add tag");
  }
  return res.json();
}

export async function removeTagFromDocument(
  documentId: string,
  tagId: string
): Promise<{ tags: Tag[] }> {
  const res = await fetch(
    `${API_BASE}/api/documents/${documentId}/tags/${tagId}`,
    { method: "DELETE", headers: authHeaders() }
  );
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Failed to remove tag");
  }
  return res.json();
}

// --- Folder API ---

export async function fetchFolders(
  parentId?: string | null
): Promise<{ folders: Folder[] }> {
  const params = new URLSearchParams();
  if (parentId) params.set("parent_id", parentId);
  const url =
    `${API_BASE}/api/folders` +
    (params.toString() ? `?${params.toString()}` : "");
  const res = await fetch(url, { headers: authHeaders() });
  if (!res.ok) throw new Error("Failed to fetch folders");
  return res.json();
}

export async function createFolder(
  name: string,
  parentId?: string | null
): Promise<{ folder: Folder }> {
  const res = await fetch(`${API_BASE}/api/folders`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ name, parent_id: parentId ?? null }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Failed to create folder");
  }
  return res.json();
}

export async function deleteFolder(folderId: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/folders/${folderId}`, {
    method: "DELETE",
    headers: authHeaders(),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Failed to delete folder");
  }
}

export async function moveDocument(
  documentId: string,
  folderId: string | null
): Promise<void> {
  const res = await fetch(`${API_BASE}/api/documents/${documentId}/move`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify({ folder_id: folderId }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Failed to move document");
  }
}

// --- Agent API ---

export async function fetchAgentSessions(): Promise<{ sessions: AgentSession[] }> {
  const res = await fetch(`${API_BASE}/api/agent/sessions`, {
    headers: authHeaders(),
  });
  if (!res.ok) throw new Error("Failed to fetch agent sessions");
  return res.json();
}

export async function createAgentSession(input: {
  provider: LLMProvider;
  model: string;
  title?: string;
  folder_id?: string | null;
}): Promise<{ session: AgentSession }> {
  const res = await fetch(`${API_BASE}/api/agent/sessions`, {
    method: "POST",
    headers: { "Content-Type": "application/json", ...authHeaders() },
    body: JSON.stringify(input),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Failed to create session");
  }
  return res.json();
}

export async function deleteAgentSession(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/agent/sessions/${id}`, {
    method: "DELETE",
    headers: authHeaders(),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Failed to delete session");
  }
}

export async function fetchAgentMessages(
  sessionId: string,
): Promise<{ messages: AgentMessage[] }> {
  const res = await fetch(
    `${API_BASE}/api/agent/sessions/${sessionId}/messages`,
    { headers: authHeaders() },
  );
  if (!res.ok) throw new Error("Failed to fetch messages");
  return res.json();
}

export async function sendAgentMessage(
  sessionId: string,
  content: string,
  llmApiKey: string,
): Promise<void> {
  const res = await fetch(
    `${API_BASE}/api/agent/sessions/${sessionId}/messages`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-LLM-API-Key": llmApiKey,
        ...authHeaders(),
      },
      body: JSON.stringify({ content }),
    },
  );
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Failed to send message");
  }
}
