"use client";

import { useRouter } from "next/navigation";
import { FileUpload } from "@/components/file-upload";
import { useAppStore } from "@/store/app-store";
import {
  uploadDocument,
  processDocument as triggerProcess,
} from "@/store/app-store";

export default function UploadPage() {
  const router = useRouter();
  const { addDocument, updateDocumentStatus } = useAppStore();

  const handleUpload = async (file: File) => {
    const { document } = await uploadDocument(file);
    addDocument(document);

    // Auto-trigger processing
    updateDocumentStatus(document.id, "processing");
    triggerProcess(document.id).catch((err) => {
      console.error("Auto-process failed:", err);
      updateDocumentStatus(document.id, "failed", String(err));
    });

    // Navigate to dashboard after a brief delay so user sees success message
    setTimeout(() => router.push("/"), 1500);
  };

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-neutral-900 dark:text-white">
          Upload Document
        </h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          Upload a PDF document to extract text, generate embeddings, and enable
          semantic search.
        </p>
      </div>

      <FileUpload onUpload={handleUpload} />

      <div className="rounded-xl border border-neutral-200 bg-neutral-50 p-5 dark:border-neutral-800 dark:bg-neutral-900">
        <h3 className="mb-3 text-sm font-semibold text-neutral-900 dark:text-white">
          How it works
        </h3>
        <ol className="space-y-2 text-sm text-neutral-600 dark:text-neutral-400">
          <li className="flex gap-3">
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-blue-100 text-xs font-bold text-blue-700 dark:bg-blue-900/40 dark:text-blue-400">
              1
            </span>
            <span>Upload a PDF document (up to 50MB)</span>
          </li>
          <li className="flex gap-3">
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-blue-100 text-xs font-bold text-blue-700 dark:bg-blue-900/40 dark:text-blue-400">
              2
            </span>
            <span>
              Text is extracted and split into intelligent semantic chunks
            </span>
          </li>
          <li className="flex gap-3">
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-blue-100 text-xs font-bold text-blue-700 dark:bg-blue-900/40 dark:text-blue-400">
              3
            </span>
            <span>
              Vector embeddings are generated using a local ML model
            </span>
          </li>
          <li className="flex gap-3">
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-blue-100 text-xs font-bold text-blue-700 dark:bg-blue-900/40 dark:text-blue-400">
              4
            </span>
            <span>
              Search across your documents using natural language queries
            </span>
          </li>
        </ol>
      </div>
    </div>
  );
}
