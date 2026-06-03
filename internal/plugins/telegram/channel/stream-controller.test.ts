import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("./formatter/telegram.js", () => ({
  formatMarkdown: vi.fn((text: string) => text),
  closeOpenTags: vi.fn((html: string) => html),
  splitMessage: vi.fn((html: string) => [html]),
}));

import { StreamController } from "./stream-controller.js";
import type { StreamControllerDeps } from "./stream-controller.js";

function createMockDeps(): StreamControllerDeps & {
  sendMessage: ReturnType<typeof vi.fn>;
  editMessage: ReturnType<typeof vi.fn>;
  sendDraft: ReturnType<typeof vi.fn>;
} {
  return {
    chatId: 123,
    sendMessage: vi.fn().mockResolvedValue(42),
    editMessage: vi.fn().mockResolvedValue(undefined),
    sendDraft: vi.fn().mockResolvedValue(undefined),
  };
}

describe("StreamController", () => {
  let deps: ReturnType<typeof createMockDeps>;

  beforeEach(() => {
    vi.useFakeTimers();
    deps = createMockDeps();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("text buffering", () => {
    it("accumulates text silently without sending", async () => {
      const sc = new StreamController(deps);
      sc.pushText("Hello");
      sc.pushText(" world");

      // No messages sent — text is buffered
      expect(deps.sendMessage).not.toHaveBeenCalled();
      expect(deps.editMessage).not.toHaveBeenCalled();

      await sc.finalize();
    });

    it("delivers buffered text as new message at finalize", async () => {
      const sc = new StreamController(deps);
      sc.pushText("Hello");
      sc.pushText(" world");

      await sc.finalize();

      // Text delivered as a single new message
      expect(deps.sendMessage).toHaveBeenCalledWith("Hello world", { parse_mode: "HTML" });
    });

    it("sends nothing if no text was pushed", async () => {
      const sc = new StreamController(deps);
      await sc.finalize();

      expect(deps.sendMessage).not.toHaveBeenCalled();
    });

    it("ignores pushText after finalize", async () => {
      const sc = new StreamController(deps);
      sc.pushText("Before");
      await sc.finalize();

      deps.sendMessage.mockClear();
      sc.pushText("After");

      // Should not trigger anything
      expect(deps.sendMessage).not.toHaveBeenCalled();
    });
  });

  describe("work indicator", () => {
    it("creates work message when thinking starts", async () => {
      const sc = new StreamController(deps);
      sc.pushThinking("Let me think...");

      // Flush the enqueued startWorkMessage
      await vi.advanceTimersByTimeAsync(0);

      // Work indicator sent as a new message (header)
      expect(deps.sendMessage).toHaveBeenCalledTimes(1);
      expect(deps.sendMessage).toHaveBeenCalledWith(
        expect.stringContaining("⏱"),
        { parse_mode: "HTML" },
      );
    });

    it("creates work message when tool starts", async () => {
      const sc = new StreamController(deps);
      sc.toolStart("tc1", "bash");

      await vi.advanceTimersByTimeAsync(0);

      expect(deps.sendMessage).toHaveBeenCalledTimes(1);
      expect(deps.sendMessage).toHaveBeenCalledWith(
        expect.stringContaining("⏱"),
        { parse_mode: "HTML" },
      );
    });

    it("edits work message periodically with updated header", async () => {
      const sc = new StreamController(deps, { editIntervalMs: 2000 });
      sc.pushThinking("thinking...");

      await vi.advanceTimersByTimeAsync(0); // flush startWorkMessage

      deps.editMessage.mockClear();

      // Advance past edit interval
      await vi.advanceTimersByTimeAsync(2000);

      expect(deps.editMessage).toHaveBeenCalled();
      const call = deps.editMessage.mock.calls[0];
      expect(call[0]).toBe(42); // workMessageId
      expect(call[1]).toContain("⏱");
    });

    it("collapses work indicator at finalize", async () => {
      const sc = new StreamController(deps, { editIntervalMs: 2000 });
      sc.toolStart("tc1", "bash");

      await vi.advanceTimersByTimeAsync(0);
      deps.editMessage.mockClear();

      sc.toolUpdate("tc1", "completed");
      sc.pushText("Result text");

      await sc.finalize();

      // Work indicator collapsed (edited to summary)
      expect(deps.editMessage).toHaveBeenCalledWith(
        42,
        expect.stringContaining("✅"),
        { parse_mode: "HTML" },
      );

      // Response text sent as NEW message
      const sendCalls = deps.sendMessage.mock.calls;
      const lastSend = sendCalls[sendCalls.length - 1];
      expect(lastSend[0]).toBe("Result text");
    });

    it("does not create work message for text-only responses", async () => {
      const sc = new StreamController(deps);
      sc.pushText("Simple response");
      await sc.finalize();

      // Only one sendMessage call — the response itself, no work indicator
      expect(deps.sendMessage).toHaveBeenCalledTimes(1);
      expect(deps.sendMessage).toHaveBeenCalledWith("Simple response", { parse_mode: "HTML" });
      expect(deps.editMessage).not.toHaveBeenCalled();
    });
  });

  describe("thinking draft", () => {
    it("sends thinking content as draft with brain emoji", async () => {
      const sc = new StreamController(deps, { draftDebounceMs: 100 });
      sc.pushThinking("Analyzing...");

      await vi.advanceTimersByTimeAsync(0); // flush startWorkMessage
      await vi.advanceTimersByTimeAsync(100); // flush draft debounce

      expect(deps.sendDraft).toHaveBeenCalledWith(
        42, // workMessageId used as draftId
        expect.stringContaining("🧠 Analyzing..."),
      );
    });

    it("accumulates thinking across calls", async () => {
      const sc = new StreamController(deps, { draftDebounceMs: 100 });
      sc.pushThinking("First");
      sc.pushThinking(" second");

      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(100);

      const lastCall = deps.sendDraft.mock.calls.at(-1)!;
      expect(lastCall[1]).toContain("🧠 First second");
    });

    it("debounces draft sends", async () => {
      const sc = new StreamController(deps, { draftDebounceMs: 500 });
      sc.pushThinking("A");

      await vi.advanceTimersByTimeAsync(0); // startWorkMessage

      // Before debounce fires
      await vi.advanceTimersByTimeAsync(200);
      expect(deps.sendDraft).not.toHaveBeenCalled();

      // After debounce
      await vi.advanceTimersByTimeAsync(300);
      expect(deps.sendDraft).toHaveBeenCalledTimes(1);
    });

    it("auto-closes thinking when tool starts", async () => {
      const sc = new StreamController(deps, { editIntervalMs: 2000 });
      sc.pushThinking("Thinking about it...");

      await vi.advanceTimersByTimeAsync(0);
      deps.editMessage.mockClear();

      sc.toolStart("tc1", "bash");

      // Advance to trigger edit
      await vi.advanceTimersByTimeAsync(2000);

      // Edit should show tool, not thinking content (thinking was closed)
      const editCall = deps.editMessage.mock.calls.at(-1);
      expect(editCall?.[1]).toContain("🔨 bash");
    });
  });

  describe("tools", () => {
    it("renders tool with in_progress status", async () => {
      const sc = new StreamController(deps, { editIntervalMs: 2000 });
      sc.toolStart("tc1", "Read file");

      await vi.advanceTimersByTimeAsync(0);

      // Work indicator should contain tool line
      expect(deps.sendMessage).toHaveBeenCalledWith(
        expect.stringContaining("⏱"),
        { parse_mode: "HTML" },
      );

      // First edit should show tool with in_progress icon
      await vi.advanceTimersByTimeAsync(2000);
      expect(deps.editMessage).toHaveBeenCalledWith(
        42,
        expect.stringContaining("🔨 Read file"),
        { parse_mode: "HTML" },
      );
    });

    it("updates tool status", async () => {
      const sc = new StreamController(deps, { editIntervalMs: 1000 });
      sc.toolStart("tc1", "npm test");

      await vi.advanceTimersByTimeAsync(0);
      sc.toolUpdate("tc1", "completed");

      await vi.advanceTimersByTimeAsync(1000);

      expect(deps.editMessage).toHaveBeenCalledWith(
        42,
        expect.stringContaining("✅"),
        { parse_mode: "HTML" },
      );
    });

    it("shows result preview in tool update", async () => {
      const sc = new StreamController(deps, { editIntervalMs: 1000 });
      sc.toolStart("tc1", "grep TODO");

      await vi.advanceTimersByTimeAsync(0);
      sc.toolUpdate("tc1", "completed", [
        { type: "content", content: { type: "text", text: "found 3 matches" } },
      ]);

      await vi.advanceTimersByTimeAsync(1000);

      expect(deps.editMessage).toHaveBeenCalledWith(
        42,
        expect.stringContaining("found 3 matches"),
        { parse_mode: "HTML" },
      );
    });

    it("renders multiple tools", async () => {
      const sc = new StreamController(deps, { editIntervalMs: 1000 });
      sc.toolStart("tc1", "read");
      sc.toolUpdate("tc1", "completed");
      sc.toolStart("tc2", "write");

      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(1000);

      const lastEdit = deps.editMessage.mock.calls.at(-1)!;
      expect(lastEdit[1]).toContain("🔨 read");
      expect(lastEdit[1]).toContain("🔨 write");
    });
  });

  describe("finalize", () => {
    it("collapses work then sends response as new message", async () => {
      const sc = new StreamController(deps, { editIntervalMs: 2000 });
      sc.pushThinking("thinking...");
      sc.toolStart("tc1", "bash");
      sc.toolUpdate("tc1", "completed");
      sc.pushText("Here is the answer");

      await vi.advanceTimersByTimeAsync(0); // startWorkMessage

      await sc.finalize();

      // 1. Work indicator created (sendMessage #1)
      // 2. Work indicator collapsed (editMessage)
      // 3. Response text sent (sendMessage #2)
      expect(deps.sendMessage).toHaveBeenCalledTimes(2);
      expect(deps.editMessage).toHaveBeenCalled();

      const responseSend = deps.sendMessage.mock.calls[1];
      expect(responseSend[0]).toBe("Here is the answer");
    });

    it("skips collapse if no work indicator was created", async () => {
      const sc = new StreamController(deps);
      sc.pushText("Quick response");

      await sc.finalize();

      // Only one send — the response
      expect(deps.sendMessage).toHaveBeenCalledTimes(1);
      expect(deps.editMessage).not.toHaveBeenCalled();
    });

    it("clears all timers", async () => {
      const sc = new StreamController(deps, { editIntervalMs: 500, draftDebounceMs: 200 });
      sc.pushThinking("thinking...");
      sc.toolStart("tc1", "bash");

      await vi.advanceTimersByTimeAsync(0);

      await sc.finalize();

      deps.editMessage.mockClear();
      deps.sendDraft.mockClear();

      // Advance time — nothing should fire
      await vi.advanceTimersByTimeAsync(5000);
      expect(deps.editMessage).not.toHaveBeenCalled();
      expect(deps.sendDraft).not.toHaveBeenCalled();
    });

    it("is idempotent", async () => {
      const sc = new StreamController(deps);
      sc.pushText("Hello");

      await sc.finalize();
      deps.sendMessage.mockClear();

      await sc.finalize(); // second call
      expect(deps.sendMessage).not.toHaveBeenCalled();
    });
  });

  describe("abort", () => {
    it("delivers partial text and cleans up", async () => {
      const sc = new StreamController(deps, { editIntervalMs: 2000 });
      sc.pushThinking("thinking...");
      sc.pushText("Partial resp");

      await vi.advanceTimersByTimeAsync(0); // startWorkMessage

      await sc.abort(new Error("connection lost"));

      // Work collapsed + partial text sent
      expect(deps.editMessage).toHaveBeenCalled(); // collapse
      const sends = deps.sendMessage.mock.calls;
      const lastSend = sends[sends.length - 1];
      expect(lastSend[0]).toBe("Partial resp");
    });

    it("does nothing if no content was produced", async () => {
      const sc = new StreamController(deps);
      await sc.abort(new Error("timeout"));

      expect(deps.sendMessage).not.toHaveBeenCalled();
      expect(deps.editMessage).not.toHaveBeenCalled();
    });

    it("is idempotent with finalize", async () => {
      const sc = new StreamController(deps);
      sc.pushText("Hello");

      await sc.finalize();
      deps.sendMessage.mockClear();

      await sc.abort(new Error("late error"));
      expect(deps.sendMessage).not.toHaveBeenCalled();
    });
  });

  describe("error resilience", () => {
    it("does not crash when editMessage rejects", async () => {
      deps.editMessage.mockRejectedValue(new Error("rate limited"));

      const sc = new StreamController(deps, { editIntervalMs: 500 });
      sc.toolStart("tc1", "bash");

      await vi.advanceTimersByTimeAsync(0); // startWorkMessage

      // Edit fails silently
      await vi.advanceTimersByTimeAsync(500);

      // Still functional — finalize should not throw
      sc.pushText("result");
      await sc.finalize();
    });

    it("does not crash when sendMessage rejects during work start", async () => {
      deps.sendMessage.mockRejectedValue(new Error("network error"));

      const sc = new StreamController(deps, { editIntervalMs: 500 });
      sc.pushThinking("thinking...");

      // startWorkMessage fails
      await vi.advanceTimersByTimeAsync(0);

      // Should not throw
      sc.pushText("result");
      await sc.finalize();
    });

    it("does not crash when sendDraft rejects", async () => {
      deps.sendDraft.mockRejectedValue(new Error("draft failed"));

      const sc = new StreamController(deps, { draftDebounceMs: 100 });
      sc.pushThinking("thinking...");

      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(100); // draft debounce fires

      // Should not throw
      sc.pushText("result");
      await sc.finalize();
    });
  });
});
