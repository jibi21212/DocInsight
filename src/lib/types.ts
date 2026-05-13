export type DocumentStatus = "pending" | "processing" | "completed" | "failed";

export interface Document {
  id: string;
  name: string;
  upload_date: string;
  page_count: number;
  status: DocumentStatus;
  file_path: string;
  file_size: number;
  error_message: string | null;
  source_type: "pdf" | "web";
  source_url: string | null;
  created_at: string;
  updated_at: string;
}

export interface Chunk {
  id: string;
  document_id: string;
  content: string;
  page_number: number;
  chunk_index: number;
  metadata: ChunkMetadata;
  created_at: string;
}

export interface ChunkMetadata {
  char_count: number;
  word_count: number;
  start_page: number;
  end_page: number;
}

export interface Embedding {
  id: string;
  chunk_id: string;
  embedding: number[];
  created_at: string;
}

export type SearchMode = "hybrid" | "semantic" | "keyword";

export interface SearchResult {
  chunk_id: string;
  content: string;
  similarity: number;
  page_number: number;
  chunk_index: number;
  metadata: ChunkMetadata;
  document_id: string;
  document_name: string;
  source_type: "pdf" | "web";
  source_url: string | null;
  match_type?: "semantic" | "keyword" | "hybrid";
  keyword_score?: number;
}

export interface Tag {
  id: string;
  name: string;
  color: string;
  created_at: string;
}

export interface SearchRequest {
  query: string;
  topK?: number;
  threshold?: number;
  documentIds?: string[];
  searchMode?: SearchMode;
}

export interface SearchResponse {
  results: SearchResult[];
  query: string;
  total: number;
  took_ms: number;
}

export interface UploadResponse {
  document: Document;
  message: string;
}

export interface ProcessingProgress {
  documentId: string;
  stage: "extracting" | "chunking" | "embedding" | "complete" | "error";
  progress: number;
  message: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
}
