"use client";

import { useState, useCallback, useRef } from "react";
import { Upload, FileUp, X, AlertCircle, CheckCircle2 } from "lucide-react";

interface FileUploadProps {
  onUpload: (file: File) => Promise<void>;
  onUploadMultiple?: (files: File[]) => Promise<void>;
}

export function FileUpload({ onUpload, onUploadMultiple }: FileUploadProps) {
  const [dragging, setDragging] = useState(false);
  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const supportsMultiple = !!onUploadMultiple;

  const validateAndAdd = useCallback(
    (files: FileList | File[]) => {
      setError(null);
      setSuccess(false);
      const pdfs: File[] = [];
      for (const file of Array.from(files)) {
        if (file.type === "application/pdf" || file.name.endsWith(".pdf")) {
          pdfs.push(file);
        }
      }
      if (pdfs.length === 0) {
        setError("Please select PDF file(s)");
        return;
      }
      if (supportsMultiple) {
        setSelectedFiles((prev) => [...prev, ...pdfs]);
      } else {
        setSelectedFiles([pdfs[0]]);
      }
    },
    [supportsMultiple]
  );

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setDragging(false);
      validateAndAdd(e.dataTransfer.files);
    },
    [validateAndAdd]
  );

  const handleFileSelect = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      if (e.target.files && e.target.files.length > 0) {
        validateAndAdd(e.target.files);
      }
    },
    [validateAndAdd]
  );

  const handleUpload = async () => {
    if (selectedFiles.length === 0) return;

    setUploading(true);
    setError(null);
    setSuccess(false);

    try {
      if (supportsMultiple && selectedFiles.length > 1) {
        await onUploadMultiple!(selectedFiles);
      } else {
        await onUpload(selectedFiles[0]);
      }
      setSuccess(true);
      setSelectedFiles([]);
      if (inputRef.current) inputRef.current.value = "";
    } catch (err) {
      setError(err instanceof Error ? err.message : "Upload failed");
    } finally {
      setUploading(false);
    }
  };

  const removeFile = (index: number) => {
    setSelectedFiles((prev) => prev.filter((_, i) => i !== index));
  };

  const clearAll = () => {
    setSelectedFiles([]);
    setError(null);
    setSuccess(false);
    if (inputRef.current) inputRef.current.value = "";
  };

  return (
    <div className="space-y-4">
      <div
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => inputRef.current?.click()}
        className={`flex cursor-pointer flex-col items-center justify-center rounded-xl border-2 border-dashed p-12 transition-all ${
          dragging
            ? "border-blue-500 bg-blue-50 dark:border-blue-400 dark:bg-blue-900/20"
            : "border-neutral-300 bg-neutral-50 hover:border-blue-400 hover:bg-blue-50/50 dark:border-neutral-700 dark:bg-neutral-900 dark:hover:border-blue-500"
        }`}
      >
        <Upload
          size={40}
          className={`mb-4 ${
            dragging
              ? "text-blue-500"
              : "text-neutral-400 dark:text-neutral-500"
          }`}
        />
        <p className="text-sm font-medium text-neutral-700 dark:text-neutral-300">
          {dragging
            ? "Drop your PDF(s) here"
            : supportsMultiple
              ? "Drag & drop PDF files here, or click to browse"
              : "Drag & drop a PDF file here, or click to browse"}
        </p>
        <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
          PDF files up to 50MB{supportsMultiple ? " each" : ""}
        </p>
        <input
          ref={inputRef}
          type="file"
          accept=".pdf,application/pdf"
          multiple={supportsMultiple}
          onChange={handleFileSelect}
          className="hidden"
        />
      </div>

      {selectedFiles.length > 0 && (
        <div className="space-y-2">
          {selectedFiles.map((file, i) => (
            <div
              key={`${file.name}-${i}`}
              className="flex items-center gap-3 rounded-lg border border-neutral-200 bg-white p-3 dark:border-neutral-800 dark:bg-neutral-900"
            >
              <FileUp
                size={18}
                className="shrink-0 text-blue-600 dark:text-blue-400"
              />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-neutral-900 dark:text-white">
                  {file.name}
                </p>
                <p className="text-xs text-neutral-500">
                  {(file.size / (1024 * 1024)).toFixed(2)} MB
                </p>
              </div>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  removeFile(i);
                }}
                className="shrink-0 rounded-lg p-1.5 text-neutral-400 hover:bg-neutral-100 dark:hover:bg-neutral-800"
              >
                <X size={14} />
              </button>
            </div>
          ))}
          <div className="flex items-center gap-2">
            <button
              onClick={handleUpload}
              disabled={uploading}
              className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
            >
              {uploading
                ? "Uploading..."
                : `Upload ${selectedFiles.length > 1 ? `${selectedFiles.length} files` : ""}`}
            </button>
            {selectedFiles.length > 1 && (
              <button
                onClick={clearAll}
                className="rounded-lg px-3 py-2 text-xs font-medium text-neutral-500 hover:bg-neutral-100 dark:hover:bg-neutral-800"
              >
                Clear all
              </button>
            )}
          </div>
        </div>
      )}

      {error && (
        <div className="flex items-center gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400">
          <AlertCircle size={16} className="shrink-0" />
          {error}
        </div>
      )}

      {success && (
        <div className="flex items-center gap-2 rounded-lg border border-green-200 bg-green-50 p-3 text-sm text-green-700 dark:border-green-800 dark:bg-green-900/20 dark:text-green-400">
          <CheckCircle2 size={16} className="shrink-0" />
          Document{selectedFiles.length > 1 ? "s" : ""} uploaded successfully!
        </div>
      )}
    </div>
  );
}
