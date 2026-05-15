import type { LLMProvider } from "./types";

const PREFIX = "docinsight_llm_key_";

export function getLLMKey(provider: LLMProvider): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(PREFIX + provider);
}

export function setLLMKey(provider: LLMProvider, key: string): void {
  if (typeof window === "undefined") return;
  localStorage.setItem(PREFIX + provider, key);
}

export function clearLLMKey(provider: LLMProvider): void {
  if (typeof window === "undefined") return;
  localStorage.removeItem(PREFIX + provider);
}

export function hasAnyLLMKey(): boolean {
  if (typeof window === "undefined") return false;
  return getLLMKey("anthropic") !== null || getLLMKey("openai") !== null;
}
