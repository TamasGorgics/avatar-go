import { createChat } from "./chat";
import { getChatElements, updateCount } from "./dom";

const reduceMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;

const elements = getChatElements();
if (elements) {
  const chat = createChat(elements, reduceMotion);

  elements.form.addEventListener("submit", (event) => {
    event.preventDefault();
    void chat.ask(elements.input.value);
  });

  elements.input.addEventListener("input", () => {
    updateCount(elements.input, elements.count);
  });

  for (const button of document.querySelectorAll<HTMLElement>("[data-prompt]")) {
    button.addEventListener("click", () => {
      if (chat.busy) {
        return;
      }
      elements.input.value = button.dataset.prompt ?? "";
      updateCount(elements.input, elements.count);
      elements.input.focus();
    });
  }

  updateCount(elements.input, elements.count);
}
