import { create } from "zustand";
import type {
  Document,
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

// API helper functions
export async function fetchDocuments(
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<Document>> {
  const res = await fetch(
    `/api/documents?page=${page}&pageSize=${pageSize}`
  );
  if (!res.ok) throw new Error("Failed to fetch documents");
  return res.json();
}

export async function uploadDocument(file: File): Promise<{ document: Document }> {
  const formData = new FormData();
  formData.append("file", file);
  const res = await fetch("/api/documents/upload", {
    method: "POST",
    body: formData,
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Upload failed");
  }
  return res.json();
}

export async function processDocument(documentId: string): Promise<void> {
  const res = await fetch("/api/documents/process", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ documentId }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Processing failed");
  }
}

export async function deleteDocument(documentId: string): Promise<void> {
  const res = await fetch(`/api/documents/${documentId}`, {
    method: "DELETE",
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
  documentIds?: string[]
): Promise<SearchResponse> {
  const res = await fetch("/api/search", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query, topK, threshold, documentIds }),
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(err.error ?? "Search failed");
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
  const res = await fetch(`/api/documents/${id}`);
  if (!res.ok) throw new Error("Failed to fetch document");
  return res.json();
}
