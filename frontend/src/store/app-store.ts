import { create } from "zustand";
import type { Document, SearchResult } from "@/lib/types";

interface AppState {
  documents: Document[];
  documentsLoading: boolean;
  totalDocuments: number;
  currentPage: number;

  searchQuery: string;
  searchResults: SearchResult[];
  searchLoading: boolean;
  searchTookMs: number;
  selectedDocumentIds: string[];

  selectedFolderId: string | null;

  darkMode: boolean;

  setDocuments: (docs: Document[], total: number) => void;
  setDocumentsLoading: (loading: boolean) => void;
  setCurrentPage: (page: number) => void;
  updateDocumentStatus: (id: string, status: Document["status"], errorMessage?: string) => void;
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
        d.id === id ? { ...d, status, error_message: errorMessage ?? d.error_message } : d,
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
  setSearchResults: (results, tookMs) => set({ searchResults: results, searchTookMs: tookMs }),
  setSearchLoading: (loading) => set({ searchLoading: loading }),
  setSelectedDocumentIds: (ids) => set({ selectedDocumentIds: ids }),
  clearSearch: () => set({ searchQuery: "", searchResults: [], searchTookMs: 0 }),

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
