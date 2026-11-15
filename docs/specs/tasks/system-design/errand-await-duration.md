---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ERRAND-AWAIT-DURATION-001
created: 2026-10-30
owners:
  - kandev
---

# Errand Await Duration Design

The current `AwaitResult` and `awaitErrandsResponse` carry no duration, and `KandevRendererProps` exposes no start or end time. Measure elapsed time with Go's monotonic clock inside the actual validated `Service.Await` operation, starting immediately before registering the waiter and ending on a successful `awaitLoop` return. Include an additive nonnegative `duration_ms` in the await response through the existing MCP result path; ensure zero is represented rather than omitted. Errors retain existing behavior and do not synthesize a duration. Successful immediate, timeout and parent-ended responses carry their real duration. Do not alter result claiming, cancellation, the timeout or authorization.

The existing tool-result persistence and normalization must be verified end to end, including MCP transport and transcript reload, before relying on the field. Parse only finite nonnegative numeric milliseconds from the result; missing or malformed legacy data is unavailable. Render a concise localized elapsed label in the completed await card using existing duration formatting conventions. Do not derive time from maximum wait, child age, client receipt time or general turn duration. Running cards retain existing spinner without a speculative timer. The phone chat uses the same compact card and retains one transcript scroller, existing Open targets and viewport containment.

Add backend tests for real wait, immediate/zero, timeout and ended paths and handler/transport serialization. Add frontend parser/rendering and persisted-reload browser coverage for valid, absent, invalid and zero values, on desktop and phone. Preserve existing result shape compatibility and document the additive field where the tool contract is described.

## Await outcome and delivery

Expose an additive closed-set end reason from the actual await-loop branch: `all_ready`, `any_ready`, `timeout`, or `parent_ended`. A zero-second poll that finds no ready result is a timeout; errors do not fabricate a successful end reason. Preserve it through the existing response and transcript. Missing or unknown legacy reasons render no reason. Per-row result-returned indicator relies only on evidence that this await carried the result, never merely on a settled status or another delivery route. Preserve the historical await result snapshot even if live child state later changes; do not imply a result was returned from an empty or ambiguous payload. Localize labels and provide accessible compact phone and desktop presentation.

## Completed history presentation

The user selected Option C: a quiet, initially collapsed disclosure row showing errand count, actual elapsed wait and recorded end reason. Errand names and per-item outcomes appear only when expanded. Completed rows use static icons; a pending result is described as not finished when the wait ended, never inferred to be running. Expanded details are explicitly historical and remain unchanged by later live state. In-flight awaits retain their existing live behavior. Reuse the quiet ActivityChip disclosure language, with a 44px phone/coarse-pointer summary target, keyboard operation, aria-expanded and inline details in the existing transcript scroller. Existing per-errand Open actions remain available inside expanded details. See [repair Task 27](../../../plans/errands-review-repairs/task-27-history-disclosure.md).

The same historical presentation extends to START and COMPLETION under [conversation navigation](errand-conversation-navigation.md). Expanded waits may expose the bounded result text recorded in that await, rendered as text only; collapsed summaries and recorded statuses remain unchanged.
