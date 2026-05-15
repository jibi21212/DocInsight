"use client";

import { useEffect, useRef } from "react";
import { Mic, MicOff } from "lucide-react";
import { useSpeechRecognition } from "@/hooks/use-speech-recognition";

type Props = {
  /** Called once for each final transcript chunk. */
  onTranscript: (text: string) => void;
  /** Disable the button (e.g. while submitting). */
  disabled?: boolean;
  /** Icon size in px. Defaults to 18. */
  size?: number;
};

export function MicButton({ onTranscript, disabled, size = 18 }: Props) {
  const { isSupported, isListening, transcript, error, start, stop, reset } =
    useSpeechRecognition();

  // When new final-transcript text arrives, forward to the parent and reset
  // so subsequent utterances don't accumulate inside the hook.
  const lastCommitted = useRef("");
  useEffect(() => {
    if (transcript && transcript !== lastCommitted.current) {
      const delta = transcript.slice(lastCommitted.current.length).trim();
      if (delta) {
        onTranscript(delta);
      }
      lastCommitted.current = transcript;
    }
  }, [transcript, onTranscript]);

  // Clear the committed cache once recognition stops, so the next
  // session starts fresh.
  useEffect(() => {
    if (!isListening && transcript) {
      reset();
      lastCommitted.current = "";
    }
  }, [isListening, transcript, reset]);

  const toggle = () => {
    if (isListening) {
      stop();
    } else {
      start();
    }
  };

  if (!isSupported) {
    return (
      <button
        type="button"
        disabled
        title="Voice input not supported in this browser"
        aria-label="Voice input not supported"
        className="rounded-xl border border-neutral-200 bg-white px-3 text-neutral-300 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-700"
      >
        <MicOff size={size} />
      </button>
    );
  }

  return (
    <button
      type="button"
      onClick={toggle}
      disabled={disabled}
      title={
        error
          ? error
          : isListening
            ? "Stop recording"
            : "Speak your prompt"
      }
      aria-label={isListening ? "Stop recording" : "Start voice input"}
      aria-pressed={isListening}
      className={`relative rounded-xl border px-3 transition-colors disabled:opacity-50 ${
        isListening
          ? "border-red-500 bg-red-50 text-red-600 dark:border-red-500 dark:bg-red-900/20 dark:text-red-400"
          : error
            ? "border-amber-400 bg-amber-50 text-amber-700 dark:border-amber-500 dark:bg-amber-900/20 dark:text-amber-300"
            : "border-neutral-200 bg-white text-neutral-600 hover:bg-neutral-50 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-400"
      }`}
    >
      <Mic size={size} />
      {isListening && (
        <span
          aria-hidden
          className="absolute -right-0.5 -top-0.5 inline-flex h-2.5 w-2.5 animate-pulse rounded-full bg-red-500"
        />
      )}
    </button>
  );
}
