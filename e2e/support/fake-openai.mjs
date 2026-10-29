import http from "node:http";

const host = "127.0.0.1";
const port = Number(process.env.FAKE_OPENAI_PORT ?? "19099");

const defaultWords = ["I ", "build ", "Go ", "services ", "and ", "developer ", "tools."];
const slowFirst = "This is a slow answer. ";
const slowWords = [
  "It ",
  "keeps ",
  "arriving ",
  "in ",
  "small ",
  "pieces ",
  "so ",
  "the ",
  "visitor ",
  "can ",
  "stop ",
  "it.",
];

const usage = { prompt_tokens: 12, completion_tokens: 7, total_tokens: 19 };

function chunk(delta, finishReason = null) {
  return {
    choices: [{ index: 0, delta, finish_reason: finishReason }],
  };
}

function send(res, payload) {
  if (!res.destroyed && !res.writableEnded) {
    res.write(`data: ${JSON.stringify(payload)}\n\n`);
  }
}

function sendText(res, text) {
  send(res, chunk({ content: text }));
}

function finish(res) {
  send(res, { choices: [], usage });
  if (!res.destroyed && !res.writableEnded) {
    res.write("data: [DONE]\n\n");
    res.end();
  }
}

function streamWords(res, words, delayMs, first = "") {
  send(res, chunk({ role: "assistant" }));
  let index = 0;
  if (first) {
    sendText(res, first);
  }

  const timer = setInterval(() => {
    if (res.destroyed || res.writableEnded) {
      clearInterval(timer);
      return;
    }
    if (index >= words.length) {
      clearInterval(timer);
      finish(res);
      return;
    }
    sendText(res, words[index]);
    index += 1;
  }, delayMs);

  res.on("close", () => clearInterval(timer));
}

function lastUserMessage(body) {
  try {
    const parsed = JSON.parse(body);
    const messages = Array.isArray(parsed.messages) ? parsed.messages : [];
    for (let index = messages.length - 1; index >= 0; index -= 1) {
      const message = messages[index];
      if (message?.role === "user" && typeof message.content === "string") {
        return message.content;
      }
    }
  } catch {
    // Fall through to the default scenario.
  }
  return "";
}

const server = http.createServer((req, res) => {
  if (req.method === "GET" && req.url === "/healthz") {
    res.writeHead(200, { "Content-Type": "text/plain; charset=utf-8" });
    res.end("ok\n");
    return;
  }

  if (req.method !== "POST" || !req.url?.endsWith("/chat/completions")) {
    res.writeHead(404, { "Content-Type": "text/plain; charset=utf-8" });
    res.end("not found\n");
    return;
  }

  let body = "";
  req.on("data", (data) => {
    body += data;
  });
  req.on("end", () => {
    const question = lastUserMessage(body).trim().toLowerCase();

    if (question.startsWith("e2e:fail")) {
      res.writeHead(400, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          error: {
            message: "forced upstream failure",
            type: "invalid_request_error",
            param: "",
            code: "e2e_failure",
          },
        }),
      );
      return;
    }

    res.writeHead(200, {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
    });
    res.flushHeaders();

    if (question.startsWith("e2e:empty")) {
      send(res, chunk({ role: "assistant" }));
      finish(res);
      return;
    }

    if (question.startsWith("e2e:slow")) {
      streamWords(res, slowWords, 150, slowFirst);
      return;
    }

    streamWords(res, defaultWords, 40);
  });
});

server.listen(port, host, () => {
  console.log(`fake OpenAI server listening on http://${host}:${port}`);
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    server.close(() => process.exit(0));
    setTimeout(() => process.exit(0), 2000).unref();
  });
}
