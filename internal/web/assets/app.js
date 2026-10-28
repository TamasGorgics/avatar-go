(() => {
  "use strict";

  const MAX_LENGTH = 500;
  const STREAM_BASE_CPS = 70;
  const STREAM_LIVE_MAX_CPS = 600;
  const STREAM_DRAIN_SECONDS = 1.4;
  const STREAM_MAX_CPS = 3000;

  const log = document.getElementById("chat-log");
  const empty = document.getElementById("chat-empty");
  const form = document.getElementById("chat-form");
  const input = document.getElementById("chat-input");
  const submit = document.getElementById("chat-submit");
  const submitLabel = submit?.querySelector('[data-role="submit-label"]');
  const count = document.getElementById("chat-count");
  const status = document.getElementById("chat-status");
  const userTemplate = document.getElementById("user-message-template");
  const assistantTemplate = document.getElementById("assistant-message-template");

  if (!log || !form || !input || !submit || !userTemplate || !assistantTemplate) {
    return;
  }

  const history = [];
  let busy = false;

  const reduceMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;
  const graphemeSegmenter =
    typeof Intl !== "undefined" && typeof Intl.Segmenter === "function"
      ? new Intl.Segmenter(undefined, { granularity: "grapheme" })
      : null;

  const splitGraphemes = (text) => {
    if (graphemeSegmenter) {
      return Array.from(graphemeSegmenter.segment(text), (part) => part.segment);
    }
    return Array.from(text);
  };

  const updateCount = () => {
    if (count) {
      count.textContent = `${input.value.length} / ${MAX_LENGTH}`;
    }
  };

  const setStatus = (text) => {
    if (status) {
      status.textContent = text;
    }
  };

  const scrollToEnd = (behavior) => {
    if (behavior === "smooth" && !reduceMotion && typeof log.scrollTo === "function") {
      log.scrollTo({ top: log.scrollHeight, behavior: "smooth" });
      return;
    }
    log.scrollTop = log.scrollHeight;
  };

  // Network chunks arrive unevenly. Queue graphemes and reveal them at a steady
  // pace, speeding up as the backlog grows so the answer still catches up.
  const createTextStream = (node) => {
    const pending = [];
    let cursor = 0;
    let rendered = "";
    let frame = null;
    let lastFrame = 0;
    let carry = 0;
    let drainCps = STREAM_BASE_CPS;
    let ended = false;
    let resolveDrained;

    const drained = new Promise((resolve) => {
      resolveDrained = resolve;
    });

    const paint = () => {
      if (node) {
        node.textContent = rendered;
      }
      scrollToEnd("auto");
    };

    const resolveIfDrained = () => {
      if (ended && cursor >= pending.length) {
        resolveDrained();
      }
    };

    const step = (now) => {
      frame = null;
      const backlog = pending.length - cursor;
      if (backlog === 0) {
        resolveIfDrained();
        return;
      }

      const elapsed = Math.min(100, Math.max(0, now - lastFrame));
      lastFrame = now;

      const charsPerSecond = Math.min(
        ended
          ? drainCps
          : Math.max(STREAM_BASE_CPS, Math.min(STREAM_LIVE_MAX_CPS, backlog * 1.2)),
        STREAM_MAX_CPS,
      );

      carry += (charsPerSecond * elapsed) / 1000;
      const count = Math.min(backlog, Math.floor(carry));
      if (count > 0) {
        carry -= count;
        rendered += pending.slice(cursor, cursor + count).join("");
        cursor += count;
        paint();
      }

      if (cursor >= pending.length) {
        pending.length = 0;
        cursor = 0;
        carry = 0;
        resolveIfDrained();
        return;
      }

      frame = requestAnimationFrame(step);
    };

    const start = () => {
      if (frame !== null || cursor >= pending.length) {
        return;
      }
      lastFrame = performance.now();
      frame = requestAnimationFrame(step);
    };

    const push = (text) => {
      if (!text) {
        return;
      }
      if (reduceMotion) {
        rendered += text;
        paint();
        return;
      }
      for (const grapheme of splitGraphemes(text)) {
        pending.push(grapheme);
      }
      start();
    };

    const end = () => {
      ended = true;
      const backlog = pending.length - cursor;
      drainCps = Math.max(STREAM_BASE_CPS, backlog / STREAM_DRAIN_SECONDS);
      if (reduceMotion || backlog === 0) {
        resolveDrained();
        return;
      }
      start();
    };

    const cancel = () => {
      ended = true;
      pending.length = 0;
      cursor = 0;
      if (frame !== null) {
        cancelAnimationFrame(frame);
        frame = null;
      }
      resolveDrained();
    };

    return { push, end, cancel, drained };
  };

  const setBusy = (value) => {
    busy = value;
    input.disabled = value;
    submit.disabled = value;
    form.setAttribute("aria-busy", value ? "true" : "false");
    if (submitLabel) {
      submitLabel.textContent = value ? "Asking" : "Ask";
    }
  };

  const appendMessage = (template, text) => {
    const fragment = template.content.cloneNode(true);
    const message = fragment.firstElementChild;
    if (!message) {
      return null;
    }

    const textNode = message.querySelector('[data-role="text"]');
    if (textNode) {
      textNode.textContent = text;
    }

    log.append(fragment);
    if (empty) {
      empty.hidden = true;
    }
    scrollToEnd("smooth");
    return message;
  };

  const finishMessage = (message) => {
    if (!message) {
      return;
    }
    message.dataset.streaming = "false";
    message.querySelector('[data-role="typing"]')?.remove();
  };

  const failMessage = (message, text) => {
    if (!message) {
      return;
    }
    message.dataset.error = "true";
    message.dataset.streaming = "false";
    message.querySelector('[data-role="typing"]')?.remove();
    const textNode = message.querySelector('[data-role="text"]');
    if (textNode) {
      textNode.textContent = text;
    }
  };

  const ask = async (raw) => {
    if (busy) {
      return;
    }

    const question = raw.trim();
    if (!question) {
      return;
    }

    input.value = "";
    updateCount();
    setBusy(true);
    setStatus("Twin is answering.");

    history.push({ role: "user", content: question });
    appendMessage(userTemplate, question);
    const bubble = appendMessage(assistantTemplate, "");
    const textNode = bubble?.querySelector('[data-role="text"]') ?? null;
    let answer = "";
    let stream = null;

    try {
      const response = await fetch("/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ history }),
      });

      if (!response.ok) {
        const message = (await response.text()).trim() || "The twin is unavailable right now.";
        failMessage(bubble, message);
        history.pop();
        setStatus("The question could not be answered.");
        return;
      }

      if (!response.body) {
        throw new Error("response body is unavailable");
      }

      stream = createTextStream(textNode);
      const reader = response.body.getReader();
      const decoder = new TextDecoder();

      for (;;) {
        const { done, value } = await reader.read();
        if (done) {
          break;
        }
        const text = decoder.decode(value, { stream: true });
        answer += text;
        stream.push(text);
      }

      const tail = decoder.decode();
      answer += tail;
      stream.push(tail);
      stream.end();
      await stream.drained;

      if (textNode) {
        textNode.textContent = answer;
      }
      finishMessage(bubble);
      scrollToEnd("auto");

      if (answer.trim()) {
        history.push({ role: "assistant", content: answer });
        setStatus("Answer complete.");
      } else {
        if (textNode) {
          textNode.textContent = "The twin did not return an answer.";
        }
        setStatus("The twin did not return an answer.");
      }
    } catch (error) {
      stream?.cancel();
      console.error("chat request failed", error);
      failMessage(bubble, "Something went wrong. Please try again.");
      history.pop();
      setStatus("The question could not be answered.");
    } finally {
      setBusy(false);
      input.focus();
    }
  };

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    if (!busy) {
      void ask(input.value);
    }
  });

  input.addEventListener("input", updateCount);

  for (const button of document.querySelectorAll("[data-prompt]")) {
    button.addEventListener("click", () => {
      if (busy) {
        return;
      }
      input.value = button.dataset.prompt ?? "";
      updateCount();
      input.focus();
    });
  }

  updateCount();
})();
