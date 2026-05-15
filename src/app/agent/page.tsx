"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Sparkles,
  Send,
  Trash2,
  Plus,
  Settings,
  AlertCircle,
} from "lucide-react";
import { useSSE } from "@/hooks/use-sse";
import {
  fetchAgentSessions,
  createAgentSession,
  deleteAgentSession,
  fetchAgentMessages,
  sendAgentMessage,
  fetchFolders,
} from "@/store/app-store";
import { getLLMKey, hasAnyLLMKey } from "@/lib/llm-key-storage";
import { SettingsLLMKeys } from "@/components/settings-llm-keys";
import { AgentMessageView } from "@/components/agent-message";
import { MicButton } from "@/components/mic-button";
import type {
  AgentSession,
  AgentMessage,
  Citation,
  Folder,
  LLMProvider,
} from "@/lib/types";

const MODELS: Record<LLMProvider, { value: string; label: string }[]> = {
  anthropic: [
    { value: "claude-opus-4-6", label: "Claude Opus 4.6" },
    { value: "claude-sonnet-4-6", label: "Claude Sonnet 4.6" },
    { value: "claude-haiku-4-5-20251001", label: "Claude Haiku 4.5" },
  ],
  openai: [
    { value: "gpt-4o", label: "GPT-4o" },
    { value: "gpt-4o-mini", label: "GPT-4o mini" },
  ],
};

export default function AgentPage() {
  const [sessions, setSessions] = useState<AgentSession[]>([]);
  const [selectedSession, setSelectedSession] = useState<AgentSession | null>(
    null,
  );
  const [messages, setMessages] = useState<AgentMessage[]>([]);
  const [input, setInput] = useState("");
  const [sending, setSending] = useState(false);
  const [streamingText, setStreamingText] = useState("");
  const [streamingCitations, setStreamingCitations] = useState<Citation[]>([]);
  const [showSettings, setShowSettings] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [folders, setFolders] = useState<Folder[]>([]);
  const [hasKey, setHasKey] = useState(false);

  // New session form
  const [newProvider, setNewProvider] = useState<LLMProvider>("anthropic");
  const [newModel, setNewModel] = useState<string>(MODELS.anthropic[0].value);
  const [newTitle, setNewTitle] = useState("");
  const [newFolderId, setNewFolderId] = useState<string | null>(null);

  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setHasKey(hasAnyLLMKey());
  }, [showSettings]);

  useEffect(() => {
    fetchAgentSessions()
      .then((res) => setSessions(res.sessions ?? []))
      .catch((err) => setError(String(err)));
    fetchFolders()
      .then((res) => setFolders(res.folders ?? []))
      .catch(() => {});
  }, []);

  useEffect(() => {
    if (!selectedSession) {
      setMessages([]);
      return;
    }
    setStreamingText("");
    setStreamingCitations([]);
    fetchAgentMessages(selectedSession.id)
      .then((res) => setMessages(res.messages ?? []))
      .catch((err) => setError(String(err)));
  }, [selectedSession]);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages, streamingText]);

  const handleSSEEvent = useCallback(
    (event: { type: string; data: Record<string, unknown> }) => {
      if (!selectedSession) return;
      const sessionId = event.data["session_id"] as string | undefined;
      if (sessionId !== selectedSession.id) return;

      if (event.type === "agent.delta") {
        const text = (event.data["text"] as string) ?? "";
        setStreamingText((prev) => prev + text);
      } else if (event.type === "agent.tool_result") {
        const cits = (event.data["citations"] as Citation[]) ?? [];
        setStreamingCitations((prev) => [...prev, ...cits]);
      } else if (event.type === "agent.complete") {
        // Reload the messages from the server to capture the persisted assistant message
        fetchAgentMessages(selectedSession.id)
          .then((res) => setMessages(res.messages ?? []))
          .catch(() => {});
        setStreamingText("");
        setStreamingCitations([]);
        setSending(false);
      } else if (event.type === "agent.error") {
        setError(String(event.data["error"] ?? "Agent error"));
        setStreamingText("");
        setStreamingCitations([]);
        setSending(false);
      }
    },
    [selectedSession],
  );

  useSSE(handleSSEEvent);

  const handleCreate = async () => {
    setError(null);
    try {
      const res = await createAgentSession({
        provider: newProvider,
        model: newModel,
        title: newTitle || undefined,
        folder_id: newFolderId,
      });
      setSessions((prev) => [res.session, ...prev]);
      setSelectedSession(res.session);
      setShowCreate(false);
      setNewTitle("");
      setNewFolderId(null);
    } catch (err) {
      setError(String(err));
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm("Delete this conversation?")) return;
    try {
      await deleteAgentSession(id);
      setSessions((prev) => prev.filter((s) => s.id !== id));
      if (selectedSession?.id === id) {
        setSelectedSession(null);
      }
    } catch (err) {
      setError(String(err));
    }
  };

  const handleSend = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || !selectedSession) return;

    const key = getLLMKey(selectedSession.provider);
    if (!key) {
      setError(`No ${selectedSession.provider} API key configured. Open settings.`);
      return;
    }

    const userMsg: AgentMessage = {
      id: `pending-${Date.now()}`,
      session_id: selectedSession.id,
      role: "user",
      content: input.trim(),
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, userMsg]);
    setInput("");
    setSending(true);
    setError(null);

    try {
      await sendAgentMessage(selectedSession.id, userMsg.content, key);
    } catch (err) {
      setError(String(err));
      setSending(false);
    }
  };

  const folderName = useMemo(() => {
    if (!selectedSession?.folder_id) return null;
    return folders.find((f) => f.id === selectedSession.folder_id)?.name;
  }, [selectedSession, folders]);

  return (
    <div className="flex h-[calc(100vh-8rem)] gap-4">
      {/* Sessions sidebar */}
      <aside className="flex w-64 shrink-0 flex-col rounded-xl border border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900">
        <div className="flex items-center justify-between border-b border-neutral-200 px-3 py-2 dark:border-neutral-800">
          <span className="text-xs font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
            Conversations
          </span>
          <div className="flex gap-1">
            <button
              type="button"
              onClick={() => setShowSettings(true)}
              className="rounded p-1 text-neutral-500 hover:bg-neutral-100 dark:hover:bg-neutral-800"
              title="LLM API keys"
            >
              <Settings size={14} />
            </button>
            <button
              type="button"
              onClick={() => setShowCreate(true)}
              className="rounded p-1 text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-900/30"
              title="New conversation"
            >
              <Plus size={14} />
            </button>
          </div>
        </div>
        <div className="flex-1 overflow-y-auto p-2">
          {sessions.length === 0 ? (
            <p className="px-2 py-2 text-xs text-neutral-500 dark:text-neutral-400">
              No conversations yet
            </p>
          ) : (
            sessions.map((s) => (
              <div
                key={s.id}
                className={`group flex items-center gap-1 rounded-md px-2 py-1.5 text-sm ${
                  selectedSession?.id === s.id
                    ? "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
                    : "text-neutral-700 hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800"
                }`}
              >
                <button
                  type="button"
                  onClick={() => setSelectedSession(s)}
                  className="flex-1 truncate text-left"
                >
                  {s.title || "Untitled"}
                </button>
                <button
                  type="button"
                  onClick={() => handleDelete(s.id)}
                  className="opacity-0 transition-opacity hover:text-red-600 group-hover:opacity-100"
                >
                  <Trash2 size={12} />
                </button>
              </div>
            ))
          )}
        </div>
      </aside>

      {/* Chat area */}
      <div className="flex min-w-0 flex-1 flex-col rounded-xl border border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900">
        {selectedSession ? (
          <>
            <div className="flex items-center justify-between border-b border-neutral-200 px-4 py-2 dark:border-neutral-800">
              <div className="min-w-0">
                <p className="truncate text-sm font-semibold text-neutral-900 dark:text-white">
                  {selectedSession.title || "Untitled"}
                </p>
                <p className="truncate text-xs text-neutral-500 dark:text-neutral-400">
                  {selectedSession.provider} · {selectedSession.model}
                  {folderName ? ` · scoped to "${folderName}"` : ""}
                </p>
              </div>
            </div>

            {!hasKey && (
              <div className="m-3 flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900 dark:bg-amber-900/20 dark:text-amber-300">
                <AlertCircle size={14} className="mt-0.5 shrink-0" />
                <span>
                  Add your {selectedSession.provider} API key in settings to start
                  chatting.
                </span>
              </div>
            )}

            <div
              ref={scrollRef}
              className="flex-1 space-y-4 overflow-y-auto p-4"
            >
              {messages.map((m) => (
                <AgentMessageView key={m.id} message={m} />
              ))}
              {streamingText && (
                <AgentMessageView
                  message={{
                    id: "streaming",
                    session_id: selectedSession.id,
                    role: "assistant",
                    content: streamingText,
                    citations: streamingCitations,
                    created_at: new Date().toISOString(),
                  }}
                />
              )}
              {sending && !streamingText && (
                <div className="flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
                  <div className="h-2 w-2 animate-pulse rounded-full bg-blue-500" />
                  Thinking…
                </div>
              )}
            </div>

            {error && (
              <div className="mx-4 mb-2 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-400">
                {error}
              </div>
            )}

            <form
              onSubmit={handleSend}
              className="flex gap-2 border-t border-neutral-200 p-3 dark:border-neutral-800"
            >
              <input
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder="Ask about your documents…"
                disabled={sending}
                className="flex-1 rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-800 dark:text-white"
              />
              <MicButton
                onTranscript={(text) =>
                  setInput((prev) => (prev ? prev + " " + text : text))
                }
                disabled={sending}
                size={16}
              />
              <button
                type="submit"
                disabled={sending || !input.trim()}
                className="flex items-center gap-1 rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
              >
                <Send size={14} />
                Send
              </button>
            </form>
          </>
        ) : (
          <div className="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center">
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-blue-100 text-blue-600 dark:bg-blue-900/30 dark:text-blue-300">
              <Sparkles size={20} />
            </div>
            <h3 className="text-lg font-semibold text-neutral-900 dark:text-white">
              Chat with your knowledge base
            </h3>
            <p className="max-w-md text-sm text-neutral-500 dark:text-neutral-400">
              The agent searches your documents for grounding and cites the
              chunks it used. Bring your own Anthropic or OpenAI key — it stays
              in this browser.
            </p>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setShowSettings(true)}
                className="rounded-lg border border-neutral-200 px-4 py-2 text-sm font-medium text-neutral-700 hover:bg-neutral-50 dark:border-neutral-700 dark:text-neutral-300 dark:hover:bg-neutral-800"
              >
                Configure API key
              </button>
              <button
                type="button"
                onClick={() => setShowCreate(true)}
                className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
              >
                New conversation
              </button>
            </div>
          </div>
        )}
      </div>

      {/* Create session modal */}
      {showCreate && (
        <div
          className="fixed inset-0 z-40 flex items-center justify-center bg-black/40"
          onClick={() => setShowCreate(false)}
        >
          <div
            className="w-full max-w-md space-y-3 rounded-xl border border-neutral-200 bg-white p-5 shadow-xl dark:border-neutral-700 dark:bg-neutral-900"
            onClick={(e) => e.stopPropagation()}
          >
            <h3 className="text-sm font-semibold text-neutral-900 dark:text-white">
              New conversation
            </h3>

            <div className="space-y-1">
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
                Title (optional)
              </label>
              <input
                value={newTitle}
                onChange={(e) => setNewTitle(e.target.value)}
                placeholder="Research notes"
                className="w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-800 dark:text-white"
              />
            </div>

            <div className="grid grid-cols-2 gap-2">
              <div>
                <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
                  Provider
                </label>
                <select
                  value={newProvider}
                  onChange={(e) => {
                    const p = e.target.value as LLMProvider;
                    setNewProvider(p);
                    setNewModel(MODELS[p][0].value);
                  }}
                  className="mt-1 w-full rounded-lg border border-neutral-200 bg-white px-2 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-800 dark:text-white"
                >
                  <option value="anthropic">Anthropic</option>
                  <option value="openai">OpenAI</option>
                </select>
              </div>
              <div>
                <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
                  Model
                </label>
                <select
                  value={newModel}
                  onChange={(e) => setNewModel(e.target.value)}
                  className="mt-1 w-full rounded-lg border border-neutral-200 bg-white px-2 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-800 dark:text-white"
                >
                  {MODELS[newProvider].map((m) => (
                    <option key={m.value} value={m.value}>
                      {m.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div>
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
                Scope to folder (optional)
              </label>
              <select
                value={newFolderId ?? ""}
                onChange={(e) =>
                  setNewFolderId(e.target.value === "" ? null : e.target.value)
                }
                className="mt-1 w-full rounded-lg border border-neutral-200 bg-white px-2 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-800 dark:text-white"
              >
                <option value="">All documents</option>
                {folders.map((f) => (
                  <option key={f.id} value={f.id}>
                    {f.name}
                  </option>
                ))}
              </select>
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                onClick={() => setShowCreate(false)}
                className="rounded-lg px-3 py-2 text-sm text-neutral-600 hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleCreate}
                className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
              >
                Create
              </button>
            </div>
          </div>
        </div>
      )}

      <SettingsLLMKeys
        isOpen={showSettings}
        onClose={() => setShowSettings(false)}
      />
    </div>
  );
}
