import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { FileText, Globe, AlertCircle, CheckCircle2 } from "lucide-react";
import { useAppStore } from "@/store/app-store";
import { addDocuments, ingestURLs } from "@/lib/api";

type Tab = "pdf" | "url";

export default function AddContentPage() {
  const navigate = useNavigate();
  const { addDocument } = useAppStore();
  const [activeTab, setActiveTab] = useState<Tab>("pdf");

  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);
  const [addSuccess, setAddSuccess] = useState<string | null>(null);

  const [urlInput, setUrlInput] = useState("");
  const [ingesting, setIngesting] = useState(false);
  const [ingestError, setIngestError] = useState<string | null>(null);
  const [ingestSuccess, setIngestSuccess] = useState<string | null>(null);
  const [crawlEnabled, setCrawlEnabled] = useState(false);
  const [crawlDepth, setCrawlDepth] = useState(3);
  const [crawlMaxPages, setCrawlMaxPages] = useState(20);

  const handleAddPDFs = async () => {
    setAdding(true);
    setAddError(null);
    setAddSuccess(null);

    try {
      const res = await addDocuments();
      for (const doc of res.documents) {
        addDocument(doc);
      }
      if (res.documents.length === 0) {
        // User cancelled the native dialog or selected nothing.
        return;
      }
      setAddSuccess(res.message);
      setTimeout(() => navigate("/"), 1500);
    } catch (err) {
      setAddError(err instanceof Error ? err.message : "Upload failed");
    } finally {
      setAdding(false);
    }
  };

  const handleIngestURLs = async () => {
    const urls = urlInput
      .split("\n")
      .map((u) => u.trim())
      .filter((u) => u.length > 0);

    if (urls.length === 0) {
      setIngestError("Enter at least one URL");
      return;
    }

    setIngesting(true);
    setIngestError(null);
    setIngestSuccess(null);

    try {
      const isSingleURL = urls.length === 1;
      const res = await ingestURLs(
        urls,
        isSingleURL && crawlEnabled ? true : undefined,
        isSingleURL && crawlEnabled ? crawlDepth : undefined,
        isSingleURL && crawlEnabled ? crawlMaxPages : undefined
      );
      for (const doc of res.documents) {
        addDocument(doc);
      }
      setIngestSuccess(res.message);
      setUrlInput("");
      setTimeout(() => navigate("/"), 2000);
    } catch (err) {
      setIngestError(err instanceof Error ? err.message : "Ingestion failed");
    } finally {
      setIngesting(false);
    }
  };

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-neutral-900 dark:text-white">
          Add Content
        </h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          Upload a PDF or provide web page URLs to extract, embed, and search.
        </p>
      </div>

      <div className="flex rounded-lg border border-neutral-200 bg-neutral-50 p-1 dark:border-neutral-800 dark:bg-neutral-900">
        <button
          onClick={() => setActiveTab("pdf")}
          className={`flex flex-1 items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium transition-all ${
            activeTab === "pdf"
              ? "bg-white text-neutral-900 shadow-sm dark:bg-neutral-800 dark:text-white"
              : "text-neutral-500 hover:text-neutral-700 dark:text-neutral-400 dark:hover:text-neutral-300"
          }`}
        >
          <FileText size={16} />
          Upload PDF
        </button>
        <button
          onClick={() => setActiveTab("url")}
          className={`flex flex-1 items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium transition-all ${
            activeTab === "url"
              ? "bg-white text-neutral-900 shadow-sm dark:bg-neutral-800 dark:text-white"
              : "text-neutral-500 hover:text-neutral-700 dark:text-neutral-400 dark:hover:text-neutral-300"
          }`}
        >
          <Globe size={16} />
          Add URLs
        </button>
      </div>

      {activeTab === "pdf" && (
        <div className="space-y-4">
          <div className="flex flex-col items-center justify-center rounded-xl border-2 border-dashed border-neutral-300 bg-neutral-50 px-6 py-12 text-center dark:border-neutral-700 dark:bg-neutral-900">
            <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-blue-100 dark:bg-blue-900/40">
              <FileText size={24} className="text-blue-600 dark:text-blue-400" />
            </div>
            <p className="mb-1 text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Choose one or more PDF files to add
            </p>
            <p className="mb-5 text-xs text-neutral-500 dark:text-neutral-400">
              A native file picker will open. Up to 50MB per file.
            </p>
            <button
              onClick={handleAddPDFs}
              disabled={adding}
              className="rounded-lg bg-blue-600 px-5 py-2.5 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
            >
              {adding ? "Adding..." : "Choose PDFs"}
            </button>
          </div>

          {addError && (
            <div className="flex items-center gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400">
              <AlertCircle size={16} className="shrink-0" />
              {addError}
            </div>
          )}

          {addSuccess && (
            <div className="flex items-center gap-2 rounded-lg border border-green-200 bg-green-50 p-3 text-sm text-green-700 dark:border-green-800 dark:bg-green-900/20 dark:text-green-400">
              <CheckCircle2 size={16} className="shrink-0" />
              {addSuccess}
            </div>
          )}
        </div>
      )}

      {activeTab === "url" && (
        <div className="space-y-4">
          <div>
            <label className="mb-2 block text-sm font-medium text-neutral-700 dark:text-neutral-300">
              Web page URLs (one per line)
            </label>
            <textarea
              value={urlInput}
              onChange={(e) => setUrlInput(e.target.value)}
              placeholder={"https://example.com/article\nhttps://blog.example.com/post"}
              rows={5}
              className="w-full rounded-xl border border-neutral-300 bg-white px-4 py-3 text-sm text-neutral-900 placeholder-neutral-400 focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-500/20 dark:border-neutral-700 dark:bg-neutral-900 dark:text-white dark:placeholder-neutral-500"
            />
            <p className="mt-1.5 text-xs text-neutral-500 dark:text-neutral-400">
              Up to 10 URLs. Only http and https schemes are supported.
            </p>
          </div>

          {urlInput.trim().split("\n").filter((u) => u.trim()).length === 1 && (
            <div className="space-y-3 rounded-lg border border-neutral-200 bg-neutral-50 p-4 dark:border-neutral-700 dark:bg-neutral-800/50">
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={crawlEnabled}
                  onChange={(e) => setCrawlEnabled(e.target.checked)}
                  className="rounded border-neutral-300 text-blue-600 focus:ring-blue-500"
                />
                <span className="font-medium text-neutral-700 dark:text-neutral-300">
                  Crawl linked pages
                </span>
              </label>
              {crawlEnabled && (
                <div className="flex gap-4 pl-6">
                  <div className="space-y-1">
                    <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
                      Max Depth
                    </label>
                    <input
                      type="number"
                      min={1}
                      max={5}
                      value={crawlDepth}
                      onChange={(e) => setCrawlDepth(parseInt(e.target.value, 10) || 3)}
                      className="w-20 rounded-lg border border-neutral-200 bg-white px-3 py-1.5 text-sm dark:border-neutral-600 dark:bg-neutral-800 dark:text-white"
                    />
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
                      Max Pages
                    </label>
                    <input
                      type="number"
                      min={1}
                      max={100}
                      value={crawlMaxPages}
                      onChange={(e) => setCrawlMaxPages(parseInt(e.target.value, 10) || 20)}
                      className="w-20 rounded-lg border border-neutral-200 bg-white px-3 py-1.5 text-sm dark:border-neutral-600 dark:bg-neutral-800 dark:text-white"
                    />
                  </div>
                </div>
              )}
            </div>
          )}

          <button
            onClick={handleIngestURLs}
            disabled={ingesting || urlInput.trim().length === 0}
            className="w-full rounded-lg bg-blue-600 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
          >
            {ingesting ? "Fetching pages..." : "Ingest URLs"}
          </button>

          {ingestError && (
            <div className="flex items-center gap-2 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400">
              <AlertCircle size={16} className="shrink-0" />
              {ingestError}
            </div>
          )}

          {ingestSuccess && (
            <div className="flex items-center gap-2 rounded-lg border border-green-200 bg-green-50 p-3 text-sm text-green-700 dark:border-green-800 dark:bg-green-900/20 dark:text-green-400">
              <CheckCircle2 size={16} className="shrink-0" />
              {ingestSuccess}
            </div>
          )}
        </div>
      )}

      <div className="rounded-xl border border-neutral-200 bg-neutral-50 p-5 dark:border-neutral-800 dark:bg-neutral-900">
        <h3 className="mb-3 text-sm font-semibold text-neutral-900 dark:text-white">
          How it works
        </h3>
        <ol className="space-y-2 text-sm text-neutral-600 dark:text-neutral-400">
          <li className="flex gap-3">
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-blue-100 text-xs font-bold text-blue-700 dark:bg-blue-900/40 dark:text-blue-400">
              1
            </span>
            <span>
              {activeTab === "pdf"
                ? "Upload a PDF document (up to 50MB)"
                : "Provide web page URLs to fetch and extract content"}
            </span>
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
              Search across all your content using natural language queries
            </span>
          </li>
        </ol>
      </div>
    </div>
  );
}
