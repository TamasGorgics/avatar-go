(() => {
  "use strict";

  const MAX_LENGTH = 500;

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
    const reduceMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
    if (behavior === "smooth" && !reduceMotion && typeof log.scrollTo === "function") {
      log.scrollTo({ top: log.scrollHeight, behavior: "smooth" });
      return;
    }
    log.scrollTop = log.scrollHeight;
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

      const reader = response.body.getReader();
      const decoder = new TextDecoder();

      for (;;) {
        const { done, value } = await reader.read();
        if (done) {
          break;
        }
        answer += decoder.decode(value, { stream: true });
        if (textNode) {
          textNode.textContent = answer;
        }
        scrollToEnd("auto");
      }

      answer += decoder.decode();
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
