# Roadmap

A living backlog of product and technical improvements for the digital twin. This is a
prioritized menu, not a commitment. Re-rank freely as the site gets real traffic and the
profile content changes.

Snapshot: the initial security pass landed in `432a578` (rate and concurrency limits,
request hardening, prompt-injection screening, CSP/security headers, and server timeouts).

## Priority at a glance

| Priority | Theme | Focus |
| --- | --- | --- |
| **P0** | Make it real and measurable | Profile content and static fallback, failed-question UX, spend accounting, observability, browser E2E and prompt evals |
| **P1** | Trust, polish, and resilience | Citations and CTAs, feedback, accessibility, model quality and caching, proxy/shutdown correctness, CSP hardening |
| **P2** | Scale and explore | RAG, structured streaming, runtime profile editing, multilingual support, shared limiter, deployment pipeline |

---

## P0 — Do these first

### 1. Fill and structure `about.md`

The twin's answers depend on `about.md`, which is still the literal text `TODO`. This is
the single biggest gap: without it the twin has nothing to answer from.

- [ ] Write the profile: summary, timeline, projects, skills, working style, FAQ, contact
      links, and a “last updated” date.
- [ ] Use one source of truth for both the system prompt and a static profile/FAQ section.
- [ ] Give visitors value without JavaScript or an LLM: static bio, selected projects, and
      common questions with answers.
- [ ] Help search engines index the content; a chat-only page gives crawlers very little.
- [ ] Use the static section as the model-unavailable fallback.
- [ ] Validate the profile at startup and warn or fail loudly if it is empty or still
      `TODO`.

### 2. Fix the failed-question UX

Landed: failed questions are recoverable, failures are classified, Stop/Copy/Regenerate/
Clear/Edit last exist, and completed/stopped turns restore from `sessionStorage`. Browser
level coverage is tracked under the E2E item below.

- [x] Restore the question to the input, or add a Retry action that resends it.
- [x] Distinguish `429`, `400`, `502`, and network failures in the UI.
- [x] Show a retry countdown when the server returns `429` (see `Retry-After` below).
- [x] Add a Stop button backed by `AbortController` so a slow answer can be cancelled.
- [x] Add Clear conversation, Copy answer, Regenerate, and Edit last question.
- [x] Optionally restore the last conversation from `sessionStorage`.

### 3. Separate anti-abuse limits from model-spend limits

Landed: per-IP attempt limits remain early, while global model-call limits, token reservations,
and concurrency are admitted only after validation. `GLOBAL_MINUTE` counts model calls.

- [x] Keep per-IP attempt limits early, before parsing.
- [x] Count global daily calls and upstream concurrency only after validation and just before
      calling the model.
- [x] Count `GLOBAL_MINUTE` as model calls, not raw requests.
- [x] Add a per-IP daily token budget so one visitor cannot consume the whole allowance.
- [x] Add tests proving malformed requests cannot exhaust the model-spend budget.

### 4. Add observability and health endpoints

Logs are good, but there is no easy way to answer: how many answers failed, what is
time-to-first-token, which limit is firing, and how many tokens are being spent?

- [x] Add `/healthz` (liveness) and `/readyz` (readiness); readiness does not probe the model
      and turns off during graceful shutdown.
- [x] Add server-generated request IDs to response headers and request-scoped logs.
- [x] Add metrics for requests, `429`s by reason, upstream latency and errors, stream
      duration, and time-to-first-token.
- [x] Add token-usage metrics and reporting, distinguishing upstream usage from estimates.
- [ ] Consider OpenTelemetry if traces become useful.
- [x] Document HTTP probes for the distroless runtime; no in-container probe binary is
      needed.

### 5. Add browser E2E tests and a prompt eval suite

Unit tests are strong, but there is no real-browser coverage for streaming, error states,
theme switching, or mobile layout. Prompt changes are also untested behavior changes.

- [ ] Add Playwright tests for a successful stream, `429`, `400`, upstream failure, Stop,
      theme switching, and a mobile viewport.
- [ ] Keep a prompt eval corpus of profile questions with expected facts and refusals.
- [ ] Include injection attempts and, equally important, benign questions that must not be
      rejected by the screening rules.
- [ ] Run evals manually or nightly rather than on every CI run if cost is a concern.
- [ ] Track false positives from the injection screen and tune the rules.

---

## P1 — Product and UX

- [ ] **Citations and source links.** Let answers point to the relevant profile section,
      project, or page. This builds trust and probably pushes toward structured responses.
- [ ] **Stronger calls to action.** Add Download CV, Email me, and project links alongside
      LinkedIn. When the twin does not know something, offer the contact path directly.
- [ ] **Feedback and unknown-question insights.** Add thumbs up/down and learn which topics
      are missing from the profile. Avoid storing raw questions by default; aggregate or
      make it opt-in.
- [ ] **Follow-up suggestions.** The starter chips are good. After an answer, offer two or
      three contextual follow-ups, ideally derived from the profile rather than invented by
      the model.
- [ ] **Shareable questions.** Support `/?q=...` to prefill a question, useful for linking
      from a CV or LinkedIn post.
- [ ] **Accessibility follow-through.** Streamed text is not announced as it arrives. Add a
      throttled live region or focus management on completion, and test contrast, focus
      order, and keyboard flow with axe.
- [ ] **Multilingual answers.** Detect the visitor’s language and answer in it while
      keeping the profile in one source language.
- [ ] **SEO and sharing.** Add OpenGraph metadata, Person schema, `robots.txt`,
      `sitemap.xml`, and the static profile content.
- [ ] **Optional safe Markdown.** Lists and links would read better, but never render raw
      HTML. Use a small sanitizer or a limited Markdown subset.
- [ ] **Privacy-friendly analytics.** Measure page views, questions asked, LinkedIn clicks,
      and conversion paths without collecting conversation content.

## P1 — Model quality and cost

- [ ] **Lower the temperature.** For factual profile answers, something like `0.2–0.4`
      reduces drift; the default is currently unset.
- [ ] **Cache common answers.** Recruiters ask many of the same questions. Add an
      exact-match normalized-question cache with a TTL, versioned by profile content.
- [ ] **Precompute a top-10 FAQ.** Serve common answers instantly and as a fallback when
      the model is unavailable.
- [ ] **Fix assistant-history truncation.** Every history message is capped at 500 bytes,
      including assistant messages, but the model can return roughly 1–2 KB at 400 tokens.
      Use separate limits, e.g. 500 bytes for user messages and 2 KB for assistant
      messages, with an overall history budget.
- [ ] **Prompt versioning and canaries.** Version the system prompt, record which version
      produced an answer, and keep a canary string to detect prompt leakage. A canary does
      not prevent extraction, but it tells you when it happens.
- [ ] **Audit the profile for sensitive data.** The model can only repeat what is in the
      profile, so the profile is the real privacy boundary.
- [ ] **Compare small models.** Re-test the model choice periodically against the eval
      corpus for quality, latency, and cost.
- [ ] **Optional moderation.** If adversarial traffic grows, run inputs and outputs through
      a moderation endpoint. It does not stop prompt injection, but it helps with abusive
      content.
- [ ] **RAG later, not now.** Once the source material grows beyond a short profile, add
      retrieval with citations. With a short profile, a single prompt is simpler and
      better.

## P1 — Reliability and operations

- [ ] **Trusted proxy CIDRs instead of a boolean.** `TRUST_PROXY=1` plus “rightmost XFF”
      assumes exactly one proxy. Behind Cloudflare and Caddy, the rightmost entry may be
      Cloudflare’s edge, collapsing every visitor into one rate-limit bucket. Walk the XFF
      chain from right to left and stop at the first untrusted address; support
      `CF-Connecting-IP` only when the peer is trusted.
- [ ] **Circuit breaker and fallback.** If the upstream hangs or repeatedly returns 5xx,
      stop hammering it. Show a friendly fallback with the profile and contact link.
- [x] **Return `Retry-After` on `429`.** Done in P0.2: time-window limits send whole
      seconds; concurrency limits use a distinct busy message with no countdown.
- [ ] **Align shutdown with stream lifetime.** The server drains for 10 seconds, but a chat
      stream can run for 60. Deploys will truncate answers. Either extend the drain or
      explicitly cancel streams and tell the client the answer was interrupted.
- [ ] **Panic recovery middleware.** `net/http` recovers per connection, but a custom layer
      can return a clean 500 and log with request context.
- [ ] **Config validation.** Invalid values silently fall back to defaults, hiding typos.
      Log a warning with the variable name, or fail fast for malformed explicit values.
- [ ] **Shared limiter only if you scale.** In-memory limits are fine for one replica. For
      multiple replicas, move counters to Redis or accept inconsistent limits.
- [ ] **Asset caching and compression.** Fingerprint `app.css` and `app.js`, serve them with
      long-lived immutable caching, and enable gzip/brotli at the proxy.
- [ ] **Load-test the limits.** Tune `GLOBAL_CONCURRENT=4` and the minute caps against real
      latency; four slow upstream requests can otherwise block everyone for up to a minute.

## P1 — Security and privacy

- [ ] **Replace `'unsafe-inline'` in the CSP with a hash.** The theme bootstrap is static,
      so its SHA-256 hash can be added to `script-src` without a nonce refactor.
- [ ] **Make the origin check scheme-aware.** It currently compares only hosts, so
      `http://example.com` and `https://example.com` are treated as the same origin. Use
      the forwarded proto when behind a trusted proxy.
- [ ] **Add HSTS and cross-origin headers at the proxy.** `Strict-Transport-Security`,
      `Cross-Origin-Opener-Policy`, and `Cross-Origin-Resource-Policy` are cheap once TLS
      is terminated correctly.
- [ ] **Add a privacy page.** State that conversations are not stored server-side, what is
      logged, and how long operational logs are retained.
- [ ] **Add Turnstile or Cloudflare when traffic justifies it.** Do not add CAPTCHA
      prematurely; it adds friction and complexity.
- [ ] **Supply-chain checks.** Add `govulncheck`, `staticcheck`, CodeQL, Trivy for the
      image, Renovate/Dependabot, pinned GitHub Actions, and an SBOM.
- [ ] **Keep message content out of logs.** This is already true; keep it that way as new
      analytics and debugging are added.

---

## P2 — Later and exploratory

- [ ] **RAG over a larger knowledge base.** Blog posts, project notes, GitHub READMEs, and
      a longer CV could be retrieved with citations. Revisit once the profile outgrows a
      single prompt.
- [ ] **Structured streaming instead of plain text.** SSE or NDJSON would allow citations,
      usage data, explicit error events, and an end-of-stream marker. Plain text remains a
      deliberate simplicity win for now.
- [ ] **Runtime profile editing or a small CMS.** Today `about.md` is embedded at compile
      time, so content changes require a rebuild. A read-only runtime file or admin editor
      would speed iteration; keep the trust boundary in mind.
- [ ] **Deployment pipeline.** Publish versioned images to GHCR, add provenance and an
      SBOM, and automate deployment after CI.
- [ ] **Embeddable widget.** A small chat widget for a portfolio or LinkedIn page could
      reuse the same API, but it needs its own origin and abuse story.
- [ ] **Multi-replica operation.** Only worth it if traffic demands it; it requires a shared
      limiter, shared cache, and sticky or stateless conversation handling.
- [ ] **Voice input.** Nice accessibility feature, but a large surface area relative to the
      value for a text-first portfolio.

## What not to rush

- **Server-side conversation storage.** It adds privacy, retention, and schema work for
  little user value right now.
- **RAG before the profile exists.** Fix the content first.
- **SSE instead of plain text.** Worth it when citations, usage, or error events are
  needed; plain text is a deliberate simplicity win today.
- **CAPTCHA/WAF before public traffic.** The current limits are a reasonable first line.
- **Multi-replica infrastructure.** One well-observed instance is easier to operate than a
  distributed limiter for a personal site.

## Suggested sequence

1. Write the profile, add the static profile/FAQ fallback, and validate it at startup.
2. Fix failed-question UX and separate spend accounting from cheap-request limits.
3. Add observability, health endpoints, browser E2E tests, and prompt evals.
4. Fix trusted-proxy handling, shutdown behavior, and CSP hashing.
5. Add citations, CTAs, feedback, caching, accessibility, and the remaining product polish.
6. Revisit RAG, structured streaming, and runtime profile editing only when the content and
   traffic justify them.
