import { useEffect, useState } from "react";
import { X, Key, AlertTriangle, Check, Trash2 } from "lucide-react";
import type { LLMProvider } from "@/lib/types";
import { getLLMKey, setLLMKey, clearLLMKey } from "@/lib/llm-key-storage";

interface SettingsLLMKeysProps {
  isOpen: boolean;
  onClose: () => void;
}

const PROVIDERS: { value: LLMProvider; label: string; placeholder: string }[] = [
  { value: "anthropic", label: "Anthropic", placeholder: "sk-ant-..." },
  { value: "openai", label: "OpenAI", placeholder: "sk-..." },
];

export function SettingsLLMKeys({ isOpen, onClose }: SettingsLLMKeysProps) {
  const [provider, setProvider] = useState<LLMProvider>("anthropic");
  const [keyValue, setKeyValue] = useState("");
  const [storedSet, setStoredSet] = useState<Set<LLMProvider>>(new Set());

  useEffect(() => {
    if (!isOpen) return;
    const next = new Set<LLMProvider>();
    PROVIDERS.forEach((p) => {
      if (getLLMKey(p.value)) next.add(p.value);
    });
    setStoredSet(next);
    setKeyValue("");
  }, [isOpen]);

  if (!isOpen) return null;

  const handleSave = () => {
    if (!keyValue.trim()) return;
    setLLMKey(provider, keyValue.trim());
    setStoredSet((prev) => new Set(prev).add(provider));
    setKeyValue("");
  };

  const handleClear = (p: LLMProvider) => {
    clearLLMKey(p);
    setStoredSet((prev) => {
      const next = new Set(prev);
      next.delete(p);
      return next;
    });
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={onClose}
    >
      <div
        className="w-full max-w-lg rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-neutral-700 dark:bg-neutral-900"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-neutral-200 px-5 py-3 dark:border-neutral-700">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-neutral-900 dark:text-white">
            <Key size={16} />
            LLM API Keys
          </h3>
          <button
            type="button"
            onClick={onClose}
            className="text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300"
          >
            <X size={16} />
          </button>
        </div>

        <div className="space-y-4 p-5">
          <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900 dark:bg-amber-900/20 dark:text-amber-300">
            <AlertTriangle size={14} className="mt-0.5 shrink-0" />
            <div>
              Your key is stored only in this browser&apos;s localStorage. It is
              sent to our backend with each agent request and forwarded to the
              provider for that call only — never persisted server-side.
            </div>
          </div>

          <div className="space-y-2">
            <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
              Provider
            </label>
            <div className="flex gap-2">
              {PROVIDERS.map((p) => (
                <button
                  key={p.value}
                  type="button"
                  onClick={() => setProvider(p.value)}
                  className={`flex-1 rounded-lg border px-3 py-2 text-sm font-medium ${
                    provider === p.value
                      ? "border-blue-500 bg-blue-50 text-blue-700 dark:border-blue-500 dark:bg-blue-900/30 dark:text-blue-300"
                      : "border-neutral-200 text-neutral-600 hover:bg-neutral-50 dark:border-neutral-700 dark:text-neutral-300 dark:hover:bg-neutral-800"
                  }`}
                >
                  <div className="flex items-center justify-center gap-1.5">
                    {p.label}
                    {storedSet.has(p.value) && (
                      <Check size={12} className="text-green-600" />
                    )}
                  </div>
                </button>
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400">
              API Key
            </label>
            <input
              type="password"
              value={keyValue}
              onChange={(e) => setKeyValue(e.target.value)}
              placeholder={
                PROVIDERS.find((p) => p.value === provider)?.placeholder
              }
              autoComplete="off"
              className="w-full rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-800 dark:text-white"
            />
            <div className="flex gap-2">
              <button
                type="button"
                onClick={handleSave}
                disabled={!keyValue.trim()}
                className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
              >
                Save key
              </button>
              {storedSet.has(provider) && (
                <button
                  type="button"
                  onClick={() => handleClear(provider)}
                  className="flex items-center gap-1.5 rounded-lg border border-red-200 px-4 py-2 text-sm font-medium text-red-600 hover:bg-red-50 dark:border-red-900 dark:text-red-400 dark:hover:bg-red-900/20"
                >
                  <Trash2 size={12} />
                  Clear {provider}
                </button>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
