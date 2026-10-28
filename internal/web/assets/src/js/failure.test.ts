import { describe, expect, it } from "vitest";

import {
  failureForEmptyAnswer,
  failureForNetwork,
  failureForStatus,
  failureForStopped,
  formatCountdown,
  MAX_RETRY_AFTER_MS,
  parseRetryAfter,
  rateLimitMessage,
} from "./failure";

const NOW = Date.parse("2026-10-28T12:00:00Z");

describe("parseRetryAfter", () => {
  it("parses delta-seconds", () => {
    expect(parseRetryAfter("40", NOW)).toBe(40_000);
  });

  it("parses zero", () => {
    expect(parseRetryAfter("0", NOW)).toBe(0);
  });

  it("parses an HTTP-date", () => {
    expect(parseRetryAfter("Wed, 28 Oct 2026 12:00:30 GMT", NOW)).toBe(30_000);
  });

  it("treats a past HTTP-date as ready now", () => {
    expect(parseRetryAfter("Wed, 28 Oct 2026 11:59:00 GMT", NOW)).toBe(0);
  });

  it("caps very large values", () => {
    expect(parseRetryAfter("999999", NOW)).toBe(MAX_RETRY_AFTER_MS);
    expect(parseRetryAfter("Thu, 01 Jan 2099 00:00:00 GMT", NOW)).toBe(
      MAX_RETRY_AFTER_MS,
    );
  });

  it("rejects missing, blank, and malformed values", () => {
    for (const value of [null, "", "   ", "soon", "-5", "+5", "1.5"]) {
      expect(parseRetryAfter(value, NOW)).toBeNull();
    }
  });
});

describe("formatCountdown", () => {
  it("formats seconds, minutes, and hours", () => {
    expect(formatCountdown(0)).toBe("0s");
    expect(formatCountdown(1)).toBe("1s");
    expect(formatCountdown(999)).toBe("1s");
    expect(formatCountdown(59_000)).toBe("59s");
    expect(formatCountdown(60_000)).toBe("1m");
    expect(formatCountdown(61_000)).toBe("1m 1s");
    expect(formatCountdown(3_600_000)).toBe("1h");
    expect(formatCountdown(3_661_000)).toBe("1h 1m");
    expect(formatCountdown(12 * 3_600_000)).toBe("12h");
  });
});

describe("rateLimitMessage", () => {
  it("includes a countdown while waiting", () => {
    expect(rateLimitMessage(40_000)).toBe(
      "Too many questions right now. Retry in 40s.",
    );
  });

  it("invites a retry once the wait is over", () => {
    expect(rateLimitMessage(0)).toBe(
      "Too many questions right now. You can retry now.",
    );
  });
});

describe("failureForStatus", () => {
  it("offers edit for rejected questions", () => {
    const failure = failureForStatus(400, null, "  That message can't be processed.  ");

    expect(failure.kind).toBe("rejected");
    expect(failure.message).toBe("That message can't be processed.");
    expect(failure.canEdit).toBe(true);
    expect(failure.canRetry).toBe(false);
  });

  it("classifies every 4xx except 429 as rejected", () => {
    for (const status of [400, 401, 403, 404, 413, 415, 418]) {
      const failure = failureForStatus(status, null);
      expect(failure.kind).toBe("rejected");
      expect(failure.canEdit).toBe(true);
    }
  });

  it("offers a countdown for 429 with Retry-After", () => {
    const failure = failureForStatus(429, 61_000);

    expect(failure.kind).toBe("rate_limited");
    expect(failure.message).toContain("Retry in 1m 1s.");
    expect(failure.canRetry).toBe(true);
    expect(failure.canEdit).toBe(false);
    expect(failure.retryAfterMs).toBe(61_000);
  });

  it("treats 429 without Retry-After as busy", () => {
    const failure = failureForStatus(429, null, "Another answer is still in progress.");

    expect(failure.kind).toBe("busy");
    expect(failure.message).toBe("Another answer is still in progress.");
    expect(failure.canRetry).toBe(true);
    expect(failure.retryAfterMs).toBeNull();
  });

  it("keeps a non-concurrency 429 message as rate-limited without a countdown", () => {
    const failure = failureForStatus(
      429,
      null,
      "Too many questions right now. Please try again later.",
    );

    expect(failure.kind).toBe("rate_limited");
    expect(failure.message).toBe("Too many questions right now. Please try again later.");
    expect(failure.canRetry).toBe(true);
    expect(failure.retryAfterMs).toBeNull();
  });

  it("uses the fallback busy message", () => {
    expect(failureForStatus(429, null).message).toBe(
      "Another answer is still in progress. Please wait for it to finish.",
    );
  });

  it("offers retry for upstream failures", () => {
    for (const status of [500, 502, 503, 504]) {
      const failure = failureForStatus(status, null, "The twin is unavailable right now.");
      expect(failure.kind).toBe("unavailable");
      expect(failure.canRetry).toBe(true);
      expect(failure.canEdit).toBe(false);
    }
  });

  it("prefers the server message for upstream failures", () => {
    expect(failureForStatus(502, null, "Busy upstream.").message).toBe("Busy upstream.");
  });
});

describe("other failures", () => {
  it("classifies network and timeout errors as retryable", () => {
    for (const kind of ["network", "timeout"] as const) {
      const failure = failureForNetwork(kind);
      expect(failure.kind).toBe(kind);
      expect(failure.canRetry).toBe(true);
      expect(failure.canEdit).toBe(false);
    }
  });

  it("marks an empty answer as retryable", () => {
    const failure = failureForEmptyAnswer();
    expect(failure.kind).toBe("unavailable");
    expect(failure.canRetry).toBe(true);
  });

  it("marks a stopped answer as retryable", () => {
    const failure = failureForStopped();
    expect(failure.kind).toBe("stopped");
    expect(failure.message).toBe("Answer stopped.");
    expect(failure.canRetry).toBe(true);
  });
});
