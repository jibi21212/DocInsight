"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export type SpeechRecognitionOptions = {
  /** BCP-47 language tag. Defaults to the browser locale (or "en-US"). */
  lang?: string;
  /** If true, keeps listening across pauses. Default false (one utterance). */
  continuous?: boolean;
  /** If true, exposes interim (in-progress) transcripts. Default true. */
  interimResults?: boolean;
};

export type SpeechRecognitionHook = {
  /** False when the Web Speech API is unavailable (e.g. Firefox without polyfill). */
  isSupported: boolean;
  /** True while the recognizer is active. */
  isListening: boolean;
  /** Accumulated final-transcript text since the last reset(). */
  transcript: string;
  /** The current in-progress (non-final) transcript, if any. */
  interimTranscript: string;
  /** Most recent friendly error message, or null. */
  error: string | null;
  /** Begin listening. No-op if unsupported or already listening. */
  start: () => void;
  /** Stop listening. No-op if not listening. */
  stop: () => void;
  /** Clear transcript and error state. Does not stop listening. */
  reset: () => void;
};

function resolveCtor(): SpeechRecognitionConstructor | undefined {
  if (typeof window === "undefined") return undefined;
  return window.SpeechRecognition ?? window.webkitSpeechRecognition;
}

function friendlyError(code: string): string {
  switch (code) {
    case "not-allowed":
    case "service-not-allowed":
      return "Microphone permission denied";
    case "no-speech":
      return "No speech detected";
    case "audio-capture":
      return "No microphone found";
    case "network":
      return "Network error during recognition";
    case "aborted":
      return "Recognition aborted";
    default:
      return `Speech recognition error: ${code}`;
  }
}

/**
 * Wraps the browser's Web Speech API as a React hook.
 *
 * Audio handling happens entirely inside the browser vendor's layer
 * (Chrome forwards to Google servers transparently; Safari handles
 * on-device). No audio bytes reach this app.
 */
export function useSpeechRecognition(
  opts: SpeechRecognitionOptions = {},
): SpeechRecognitionHook {
  const Ctor = useRef<SpeechRecognitionConstructor | undefined>(undefined);
  const recognitionRef = useRef<SpeechRecognition | null>(null);
  const [isSupported, setIsSupported] = useState(false);
  const [isListening, setIsListening] = useState(false);
  const [transcript, setTranscript] = useState("");
  const [interimTranscript, setInterimTranscript] = useState("");
  const [error, setError] = useState<string | null>(null);

  // Resolve the constructor once on mount.
  useEffect(() => {
    const found = resolveCtor();
    Ctor.current = found;
    setIsSupported(!!found);
  }, []);

  // Cleanup any in-flight recognition on unmount. Detach the handlers BEFORE
  // tearing down so the asynchronous onend can't call setState on an unmounted
  // hook, and use abort() (synchronous, discards any pending result) rather than
  // stop() (which defers onend and can still deliver one final result).
  useEffect(() => {
    return () => {
      const r = recognitionRef.current;
      if (r) {
        r.onresult = null;
        r.onerror = null;
        r.onend = null;
        r.onstart = null;
        try {
          r.abort();
        } catch {
          // ignore — recognizer may already be stopped
        }
        recognitionRef.current = null;
      }
    };
  }, []);

  const start = useCallback(() => {
    if (!Ctor.current) return;
    if (recognitionRef.current) return; // already listening

    const r = new Ctor.current();
    r.lang =
      opts.lang ??
      (typeof navigator !== "undefined" ? navigator.language : undefined) ??
      "en-US";
    r.continuous = opts.continuous ?? false;
    r.interimResults = opts.interimResults ?? true;
    r.maxAlternatives = 1;

    r.onresult = (event) => {
      let finalDelta = "";
      let interim = "";
      for (let i = event.resultIndex; i < event.results.length; i++) {
        const result = event.results[i];
        const text = result[0]?.transcript ?? "";
        if (result.isFinal) {
          finalDelta += text;
        } else {
          interim += text;
        }
      }
      if (finalDelta) {
        setTranscript((prev) => (prev ? prev + " " + finalDelta.trim() : finalDelta.trim()));
      }
      setInterimTranscript(interim);
    };

    r.onerror = (event) => {
      // "no-speech" is a graceful timeout — surface it as info, not an error.
      if (event.error === "no-speech" || event.error === "aborted") {
        return;
      }
      setError(friendlyError(event.error));
    };

    r.onend = () => {
      setIsListening(false);
      setInterimTranscript("");
      recognitionRef.current = null;
    };

    r.onstart = () => {
      setIsListening(true);
      setError(null);
    };

    recognitionRef.current = r;
    try {
      r.start();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to start recognition");
      recognitionRef.current = null;
    }
  }, [opts.lang, opts.continuous, opts.interimResults]);

  const stop = useCallback(() => {
    const r = recognitionRef.current;
    if (!r) return;
    try {
      r.stop();
    } catch {
      // ignore
    }
  }, []);

  const reset = useCallback(() => {
    setTranscript("");
    setInterimTranscript("");
    setError(null);
  }, []);

  return {
    isSupported,
    isListening,
    transcript,
    interimTranscript,
    error,
    start,
    stop,
    reset,
  };
}
