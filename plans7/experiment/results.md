# results.md — Controlled Authoring-Prompt Experiment (prompt4.md)

Date: 2026-08-23 · Intent: fixed salary-vs-W2 requirement · ≤3 attempts/model/arm

## Matrix (attempts passing each level)

| Model | Arm | Att | Snapshot | Struct | Gate4 | ScenarioExec | FULL |
|---|---|---|---|---|---|---|---|
| gemma-4-31b | new | 3 | 3 | 3 | 3 | 0 | **0** |
| laguna-s | new | 3 | 1 | 1 | 1 | 1 | **0** |
| nemotron-ultra | new | 3 | 1 | 1 | 0 | 0 | **0** |
| gemma-4-31b | new-retry | 3 | 3 | 3 | 3 | 0 | **0** |
| laguna-s | new-retry | 3 | 0 | 0 | 0 | 0 | **0** |
| nemotron-ultra | new-retry | 3 | 3 | 3 | 2 | 2 | **1** |
| gemma-4-31b | old | 3 | 3 | 1 | 0 | 0 | **0** |
| inkling° | old | 3 | 0 | 0 | 0 | 0 | **0** |
| laguna-s | old | 3 | 3 | 2 | 1 | 1 | **0** |
| nemotron-super‡ | old | 3 | 0 | 0 | 0 | 0 | **0** |
| nemotron-ultra | old | 3 | 2 | 1 | 0 | 0 | **0** |
| north-mini-code† | old | 3 | 0 | 0 | 0 | 0 | **0** |

† hard-unavailable (3× TIMEOUT) — substituted by ‡ nemotron-super per rule; ‡ also hard-unavailable (3× TIMEOUT).
° categorically blocked: OpenRouter serves inkling only via agentic harnesses (HTTP 403 direct API).

## Endpoint-contamination notes

- Evening degradation hit nemotron-ultra cells (upstream-overload / timeouts mid-cell).
- Two nemotron-ultra/improved attempts reached scenario-exec with perfect self-prediction but were blocked by a HARNESS bug (empty codified reason); rows purged as tooling-invalidated per established precedent; the re-run cell still qualified for the matrix.

## Failure taxonomy (all classified attempts)

- gemma-4-31b/new: SCENARIO:3
- laguna-s/new: INVALID_JSON:1, SCENARIO:1, TIMEOUT:1
- nemotron-ultra/new: OTHER:2, COMPILE:1
- gemma-4-31b/new-retry: SCENARIO:3
- laguna-s/new-retry: TIMEOUT:2, INVALID_JSON:1
- nemotron-ultra/new-retry: COMPILE:2, SUCCESS:1
- gemma-4-31b/old: STRUCTURAL:2, COMPILE:1
- inkling°/old: OTHER:3
- laguna-s/old: SCENARIO:1, COMPILE:1, STRUCTURAL:1
- nemotron-super‡/old: TIMEOUT:3
- nemotron-ultra/old: STRUCTURAL:1, COMPILE:1, OTHER:1
- north-mini-code†/old: TIMEOUT:3
