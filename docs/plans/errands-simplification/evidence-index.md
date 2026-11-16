# Errands historical evidence index

The [package plan](plan.md) owns delivery status. These exact originals preserve measurements, protocols, failures and implementation narratives before Task08 consolidation; they grant no new acceptance. Global performance acceptance remains open. See the [current measured disposition](../errands-remaining/code-quality-roadmap.md#current-measured-disposition-2026-11-16), [Task09 results and verified current archives](task-09-ordinary-recognition-experiment.md#results), [historical ordinary receipt](ordinary-header-verification.md) and [first-delivery exceptions](../errand-history-loading/summary-followup-handoff.md). Earlier failed cohorts and frozen caps remain unchanged.

## Recovery and verification

All 17 originals (11 JSONs and six complete Markdown files, 5,699,179 bytes) matched immutable checkpoint `7f99994b184234eaa2a2a9695140fa7f62a49f67` and published pre-consolidation checkpoint `55b5865a671d22036cd3dd4a3fd6836cf65611ae`. The original `d08b07c5b3` / review `47f459b5cd` identical-tree pair is earlier history; that review branch is not the current candidate. Source history is retained on the original delivery branch. GitHub links require repository access.

The verified host-local archive directory is `/home/luancm/kandev-task-evidence/errands-20261115/task08-preconsolidation-7f99994b1842/`:

- `originals.tar.gz`: SHA-256 `c48430cc24b00d103b179a9b686c0156bd2fc7e5af7643a1f1c52101d3193874` (606,220 bytes; 17 originals plus manifest).
- `manifest.json`: SHA-256 `1648c21e86f8c9ad8332f3dc1059e71bf4a88e1f6c68340f3b74360b7bd1a9a4`; records paths, sizes, SHA-256 and Git blob IDs.
- `restoration-receipt.json`: SHA-256 `1ed3e7a5f971248e08f6ebc07209f0a241397d8c882005d1272d3da21fa474c7`; all 17 extracted originals byte-compared with checkout and fresh Git extraction. The adjacent `restored/` directory retains that extraction.

Task08 rechecked all 17 against published `55b5865a671d` before pruning, with no differences. The receipt is `/home/luancm/kandev-task-evidence/errands-20261115/task08-execution-55b5865a671d/preprune-verification.json`. No raw-run archive is treated as a substitute for these exact derived originals. The host-local archive needs host access; immutable Git extraction provides a separate reviewer route.

For example, recover the frozen baseline without rebuilding or changing limits:

```bash
git show 7f99994b184234eaa2a2a9695140fa7f62a49f67:docs/plans/errand-history-loading/baseline-results.json > /tmp/errand-baseline-results.json
printf '%s  %s\n' cfe958aa82328edb59e1e6c9c6620acfd7295d112c4360d9de2eac8dd1f5b4dd /tmp/errand-baseline-results.json | sha256sum -c -
```

For any record below, use its original repository path after the same immutable commit and verify its listed SHA-256. Extraction can target any separate evidence directory. To verify/extract the archive itself:

```bash
cd /home/luancm/kandev-task-evidence/errands-20261115/task08-preconsolidation-7f99994b1842
printf '%s  %s\n' c48430cc24b00d103b179a9b686c0156bd2fc7e5af7643a1f1c52101d3193874 originals.tar.gz | sha256sum -c -
mkdir -p /tmp/errand-task08-originals
tar -xzf originals.tar.gz -C /tmp/errand-task08-originals
```

## Exact originals

### baseline-control-results

[docs/plans/errand-history-loading/baseline-control-results.json](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errand-history-loading/baseline-control-results.json): 20850 bytes. SHA-256 `be13b2559847552fba37b1a17518301a88cab8a19b9d0b8f1485ebe2a2b950f5`; Git blob `424b7b5a2d3519426f23cc53c675aef23594b17d`.

### baseline-results

[docs/plans/errand-history-loading/baseline-results.json](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errand-history-loading/baseline-results.json): 224170 bytes. SHA-256 `cfe958aa82328edb59e1e6c9c6620acfd7295d112c4360d9de2eac8dd1f5b4dd`; Git blob `baa2a2ef0bbc775b7c444951443474cb244114ea`.

### candidate-results

[docs/plans/errand-history-loading/candidate-results.json](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errand-history-loading/candidate-results.json): 171748 bytes. SHA-256 `d7800423cfc9e2e02bb36832e44cb1154715c25391501384252bc22c15b1b8f3`; Git blob `2f2bfe200de9c5d4ffa07bacc0a795957f328423`.

### main-comparison-results

[docs/plans/errand-history-loading/main-comparison-results.json](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errand-history-loading/main-comparison-results.json): 349911 bytes. SHA-256 `03199a5ed4f89aa2f2eef01bb78d44f063e84f9d29a420321f2c89a623902072`; Git blob `63f4e7ef4c039c863301c8c74f3253c2edcc5b05`.

### paired-performance-results

[docs/plans/errand-history-loading/paired-performance-results.json](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errand-history-loading/paired-performance-results.json): 725924 bytes. SHA-256 `e0121077d65229b72f8fd5e89d36326f9ace32b291abfa33f21a5d041b4e84f6`; Git blob `44ffdeb2591134f6cd393795110f5d0f4758180e`.

### paired-performance-v2-results

[docs/plans/errand-history-loading/paired-performance-v2-results.json](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errand-history-loading/paired-performance-v2-results.json): 768445 bytes. SHA-256 `a2e441af213d53dafd588c5788a40ee0460a2b977859fcf3abb21755068cb93d`; Git blob `d9c8d66525174269b29c3284acb784a6a4295b53`.

### retention-browser-results

[docs/plans/errand-history-loading/retention-browser-results.json](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errand-history-loading/retention-browser-results.json): 26541 bytes. SHA-256 `3ad403f37bd82677089aff26faa533b62b048fcb07405de0a540abaf0bc7a8c3`; Git blob `ffc7af782b3213199bb7241a5b48fd23d3dfa1e8`.

### summary-followup-results

[docs/plans/errand-history-loading/summary-followup-results.json](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errand-history-loading/summary-followup-results.json): 164757 bytes. SHA-256 `515f9c70b4332050114cd8f93dda8ab8af9551c7ca25b1aa86a30f264c6a1781`; Git blob `b67d135782e108b577dbaf228a262495054e655b`.

### zero-history-diagnostic-results

[docs/plans/errand-history-loading/zero-history-diagnostic-results.json](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errand-history-loading/zero-history-diagnostic-results.json): 249730 bytes. SHA-256 `43200d20ea561179de2786365dd225b1d1432ae268e18d5e13b8a6e53559eac5`; Git blob `2401909e89e5d9bb5d14ba16377c7e2d519708b6`.

### final-verification-results

[docs/plans/errands-simplification/final-verification-results.json](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errands-simplification/final-verification-results.json): 2851156 bytes. SHA-256 `52d020ba92713694e256fbf27aa2b405ced12eafb7278c7c7611089e905c1a2c`; Git blob `8ac8ac6e9882b9c261f455efc9b40a026fb9ee23`.

### ordinary-header-verification

[docs/plans/errands-simplification/ordinary-header-verification.json](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errands-simplification/ordinary-header-verification.json): 72868 bytes. SHA-256 `61259f6650c43b0e7018c71a0f702025a8a4ab03fbe981379a28f66c976c231e`; Git blob `f8ba4c94fd96e07b910a11818edf76e1dfaa319c`.

### ordinary-header-subscriptions-handoff

[docs/plans/errands-simplification/ordinary-header-subscriptions-handoff.md](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errands-simplification/ordinary-header-subscriptions-handoff.md): 5307 bytes. SHA-256 `10bb1ce5cbacf75e8217202c94ea5f8080f31a691820128f8815aaa1e57aa6ec`; Git blob `d660b49645256ffcebaa0f1c2248870deaf6f23a`.

### task-01-private-browser-state

[docs/plans/errands-simplification/task-01-private-browser-state.md](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errands-simplification/task-01-private-browser-state.md): 9599 bytes. SHA-256 `8f636aef00b7a054f4f29f6ceae5b4ca63c44326367650a9152cd1c6dd70b64f`; Git blob `3c0cd918df5d58be0673c81a300fe6c5e85db16b`.

### task-02-lifecycle-controller

[docs/plans/errands-simplification/task-02-lifecycle-controller.md](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errands-simplification/task-02-lifecycle-controller.md): 24557 bytes. SHA-256 `ddab132b9947f30a7e33704c864e809131f7b744a2253353fc710184a96dc3c2`; Git blob `8a7eed6c77fca233503de08e05224fc108f12173`.

### task-03-owned-await

[docs/plans/errands-simplification/task-03-owned-await.md](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errands-simplification/task-03-owned-await.md): 12578 bytes. SHA-256 `e69cc7b3f3fc4610b2b8e5b13ce26eda94613b6c57ab70cdd4820a1c79cb8f2a`; Git blob `0598c36fba816ea911a3bb2dd1fa8b53f52aed77`.

### implementation-handoff

[docs/plans/errand-orphan-settlement/implementation-handoff.md](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errand-orphan-settlement/implementation-handoff.md): 8418 bytes. SHA-256 `7430c31358437b8b8460cb2c7e610cd84d0ba9a2a2a0ca1f95413007e866a421`; Git blob `7c87539a0343a5e8222518c61ac2b7611c121319`.

### recovery-audit-remediation

[docs/plans/errand-orphan-settlement/recovery-audit-remediation.md](https://github.com/luancm/kandev/blob/7f99994b184234eaa2a2a9695140fa7f62a49f67/docs/plans/errand-orphan-settlement/recovery-audit-remediation.md): 12620 bytes. SHA-256 `051058465aab34e3b5761f1b714a71ced7b4de8ac5e76c5a6b8bca35b6a9598c`; Git blob `8bcfdd07f0868aafc4eff575a5a8ead3f10e9ec1`.

## Task07 source checkpoint evidence

[Immutable source `90fad6e28e3e95901a735e6878d9d9474edd66e2`](https://github.com/luancm/kandev/tree/90fad6e28e3e95901a735e6878d9d9474edd66e2) is the source authority. Its parent is `7f99994b1842`; browser/numerical acceptance was not part of that source checkpoint. Later functional acceptance and integration belong to the [package plan](plan.md).

The host-local `/home/luancm/kandev-task-evidence/errands-20261115/task07-source-90fad6e28e/` preserves 77 named source/static logs, frozen snapshots and parent delivery receipts (261,905 original bytes; four empty logs retained). `originals.tar.gz` is 58,070 bytes, SHA-256 `26a1840dd25a1efba51d012242f27d77046a8329b70cc4387ee5f02da4a52223`; manifest SHA-256 `41b9cffba4042c8ce98b6e88d53522f83034d02f54307545b502a2ba6bb0f144`; restoration receipt SHA-256 `c0e396dee4ea54aa36741d7f8676395c59680c563d5ae07dcc187e2c47045dc4`. All 77 restored files matched original bytes. Active browser evidence and hook scratch were excluded. Frozen snapshot bytes are historical receipts, not a claim that every snapshot equals final source.

## Accepted B4 integration evidence

[Published integration `55b5865a671d22036cd3dd4a3fd6836cf65611ae`](https://github.com/luancm/kandev/tree/55b5865a671d22036cd3dd4a3fd6836cf65611ae) and the host-local `/home/luancm/kandev-task-evidence/errands-20261115/upstream-b4-55b5865a671d/README.md` own source lineage and acceptance limits. `accepted-artifacts.tar.gz` SHA-256 is `171e4bc9b7735103715fd4b12082203e17383e3e338269e9a3ffcdf08fb90d1e` (712 restored members, 1,042,650,697 bytes, zero mismatches). `verification-receipts.tar.gz` SHA-256 is `c6331297943eb64887f79952f602cd792ccd18572e54ef4935cc1db3a6433b6e` (88 restored files, 30,795,826 bytes, zero mismatches). `SHA256SUMS` SHA-256 is `e121f124746dca6a8e37625c2790b92366cd915dcab01c37208d76b285f54742` (91 files).

The unique-case index records 15 desktop and 17 phone cases. Its README preserves initial unit/geometry failures, lint timeouts, package-only race compilation, native-provider limits and the synthetic geometry transport boundary. Raw originals remain; unidentified quiet logs are not claimed. This does not approve global numerical performance.

## Limits

Exact-name searches across repository source, scripts, Markdown, HTML and command references found no supported programmatic consumer of the eleven removed JSON paths. Documentation links now resolve to the corresponding immutable record here. An external downstream consumer is not disproved by that search; reproduce historical analyses from extracted originals. The external final-performance bundle already copied its baseline/paired-v2 inputs and was not changed by Task08. Historical missing raw logs remain missing, PostgreSQL skips remain skips, and mock/emulated checks do not certify every provider, physical phone or main-instance deployment.
