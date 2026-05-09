export const config = {
  supabase: {
    url: process.env.NEXT_PUBLIC_SUPABASE_URL ?? "",
    anonKey: process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY ?? "",
    serviceRoleKey: process.env.SUPABASE_SERVICE_ROLE_KEY ?? "",
  },
  upload: {
    dir: process.env.UPLOAD_DIR ?? "./uploads",
    maxSizeMB: 50,
  },
  embedding: {
    model: process.env.EMBEDDING_MODEL ?? "Xenova/all-MiniLM-L6-v2",
    dimension: parseInt(process.env.EMBEDDING_DIMENSION ?? "384", 10),
  },
  chunking: {
    chunkSize: parseInt(process.env.CHUNK_SIZE ?? "1000", 10),
    chunkOverlap: parseInt(process.env.CHUNK_OVERLAP ?? "200", 10),
  },
  search: {
    topK: parseInt(process.env.SEARCH_TOP_K ?? "10", 10),
    similarityThreshold: parseFloat(
      process.env.SIMILARITY_THRESHOLD ?? "0.5"
    ),
  },
} as const;
