# PADS — Predictive Adaptive Download Scheduler

## Enterprise-Grade CLI Implementation Guide (Go)

**Document status:** Authoritative implementation specification  
**Quality target:** Production-ready, portfolio-grade, and review-safe  
**Primary goal:** Deliver a maintainable, testable, and realistic download manager with a clear architecture and disciplined engineering process.

---

## 1. Purpose

PADS is a Go-based CLI download manager that uses segmented downloading, adaptive connection scaling, server probing, resume support, and queue orchestration to maximize throughput while remaining reliable under real-world network conditions.

This document is not only a feature list. It is the engineering contract for how the project must be built, tested, reviewed, committed, and released.

---

## 2. Product goals

PADS must:

- Download files reliably over HTTP/1.1 and HTTP/2 where supported.
- Resume interrupted downloads from persisted state.
- Use segmented downloads only when the server supports byte ranges.
- Adapt connection count based on observed network and server behavior.
- Preserve correctness under cancellation, partial failure, and restart.
- Provide a clean CLI experience suitable for portfolio review.
- Maintain a disciplined commit history with only meaningful, professional commits.

---

## 4. Engineering principles

1. **Correctness before performance.**  
   A fast incorrect downloader is worse than a slower reliable one.

2. **Small, explicit abstractions.**  
   Each package should have one clear purpose.

3. **Fail loudly, fail safely.**  
   Errors must be surfaced with context and never ignored.

4. **State must be recoverable.**  
   Partial progress must survive interruption whenever possible.

5. **Concurrency must be owned.**  
   Every goroutine must have a clear lifecycle, cancellation path, and shutdown rule.

6. **Dependencies must be justified.**  
   Prefer the standard library unless a dependency adds clear value.

7. **History must be professional.**  
   Only meaningful commits belong in the repository. No emoji spam, no filler commits, no AI-style noise.

---

## 5. Coding style guide

### 5.1 Go style

Follow idiomatic Go first. Write code that other Go developers can read without explanation.

### 5.2 Naming

Use descriptive names that communicate intent.

Good:

```go
downloadState
segmentManager
calculateEffectiveSpeed
saveResumeState
```

Avoid:

```go
ds
mgr
calc
tmp1
run
```

Single-letter variables are acceptable only for short-lived loop indexes or conventional context names such as `i`, `j`, `ctx`, `r`, and `w`.

### 5.3 Function size and scope

- Prefer functions between 10 and 40 lines.
- Refactor any function that grows beyond 80 lines unless there is a strong reason not to.
- Each function should have one responsibility.
- Avoid deeply nested logic; use guard clauses early.

### 5.4 Error handling

- Never ignore errors.
- Always wrap errors with context at package boundaries.
- Do not return raw sentinel errors from deep layers unless the caller explicitly needs them.

Example:

```go
if err != nil {
    return fmt.Errorf("save queue state: %w", err)
}
```

### 5.5 Comments

Comments should explain **why**, not repeat **what** the code already says.

Bad:

```go
// increment i by one
i++
```

Good:

```go
// Use a smaller initial segment count to reduce startup latency on slow links.
```

### 5.6 Formatting

- Use `gofmt` and `goimports`.
- No manual alignment that fights the formatter.
- Keep line length reasonable.
- Prefer explicit structure over clever one-liners.

---

## 6. Repository structure

```text
pads/
├── main.go
├── go.mod
├── go.sum
├── README.md
├── LICENSE
├── Makefile
│
├── cmd/
│   ├── root.go
│   ├── get.go
│   ├── queue.go
│   ├── resume.go
│   ├── pause.go
│   ├── status.go
│   ├── schedule.go
│   └── config.go
│
├── internal/
│   ├── app/
│   ├── scheduler/
│   ├── downloader/
│   ├── probe/
│   ├── state/
│   ├── queue/
│   ├── writer/
│   ├── ui/
│   ├── config/
│   └── util/
│
├── testutil/
├── docs/
│   └── adr/
└── scripts/
```

### Boundary rule

- CLI orchestration belongs in `cmd/`.
- Core logic belongs in `internal/`.
- Shared test helpers belong in `testutil/`.
- Architectural decisions belong in `docs/adr/`.

No package should depend on CLI code for business logic.

---

## 7. Dependency policy

Only use dependencies that are well-maintained and clearly justified.

Preferred external dependencies:

- `github.com/spf13/cobra` for CLI parsing
- `github.com/vbauerster/mpb/v8` for progress bars
- `github.com/google/uuid` for unique identifiers when needed
- `github.com/fatih/color` only if terminal color adds clear value

### Dependency rules

- Prefer the Go standard library for HTTP, JSON, filesystem, time, and concurrency.
- Do not add a dependency if the standard library can already solve the problem cleanly.
- Avoid large framework dependencies.
- Avoid database dependencies unless the project truly needs them.
- Re-evaluate every dependency before adding it.

---

## 8. Commit, branch, and PR standards

### 8.1 Branch strategy

Use a small, readable branch strategy:

- `main`
- `develop` optional
- `feature/*`
- `fix/*`
- `refactor/*`
- `test/*`
- `docs/*`

### 8.2 Commit standards

Commit messages must be concise, professional, and meaningful.

Allowed prefixes:

```text
feat:
fix:
refactor:
test:
docs:
build:
ci:
perf:
security:
chore:
```

Examples:

```text
feat: add resume state loader
fix: handle HTTP 416 range responses
refactor: simplify segment ETA calculation
test: add probe server coverage
docs: update architecture decisions
```

Rules:

- No emojis.
- No AI signatures.
- No “final fix”, “working version”, or other vague commits.
- No co-authored messages unless explicitly required by a real collaboration process.
- Keep only critical commits in the final history. Experimental work should be squashed or cleaned up before publishing.

### 8.3 Pull request standards

Every PR should include:

- Problem statement
- Proposed solution
- Testing evidence
- Risk assessment
- Rollback notes if relevant

---

## 9. Architecture overview

PADS is organized around a staged download lifecycle:

1. **Probe** the server.
2. **Plan** segments and connections.
3. **Download** with adaptive scheduling.
4. **Persist** state continuously.
5. **Merge** final output safely.
6. **Resume** on interruption.
7. **Report** progress clearly.

The architecture must remain modular so each stage can be tested independently.

---

## 10. Core data contracts

### 10.1 Segment

A segment represents a byte range assigned to one worker at a time.

Required fields:

- segment ID
- byte start/end
- bytes downloaded so far
- status
- assigned connection
- temp file path
- progress timestamps
- speed and ETA estimates

Rules:

- Byte ranges are inclusive.
- Segment state must be serializable.
- Segment updates must be concurrency-safe.

### 10.2 Connection

A connection represents one active HTTP worker.

Required fields:

- connection ID
- lifecycle state
- current segment
- warm-up tracking
- current speed
- peak speed
- open timestamp

Rules:

- Connections must be cancellable.
- Idle, active, warming, draining, and closed states must be explicit.
- A connection must never be reused after a fatal protocol error without reinitialization.

### 10.3 Server profile

The probe phase must produce a server profile that captures:

- range support
- content length
- content type
- HTTP protocol version
- latency estimate
- rough throttling behavior
- recommended connection count

This profile should guide scheduling decisions but must not be treated as perfect truth.

### 10.4 Download state

Persisted resume state must include:

- download ID
- URL
- output path
- total size
- all segments
- created/updated timestamps
- completion flag
- version number

State files must be versioned to allow future migration.

---

## 11. CLI specification

### Commands

```text
pads get <url>
pads queue add <url>
pads queue list
pads queue start
pads queue clear
pads queue remove <id>
pads resume <id>
pads pause <id>
pads status
pads schedule <url> --at "HH:MM"
pads config set <key> <value>
pads config show
pads config reset
```

### Global rules

- CLI output should be concise and readable.
- Errors must be human-friendly.
- Progress reporting should never corrupt terminal output.
- Command flags must be validated before execution.

---

## 12. Server probe requirements

The server probe exists to reduce wasted work and to improve scheduling quality.

### Probe must determine:

- whether byte-range requests are supported
- approximate content length
- approximate protocol behavior
- average request latency
- whether more connections appear beneficial

### Probe rules

- Probe time must be bounded.
- Probe must not block indefinitely.
- A failed probe must not crash the application.
- If range support is absent, the downloader must fall back to single-connection mode.
- Heuristics should be conservative; never over-assert server capabilities.

### Important caution

Any “optimal connection count” is a recommendation, not a promise. The scheduler must continue to adapt after the probe completes.

---

## 13. Segment manager requirements

### Initial segmentation

- Split content into evenly sized ranges.
- Enforce a minimum segment size to avoid excessive overhead.
- Reduce segment count automatically when the file is too small.
- The last segment should absorb remainder bytes.

### Dynamic stealing

Segment stealing is allowed only when:

- the remaining bytes are large enough to justify overhead
- the target segment is clearly slower than the average
- the scheduler has an available connection or replacement worker

### Safety rules

- Keep overlap minimal and well-documented.
- Do not steal from tiny segments.
- Never lose bytes at boundary transitions.
- Any boundary recalculation must preserve exact coverage of the file.

---

## 14. Connection pool requirements

### Lifecycle

```text
Open → Warming → Active → Draining → Closed
```

### Rules

- Warming connections must be treated cautiously.
- New workers should start with smaller segment assignments when appropriate.
- Closed connections must not continue writing.
- Every connection must be visible to the scheduler and debuggable in logs or status output.

### Scaling

Connection scaling must consider:

- observed throughput
- velocity trend
- server throttle behavior
- remaining file size
- current phase of the download

Scaling must not oscillate excessively. Hysteresis is required.

---

## 15. Scheduler requirements

The scheduler is the brain of the system.

### Phases

1. **Probe**
2. **Ramp**
3. **Cruise**
4. **Tail**
5. **Done**

### Scheduling rules

- Make decisions on a fixed tick interval.
- Never rely on a single measurement if a trend is available.
- Prefer measured throughput over theoretical assumptions.
- Adapt cautiously when speed is falling.
- Stop aggressive scaling near the end of the download.
- Prevent segment churn caused by overly sensitive steal logic.

### Tail handling

The final portion of the download should favor stability over aggressive parallelism. The goal is not to maximize connection count; it is to finish cleanly and avoid overhead.

---

## 16. Bandwidth and ETA logic

Bandwidth monitoring must support:

- current speed
- rolling average
- trend / velocity
- ETA estimate per segment
- ETA estimate for the whole download

### Rules

- Use rolling windows, not single samples.
- Avoid noisy overreaction.
- Keep the math simple enough to test.
- Velocity is a scheduling hint, not a source of truth.

---

## 17. File writer requirements

The writer is responsible for safely producing the final file.

### Rules

- Write segment data to temp files first.
- Merge only when all required segments are complete.
- Use atomic rename for final output.
- Do not destroy resume data before the final file is verified.
- Keep merge logic deterministic and testable.

### Temp file policy

- Temp files must be isolated per download ID.
- Temp files must use a predictable structure.
- Cleanup should occur after successful completion.
- On failure, preserve resumable state where possible.

---

## 18. Resume and state requirements

### State behavior

- Persist state atomically.
- Save progress often enough to survive interruption.
- Load state safely and validate its contents.
- Ignore or quarantine obviously corrupted state rather than crashing.

### Resume rules

- Resume must continue from the downloaded offset.
- Completed segments must not be redownloaded.
- Resume should preserve integrity checks where practical.
- Any mismatch between saved state and server reality must be handled explicitly.

### Versioning

State must include a version field so future migrations are possible.

---

## 19. Queue requirements

The queue is a persistence-backed list of pending downloads.

### Queue rules

- Support add, list, start, clear, remove.
- Store queue state in a stable format.
- Keep queue operations idempotent where practical.
- Mark completed and failed jobs clearly.
- Retain enough metadata to debug failures.

### Queue execution

Queue processing must:

- load queue state
- process pending entries one by one or according to the selected policy
- preserve per-download options
- continue cleanly after failure when possible

---

## 20. Error handling policy

### Retry policy

- Use bounded retries.
- Backoff should increase between attempts.
- Retries should be segment-specific when possible.
- A full download should not be retried endlessly.

### Error categories

Handle these explicitly:

- network timeout
- connection reset
- HTTP 416
- HTTP 429
- HTTP 503
- disk full
- permission denied
- invalid URL
- corrupted state file
- unsupported range behavior

### Required behavior

- Add context to all surfaced errors.
- Distinguish between recoverable and terminal failures.
- Preserve resumable state on interruption or failure when possible.
- Do not silently continue after a serious consistency issue.

---

## 21. Security requirements

- Validate all input from the CLI.
- Validate and normalize output paths.
- Prevent path traversal.
- Enforce safe file creation and atomic renames.
- Reject malformed URLs early.
- Treat server headers as untrusted input.
- Never assume `Content-Length` is truthful without validation logic.
- Limit redirects to a reasonable number.
- Avoid unsafe filesystem assumptions across platforms.

---

## 22. Observability requirements

PADS should be understandable while running.

### Required runtime signals

- current phase
- active download count
- connection count
- current speed
- average speed
- ETA
- retry count
- failed segment count
- completed segment count
- queue depth

### Logging rules

- Use structured, human-readable logs.
- Include download ID and segment ID where relevant.
- Log state transitions.
- Log retries and recoveries.
- Avoid noisy logs unless debug mode is enabled.

---

## 23. Testing requirements

### Unit tests

Required coverage areas:

- segment math
- ETA calculation
- connection state transitions
- bandwidth trend logic
- probe interpretation
- queue persistence
- state save/load
- merge behavior

### Integration tests

Use a local HTTP test server to simulate:

- range support
- range rejection
- throttling
- intermittent failures
- 503/429 responses
- resume after cancellation
- corrupted state recovery

### Reliability tests

Also verify:

- graceful shutdown
- repeated pause/resume
- large file handling
- disk-space failure handling
- concurrent segment updates
- race detection with `go test ./... -race`

### Coverage target

- Core packages: 95% or better
- Integration-critical logic: high coverage and explicit scenario tests
- A lower threshold for CLI wrapper code is acceptable if the core logic is thoroughly tested

---

## 24. Benchmarking requirements

Benchmarks should exist for:

- segment creation
- ETA calculation
- bandwidth velocity
- state persistence
- file merging
- scheduler tick overhead

Benchmarks are used to validate changes, not to justify premature optimization.

---

## 25. Release and versioning policy

### Versioning

Use semantic versioning:

- major for breaking changes
- minor for new functionality
- patch for bug fixes

### Release readiness

A release is acceptable only when:

- tests pass
- race checks pass
- resume logic works
- queue logic works
- error handling is stable
- documentation is updated
- no critical TODOs remain in production code

### Changelog policy

Each release must include a clear change log entry summarizing features, fixes, and breaking changes.

---

## 26. Architecture Decision Records

Maintain ADRs in `docs/adr/`.

Required ADR topics:

- dependency selection
- state format
- connection scaling model
- segment stealing strategy
- queue persistence model
- retry strategy
- output merge strategy

ADR format:

- context
- decision
- alternatives considered
- consequences

---

## 27. Definition of done

A feature is done only when:

- implementation is complete
- tests are added
- documentation is updated
- edge cases are considered
- logs are useful
- errors are handled
- review notes are addressed

A feature is not done if it only works in the happy path.

---

## 28. Implementation roadmap

### Phase A — foundation

- initialize module
- set up CLI skeleton
- implement config loading
- add HTTP helpers
- establish repository structure

### Phase B — baseline downloader

- probe server
- implement single-connection download
- implement writer
- add basic progress UI

### Phase C — segmented download

- implement segment generation
- add parallel connection workers
- merge segment files
- validate range behavior

### Phase D — adaptive scheduler

- add bandwidth monitoring
- add connection ramping
- add ETA-based stealing
- add tail handling

### Phase E — persistence

- implement state save/load
- implement resume
- implement queue persistence
- implement pause/resume flow

### Phase F — polish and hardening

- improve error handling
- add graceful shutdown
- finish tests
- add ADRs
- update README and release notes

---

## 29. Final quality gate

Before the project is considered complete, confirm all of the following:

- repository structure is clean
- package boundaries are respected
- no circular dependencies exist
- no ignored errors remain
- all core logic is tested
- race detector passes
- resume works after interruption
- queue persists correctly
- state files are versioned
- commit history is professional
- no emojis appear in commit messages
- only meaningful commits remain in history
- docs match behavior
- the CLI feels polished and stable

---

## 30. Appendix — original project intent

PADS remains a predictive adaptive download scheduler for Go, but now with the engineering discipline expected from a serious portfolio-grade implementation.

_End of file._
