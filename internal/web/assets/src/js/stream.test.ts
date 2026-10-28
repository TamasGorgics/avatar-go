import { describe, expect, it } from "vitest";

import { createTextStream, splitGraphemes } from "./stream";

interface Scheduler {
  now: number;
  request(callback: FrameRequestCallback): number;
  cancel(handle: number): void;
  advance(ms: number): void;
  advanceFrames(count: number, ms?: number): void;
}

function createScheduler(): Scheduler {
  let nextHandle = 1;
  const queue = new Map<number, FrameRequestCallback>();

  const scheduler: Scheduler = {
    now: 0,
    request(callback) {
      const handle = nextHandle;
      nextHandle += 1;
      queue.set(handle, callback);
      return handle;
    },
    cancel(handle) {
      queue.delete(handle);
    },
    advance(ms) {
      scheduler.now += ms;
      const callbacks = [...queue.values()];
      queue.clear();
      for (const callback of callbacks) {
        callback(scheduler.now);
      }
    },
    advanceFrames(count, ms = 1000 / 60) {
      for (let index = 0; index < count; index += 1) {
        scheduler.advance(ms);
      }
    },
  };

  return scheduler;
}

function createHarness(options: { reducedMotion?: boolean } = {}) {
  const scheduler = createScheduler();
  const writes: string[] = [];
  const paints: string[] = [];
  let framesRequested = 0;

  const stream = createTextStream({
    write(text) {
      writes.push(text);
    },
    onPaint() {
      const last = writes.at(-1);
      if (last !== undefined) {
        paints.push(last);
      }
    },
    reducedMotion: options.reducedMotion ?? false,
    requestFrame(callback) {
      framesRequested += 1;
      return scheduler.request(callback);
    },
    cancelFrame(handle) {
      scheduler.cancel(handle);
    },
    now() {
      return scheduler.now;
    },
  });

  return {
    stream,
    scheduler,
    writes,
    paints,
    framesRequested: () => framesRequested,
  };
}

describe("splitGraphemes", () => {
  it("keeps emoji sequences and combining marks intact", () => {
    expect(splitGraphemes("a👨‍👩‍👧‍👦e\u0301")).toEqual(["a", "👨‍👩‍👧‍👦", "e\u0301"]);
  });
});

describe("createTextStream", () => {
  it("reveals text gradually instead of in one burst", () => {
    const harness = createHarness();
    const text = "Nordic ".repeat(10).trim();

    harness.stream.push(text);
    expect(harness.writes).toHaveLength(0);

    harness.scheduler.advance(100);
    expect(harness.writes).toHaveLength(1);
    const first = harness.writes[0] ?? "";
    expect(first.length).toBeGreaterThan(0);
    expect(first.length).toBeLessThan(text.length);
    expect(text.startsWith(first)).toBe(true);
    expect(harness.paints).toHaveLength(1);

    harness.scheduler.advanceFrames(60);
    expect(harness.writes.at(-1)).toBe(text);
    expect(harness.paints.at(-1)).toBe(text);
  });

  it("speeds up when the backlog grows", () => {
    const harness = createHarness();

    harness.stream.push("x".repeat(600));
    harness.scheduler.advance(100);

    expect(harness.writes[0]?.length).toBe(60);
    harness.stream.cancel();
  });

  it("drains the backlog before resolving after end", async () => {
    const harness = createHarness();
    const text = "y".repeat(200);
    let resolved = false;

    harness.stream.push(text);
    harness.scheduler.advance(100);
    const visibleBeforeEnd = harness.writes.at(-1)?.length ?? 0;
    expect(visibleBeforeEnd).toBeGreaterThan(0);
    expect(visibleBeforeEnd).toBeLessThan(text.length);

    void harness.stream.drained.then(() => {
      resolved = true;
    });
    harness.stream.end();
    await Promise.resolve();
    expect(resolved).toBe(false);

    harness.scheduler.advanceFrames(120);
    await harness.stream.drained;

    expect(resolved).toBe(true);
    expect(harness.writes.at(-1)).toBe(text);
  });

  it("renders immediately when reduced motion is on", async () => {
    const harness = createHarness({ reducedMotion: true });

    harness.stream.push("hej");
    expect(harness.writes).toEqual(["hej"]);

    harness.stream.end();
    await harness.stream.drained;

    expect(harness.framesRequested()).toBe(0);
    expect(harness.writes).toEqual(["hej"]);
  });

  it("stops painting and resolves after cancel", async () => {
    const harness = createHarness();

    harness.stream.push("z".repeat(100));
    harness.scheduler.advance(100);
    const writesBeforeCancel = harness.writes.length;

    harness.stream.cancel();
    harness.scheduler.advanceFrames(10);
    await harness.stream.drained;

    expect(harness.writes).toHaveLength(writesBeforeCancel);
  });

  it("resolves immediately for empty input", async () => {
    const harness = createHarness();

    harness.stream.push("");
    expect(harness.writes).toHaveLength(0);

    harness.stream.end();
    await harness.stream.drained;

    expect(harness.writes).toHaveLength(0);
    expect(harness.framesRequested()).toBe(0);
  });
});
