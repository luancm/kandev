---
id: 08-current-docs-and-evidence
title: Consolidate current Errand contracts and historical evidence
status: done
wave: 8
depends_on:
  - 07-browser-read-owner
plan: plan.md
requirements:
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRAND-HISTORY-001
  - REQ-TASKS-ERRAND-COMPLETION-001
acceptance_criteria:
  - AC-TASKS-ERRANDS-002.8
  - AC-TASKS-ERRANDS-004.9
  - AC-TASKS-ERRAND-HISTORY-001.1
  - AC-TASKS-ERRAND-HISTORY-001.6
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
  - ../../specs/tasks/system-design/errand-completion-cleanup.md
  - ../../specs/tasks/system-design/errand-await-receipts.md
  - ../../specs/tasks/system-design/errand-history-loading.md
---

# Task08: current authority map and preserved historical evidence

Complete the documentation consolidation identified by the quality review. Preserve existing product behavior, unique contracts, meaningful decisions, protocols, failed measurements and reproducible evidence. The parent supplies an immutable published pre-consolidation checkpoint before any original evidence is removed from the tip; this is a delivery dependency, not permission to discard dirty work.

## Current contracts

Consolidate docs/specs/tasks/system-design/errands.md using existing companion authority. Preserve its frontmatter, requirement map, every existing heading/anchor, unique persistence/DTO/MCP/API/error/WS contracts, status-block bounds, workflow-detached exclusions, prompts/security, observability and release-toggle behavior.

Replace duplicated Components, Start, Queued admission, settlement, Await, parent delivery, cancellation, failure/recovery and frontend bodies with concise ownership summaries and links. Keep locally the unique eligible-parent/five-live/profile/reserved-identity/attach-only launch rules, ordinary-first queued order, parent-origin/result capture and 16KiB rune bound, exact CanSendResult predicate, waiter behavior, running-parent/Auto-run/parking/outstanding rules and each existing cancellation trigger/source/outcome. Do not discard a unique contract merely because an adjacent algorithm has a companion.

Add one Errands design map linking the ten authorities: recovery/delivery, await receipts, completion cleanup, status/continuation, history loading, conversation navigation, context window, panel order, await duration and result duration. Replace duplicated final recovery prose with the existing repair-package link. Correct the old WAITING_FOR_INPUT completion description to the existing exact-settlement/confirmed-close/conditional-COMPLETED contract; correct abandonment wording so only proven fenced abort releases eligible claims and uncertain reception stays retained. Restore the existing missing inline-current-errands-summary anchor as a forwarding anchor to the conversation/history authority. Any real source/contract disagreement is reported, not silently resolved as new behavior.

In docs/specs/tasks/README.md replace the six partial Errand system-design entries with one link to the complete umbrella design map; keep requirements and unrelated systems intact. Measure AFTER repository formatting: the untouched umbrella formats to42,773bytes even though its raw text is32,762bytes. Sol's in-memory consolidation trial was32,526bytes before a further duplicate paragraph cut. Final umbrella must be <=32KiB and index <=16KiB, with sensible headroom; no lint exemption or needless new document split.

## Package status corrections

Reconcile only current status and directions against accepted integration receipts; preserve historical incident narratives and limitations.

- Mark docs/plans/errands/plan.md and docs/plans/errands-review-repairs/plan.md done: their acceptance checkpoints and all referenced work orders already record completion.
- Mark task-04-ordinary-header-subscriptions.md done for delivered source and completed desktop3/3, phone3/3 and28-scenario checks. Keep release-performance acceptance explicitly open with every numerical observation. Replace its future pilot direction with the completed, qualified bounded-pilot-receipt.md.
- In errand-tool-terminal-replay/plan.md and task-01-exact-tool-replay.md, add the current delivered checkpoint from final-integration-handoff.md (source ee0f077cb9 includes exact late-tool replay). Label the old launch/integration hold historical; do not claim the unidentified historical record or main instance was repaired.
- In errands-remaining/plan.md, replace current launch holds/next-pilot directions with the completed isolated pilot and continuing quality roadmap. Keep the delivery package done; private pilot evidence is not main-instance deployment.
- Refresh bounded-use-roadmap.md through a link to the simplification plan's current delivery checkpoint instead of another independently maintained SHA/status ledger. Preserve the open performance gate.
- Preserve the parent's orphan package/work-order done corrections. History-loading's delivered status with disclosed exceptions and parent-terminal-identity's historical sections are already correctly qualified. Keep unfinished simplification work open and do not infer authority to repair retired records.

## Historical evidence

Before pruning, inventory each exact current source and its immutable Git blob. Parent must provide a committed/pushed checkpoint matching the current originals; never label a dirty working copy as matching an older commit. Original delivery d08b07c5b399f548092b290a91187f8eccf47313 and published review47f459b5cd312333ac689a460376af0781ab5c24 have tree62cebc1a45566c19c2e96152131f4ae9372c28f3 and already contain all eleven derived JSONs exactly. Retain reachability through preserved remote history, plus a verified durable archive.

Archive originals under /home/luancm/kandev-task-evidence/errands-20261115 with path, byte length, SHA-256, Git blob and immutable commit in a manifest. Verify every archive member, restore into a separate directory and byte-compare every file. Existing raw-run archives are not substitutes for preserving these derived files or narratives. Do not touch live database/runtime or remove existing archives.

After preservation, remove the eleven derived result JSONs from the tip: nine under errand-history-loading (baseline-control, baseline, candidate, main-comparison, paired-performance, paired-performance-v2, retention-browser, summary-followup, zero-history-diagnostic), plus errands-simplification/final-verification-results.json and ordinary-header-verification.json. Recheck actual source/script consumers; the read-only exact-name search found none, but an external consumer is not disproved by that search. If a supported consumer exists, retain or migrate it explicitly.

Keep one concise tracked evidence index with immutable GitHub/blob links, hashes, extraction/verification commands and durable-archive location. Redirect every inbound Markdown link and reproduction command to an exact retained record/extraction path. Preserve numerical protocols and current outcome summaries rather than duplicate every raw sample. Workloads, frozen thresholds, failed cohorts, open phone timing/heap warnings and evidence limitations stay unchanged.

Replace orphan-settlement/implementation-handoff.md, recovery-audit-remediation.md and simplification/ordinary-header-subscriptions-handoff.md with short same-path historical receipts pointing to their immutable originals; preserve needed inbound anchors. Condense only the completed Results narratives in simplification Tasks01–03: keep pre-Results objectives, scope, acceptance and commands unchanged; retain final counts, RED/failure/timeout disposition, authority repair, review and limits. Preserve the parent's corrected current statuses. No test deletion is authorized: sampled public, race, reopen and receipt-failure oracles protect different guarantees.

## Ownership and acceptance

Own those named documentation/evidence paths, one evidence index and affected inbound documentation links/commands. Own this Results section and one concise receipt. Parent owns package manifest, roadmap decisions, immutable checkpoint creation and Git delivery. One shared-checkout writer including docs; do not stage, commit, push, run production tests/builds or alter source/test semantics. Do not revert other work or delegate.

1. Current authority is findable and non-contradictory, original anchors and unique contracts survive, and formatted document-size/catalog/spec gates pass.
2. Every pruned original is recoverable byte-for-byte through a verified durable archive and a reachable immutable Git record. No missing links, lost failure evidence, changed caps or unsupported portability claims.
3. Historical receipts clearly distinguish then-current state from delivered/current acceptance. Evidence inventory shows retained responsibilities rather than treating lower line count as correctness.

## Verification

Use repository Prettier on the exact changed Markdown files. Run formatted-byte assertions, a focused local-link/anchor check for all changed/redirected paths, and from repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the repository documentation-coverage check using its actual supported invocation for this docs-only scope; record whether it is exempt rather than inventing a pass. Report the exact manifest/archive hashes, restored-member counts, reachable commit/ref evidence, formatted sizes and unresolved links. No runtime/browser rerun follows from unchanged documentation contracts.

## Results

Implemented and accepted after independent documentation review; documentation status is `done`. The [handoff](task08-docs-handoff.md) records scope, validation and limits. No production/test/config/locale/SDK change, build, runtime operation or Git mutation was performed.

Published base `55b5865a671d22036cd3dd4a3fd6836cf65611ae` matched all 17 archived originals byte-for-byte before pruning. The [evidence index](evidence-index.md) retains immutable commit/blob links and full hashes, extraction commands and verified durable locations for those originals, Task07 source receipts and accepted B4 artifacts/receipts. Eleven derived JSONs were removed only after that check and consumer/link audit; six completed narratives were condensed with immutable originals retained. Failed cohorts, caps, uncertainty and performance warnings remain.

Repository-formatted umbrella/index are 31,634/16,015 bytes, below 32/16 KiB. All original headings survive, the missing inline-summary forwarding anchor is restored, and all ten companions link back to the owner map. Frontmatter and nine retained requirement/API/WS/status-block/prompt/security/metric/flag sections are unchanged except table formatting; Tasks01–03 pre-Results prefixes are byte-identical. Catalog validation passed (455 decisions, 2033 specifications), all-spec lint and whitespace checks passed, and the actual documentation-coverage API returned `exempt` for docs-only paths. The focused link check found no new broken links; three unrelated index targets were already absent at the published base and remain explicitly reported in the handoff. No new product-policy decision was required. Independent review is accepted. Parent delivery is tracked in the task plan; global numerical gates and review-history preparation remain open.

Independent review accepted the remaining scope and identified one P2 omission in the condensed Start section: profile guidance must use the same eligibility/executor filter as launch validation. Restored that existing invariant without changing policy. The narrow correction and updated final manifest are retained beside the first frozen snapshot; independent narrow review accepted the correction with no remaining findings. Documentation status is `done`; global numerical acceptance and review-history preparation remain open.
