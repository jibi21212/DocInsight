import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";

// Fake SpeechRecognition constructor with introspectable instance state.
class FakeRecognition extends EventTarget {
  lang = "";
  continuous = false;
  interimResults = false;
  maxAlternatives = 1;
  onresult: ((ev: SpeechRecognitionEvent) => unknown) | null = null;
  onerror: ((ev: SpeechRecognitionErrorEvent) => unknown) | null = null;
  onend: ((ev: Event) => unknown) | null = null;
  onstart: ((ev: Event) => unknown) | null = null;
  startCalls = 0;
  stopCalls = 0;
  abortCalls = 0;
  start() {
    this.startCalls++;
    queueMicrotask(() => this.onstart?.(new Event("start")));
  }
  stop() {
    this.stopCalls++;
    queueMicrotask(() => this.onend?.(new Event("end")));
  }
  abort() {
    this.abortCalls++;
    // Real abort() also fires onend; modeling it lets the unmount test prove the
    // hook detaches handlers before aborting (so this fires into the void).
    queueMicrotask(() => this.onend?.(new Event("end")));
  }
  /** Helper for tests to push a synthetic result event. */
  emitResult(parts: Array<{ transcript: string; isFinal: boolean }>): void {
    const results: SpeechRecognitionResult[] = parts.map((p) => {
      const alt: SpeechRecognitionAlternative = { transcript: p.transcript, confidence: 1 };
      const result = {
        isFinal: p.isFinal,
        length: 1,
        0: alt,
        item: (_i: number) => alt,
      } as unknown as SpeechRecognitionResult;
      return result;
    });
    const list = {
      length: results.length,
      item: (i: number) => results[i],
      ...results,
    } as unknown as SpeechRecognitionResultList;
    const ev = {
      resultIndex: 0,
      results: list,
    } as unknown as SpeechRecognitionEvent;
    this.onresult?.(ev);
  }
  emitError(code: string): void {
    const ev = { error: code } as unknown as SpeechRecognitionErrorEvent;
    this.onerror?.(ev);
  }
}

let lastInstance: FakeRecognition | null = null;

function installFakeAPI() {
  lastInstance = null;
  (window as Window).SpeechRecognition = class extends FakeRecognition {
    constructor() {
      super();
      lastInstance = this;
    }
  } as unknown as SpeechRecognitionConstructor;
}

function uninstallAPI() {
  delete (window as Window).SpeechRecognition;
  delete (window as Window).webkitSpeechRecognition;
}

describe("useSpeechRecognition", () => {
  beforeEach(() => {
    installFakeAPI();
  });
  afterEach(() => {
    uninstallAPI();
    lastInstance = null;
  });

  it("reports supported when SpeechRecognition is available", async () => {
    const { useSpeechRecognition } = await import("./use-speech-recognition");
    const { result } = renderHook(() => useSpeechRecognition());
    expect(result.current.isSupported).toBe(true);
  });

  it("reports not supported when the API is missing", async () => {
    uninstallAPI();
    vi.resetModules();
    const { useSpeechRecognition } = await import("./use-speech-recognition");
    const { result } = renderHook(() => useSpeechRecognition());
    expect(result.current.isSupported).toBe(false);
    // start() is a no-op when unsupported
    act(() => result.current.start());
    expect(result.current.isListening).toBe(false);
  });

  it("toggles isListening between start and stop", async () => {
    const { useSpeechRecognition } = await import("./use-speech-recognition");
    const { result } = renderHook(() => useSpeechRecognition());

    await act(async () => {
      result.current.start();
      // allow microtask queue for onstart
      await Promise.resolve();
    });
    expect(result.current.isListening).toBe(true);
    expect(lastInstance?.startCalls).toBe(1);

    await act(async () => {
      result.current.stop();
      await Promise.resolve();
    });
    expect(result.current.isListening).toBe(false);
    expect(lastInstance?.stopCalls).toBe(1);
  });

  it("accumulates final transcripts and surfaces interim ones", async () => {
    const { useSpeechRecognition } = await import("./use-speech-recognition");
    const { result } = renderHook(() => useSpeechRecognition());

    await act(async () => {
      result.current.start();
      await Promise.resolve();
    });

    act(() => {
      lastInstance!.emitResult([{ transcript: "hello", isFinal: false }]);
    });
    expect(result.current.interimTranscript).toBe("hello");
    expect(result.current.transcript).toBe("");

    act(() => {
      lastInstance!.emitResult([{ transcript: "hello world", isFinal: true }]);
    });
    expect(result.current.transcript).toBe("hello world");

    act(() => {
      lastInstance!.emitResult([{ transcript: "again", isFinal: true }]);
    });
    expect(result.current.transcript).toBe("hello world again");
  });

  it("maps error codes to friendly messages", async () => {
    const { useSpeechRecognition } = await import("./use-speech-recognition");
    const { result } = renderHook(() => useSpeechRecognition());

    await act(async () => {
      result.current.start();
      await Promise.resolve();
    });

    act(() => {
      lastInstance!.emitError("not-allowed");
    });
    expect(result.current.error).toBe("Microphone permission denied");
  });

  it("treats no-speech as a graceful stop, not an error", async () => {
    const { useSpeechRecognition } = await import("./use-speech-recognition");
    const { result } = renderHook(() => useSpeechRecognition());

    await act(async () => {
      result.current.start();
      await Promise.resolve();
    });

    act(() => {
      lastInstance!.emitError("no-speech");
    });
    expect(result.current.error).toBeNull();
  });

  it("aborts native recognition and detaches handlers when the hook unmounts mid-listen", async () => {
    const { useSpeechRecognition } = await import("./use-speech-recognition");
    const { result, unmount } = renderHook(() => useSpeechRecognition());

    await act(async () => {
      result.current.start();
      await Promise.resolve();
    });
    expect(lastInstance?.startCalls).toBe(1);
    const inst = lastInstance!;

    unmount();

    // Unmount must abort() (synchronous, discards pending results), NOT stop()
    // (which defers onend and can still deliver one final result).
    expect(inst.abortCalls).toBe(1);
    expect(inst.stopCalls).toBe(0);
    // Handlers must be detached so the deferred onend can't setState on the
    // now-unmounted hook.
    expect(inst.onend).toBeNull();
    expect(inst.onresult).toBeNull();
    expect(inst.onerror).toBeNull();
    expect(inst.onstart).toBeNull();

    // Flush the microtask abort() queued; with handlers detached it's a no-op
    // and must not throw.
    await act(async () => {
      await Promise.resolve();
    });
    expect(inst.abortCalls).toBe(1);
  });

  it("reset() clears transcript and error but does not stop", async () => {
    const { useSpeechRecognition } = await import("./use-speech-recognition");
    const { result } = renderHook(() => useSpeechRecognition());

    await act(async () => {
      result.current.start();
      await Promise.resolve();
    });

    act(() => {
      lastInstance!.emitResult([{ transcript: "data", isFinal: true }]);
      lastInstance!.emitError("network");
    });
    expect(result.current.transcript).toBe("data");
    expect(result.current.error).not.toBeNull();

    act(() => result.current.reset());
    expect(result.current.transcript).toBe("");
    expect(result.current.error).toBeNull();
    // recognition still active
    expect(result.current.isListening).toBe(true);
  });
});
