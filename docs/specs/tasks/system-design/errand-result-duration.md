---
status: draft
system: tasks
created: 2026-11-10
owners:
  - kandev
requirements:
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
---

# Historical Errand result elapsed time

This focused extension of [Errands](errands.md) owns completion snapshot timing. REQ-TASKS-ERRANDS-010 maps to the snapshot and shared rendering contract; REQ-TASKS-ERRANDS-011 maps to phone containment. Existing settlement, delivery and navigation authority remains unchanged.

## Snapshot and rendering contract

`DeliveryItem` optionally carries `duration_ms`, an integer number of elapsed
milliseconds from the claimed errand's `created_at` to its recorded `settled_at`.
This matches the existing panel's creation-to-settlement meaning: queueing,
idle intervals and later follow-ups are included. It is not provider execution
time or the duration of a parent's await. For an explicitly sent result the
endpoint is its recorded result capture, as stored by `writePendingResult`.
The task system owns this measurement because it owns the result snapshot.

Both normal composition and preserved cancellation notices take the measurement
from the same row/revision used for the result's text. Missing or zero-value
timestamps and reversed intervals omit the optional field; an actual zero
interval is preserved. Never substitute `updated_at` or the current clock.
The Go representation must distinguish omitted from zero. This additive JSON
field requires no table migration and must survive the existing metadata
serialization and admitted-prompt replay paths unchanged.

The frontend parser accepts only finite nonnegative safe integer milliseconds;
missing or invalid values become unavailable without hiding the result card.
The shared finished disclosure formats the snapshot using existing localized
duration units, showing it in the collapsed summary and beside the expanded
historical outcome. Legacy snapshots omit elapsed text; no live-row lookup or
history backfill supplies it. Later child turns, delivery retries, deletion and
reload cannot change an already saved measurement. WAIT retains its separately
recorded wait duration.

Desktop and phone share the disclosure and snapshot parser. Long localized
durations may wrap with status while title, result and Open stay usable; the
existing chat remains the scroll owner. The [duration work order](../../../plans/errands-remaining/task-01-result-duration.md)
maps AC-TASKS-ERRANDS-010.6/010.7 and 011.5/011.7 to producer, parser, rendering
and browser evidence. This is an additive implementation of recorded historical
timing, not a change to delivery, settlement or continuation authority.
