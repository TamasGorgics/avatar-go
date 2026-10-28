import {
  appendMessage,
  failMessage,
  finishMessage,
  scrollToEnd,
  setBusy,
  setStatus,
  updateCount,
  type ChatElements,
} from "./dom";
import { createTextStream, type TextStream } from "./stream";
import type { ChatMessage, ChatRequest } from "./types";

export interface ChatController {
  ask(raw: string): Promise<void>;
  readonly busy: boolean;
}

export const MAX_HISTORY = 6;

export function recentHistory(history: ChatMessage[]): ChatMessage[] {
  return history.slice(-MAX_HISTORY);
}

export function createChat(elements: ChatElements, reducedMotion: boolean): ChatController {
  const history: ChatMessage[] = [];
  let busy = false;

  const ask = async (raw: string): Promise<void> => {
    if (busy) {
      return;
    }

    const question = raw.trim();
    if (!question) {
      return;
    }

    elements.input.value = "";
    updateCount(elements.input, elements.count);
    busy = true;
    setBusy(elements, true);
    setStatus(elements.status, "Twin is answering.");

    history.push({ role: "user", content: question });
    appendMessage(elements, elements.userTemplate, question, reducedMotion);
    const bubble = appendMessage(elements, elements.assistantTemplate, "", reducedMotion);
    const textNode = bubble?.querySelector<HTMLElement>('[data-role="text"]') ?? null;
    let answer = "";
    let stream: TextStream | null = null;

    try {
      const request: ChatRequest = { history: recentHistory(history) };
      const response = await fetch("/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(request),
      });

      if (!response.ok) {
        const message = (await response.text()).trim() || "The twin is unavailable right now.";
        failMessage(bubble, message);
        history.pop();
        setStatus(elements.status, "The question could not be answered.");
        return;
      }

      if (!response.body) {
        throw new Error("response body is unavailable");
      }

      const textStream = createTextStream({
        write: (text) => {
          if (textNode) {
            textNode.textContent = text;
          }
        },
        onPaint: () => scrollToEnd(elements.log, "auto", reducedMotion),
        reducedMotion,
      });
      stream = textStream;

      const reader = response.body.getReader();
      const decoder = new TextDecoder();

      for (;;) {
        const { done, value } = await reader.read();
        if (done) {
          break;
        }
        const text = decoder.decode(value, { stream: true });
        answer += text;
        textStream.push(text);
      }

      const tail = decoder.decode();
      answer += tail;
      textStream.push(tail);
      textStream.end();
      await textStream.drained;

      if (textNode) {
        textNode.textContent = answer;
      }
      finishMessage(bubble);
      scrollToEnd(elements.log, "auto", reducedMotion);

      if (answer.trim()) {
        history.push({ role: "assistant", content: answer });
        setStatus(elements.status, "Answer complete.");
      } else {
        if (textNode) {
          textNode.textContent = "The twin did not return an answer.";
        }
        setStatus(elements.status, "The twin did not return an answer.");
      }
    } catch (error) {
      stream?.cancel();
      console.error("chat request failed", error);
      failMessage(bubble, "Something went wrong. Please try again.");
      history.pop();
      setStatus(elements.status, "The question could not be answered.");
    } finally {
      busy = false;
      setBusy(elements, false);
      elements.input.focus();
    }
  };

  return {
    ask,
    get busy() {
      return busy;
    },
  };
}
