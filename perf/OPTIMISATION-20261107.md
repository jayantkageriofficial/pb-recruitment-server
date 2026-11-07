# Callback and compilation optimisation — 7 November 2026

**GO for isolated full-stack evaluation; NO-GO for production activation or
600-participant capacity certification.** Local database contention and compiler
cost improved, with correct results and recovery. The combined implementation has
not executed against a native API/Judge0 stack. Production remains unchanged.

[Aggregate measurements and validation](results/optimisation-20261107.json)
contain every measured repetition, query timings, lock samples, pool waits and
the unexecuted stages. Protected logs and test binaries are excluded from Git.

This publication contains the report, aggregate results and handoff link only.
The measured application, tests and harness changes remain uncommitted in the
local `perf/k6-judging-suite` worktree; this push does not publish that implementation.

## Implementation and compatibility

Work continues on `perf/k6-judging-suite`, based on `769677c`. Application baseline
code is the branch's unchanged main revision `d32cbfb`; the historical production
retest measured an older application revision and remains separate evidence.

`ExecutionStore.ProcessFinal` now locks only its execution. One statement saves
the fenced final result, token and testcase result in the same transaction.
After commit, completion checks whether every case is durable. Only a ready
parent acquires a separate, nonblocking `FOR NO KEY UPDATE SKIP LOCKED` lock for
aggregate verdict and ranking updates. Concurrent deliveries can acknowledge
their durable result while another scorer completes the parent. If scoring is
interrupted or an unrelated transaction holds the parent, the existing recovery
sweep completes the pending parent. There is no new queue, scheduler or table in
the recruitment database.

First-final-result semantics, attempt/claim/token fences, blank-output handling,
failure precedence, sum-runtime/max-memory aggregation, missing-result repair,
MCQ/code ranking serialization and retained accepted scores remain covered by
regressions. Callback authentication, admission controls, submission retries and
frontend polling are unchanged. A scoring failure may now leave a durable child
and pending parent; this is intentional and explicitly tested through recovery.

The existing compile-once candidate was ported from `dc110f5` (PR #87), without
reverting main's newer problem-progress changes. `JUDGE0_COMPILE_ONCE=false`
remains the default. Enabling it requires the patched Judge0 API and every
worker, a compiler-build identity, and the Judge0-only migration described in
[the candidate's rollout instructions](https://github.com/pointblank-club/pb-recruitment-server/blob/dc110f573cd20425f28188984a0d6ddc66c83f78/judge0/compile-once/README.md). It shares bounded
C++/Java compilation artifacts within a submission, with separate fresh native
sandboxes, limits, verdicts and callbacks for every testcase. Recovery snapshots
retain the group. Ten testcases still mean ten executions and callbacks.

The harness's two remaining hardcoded k6 paths now honor the existing `K6`
override, allowing the documented checked archive to validate on this machine.
Workload pacing, budgets, thresholds and phase-preserving regression are retained.

## Comparable local callback measurements

The baseline uses the original execution store from `769677c`, selected through
Go's file overlay, and the same measurement harness as the candidate. Both use
local PostgreSQL 18.6, 25 open/5 idle connections, identical pre-created fixtures,
five-second callback contexts, and concurrent deliveries for one parent at a
time. Each table cell contains three repetitions of 30 batches. The displayed
p95 is the **median of the three run p95s**, not a pooled percentile.

The workstation has an eight-core/16-thread Ryzen AI 7 350, Go 1.26.5 and Kali
2026.3. Database statement statistics and a separate lock sampler were enabled
for both sides. Preparation, verification and schema teardown are outside timed
batches. These are database microbenchmarks, not authenticated API traffic.

| Cases | Deliveries per case | Before callback p95 | After callback p95 |
| ---: | ---: | ---: | ---: |
| 1 | 1 | 2.26 ms | 3.09 ms |
| 1 | 3 | 4.95 ms | 3.57 ms |
| 10 | 1 | 20.97 ms | 10.25 ms |
| 10 | 3 | 58.20 ms | 18.29 ms |
| 20 | 1 | 48.18 ms | 19.44 ms |
| 20 | 3 | 112.37 ms | 22.57 ms |

Per variant, all **540 measured parents, 5,580 testcase results and 11,160 callback
deliveries** completed correctly, with zero callback errors, unresolved measured
parents or score mismatches. An additional 18 untimed fixture parents/executions
per variant were scaffolding and were removed with their exact schemas.

For ten cases/one delivery, testcase completions during timed batches rose from
468.2/s to 986.1/s; for twenty cases/three deliveries, from 178.5/s to 878.3/s.
These rates exclude fixture work and gaps between batches, have no native judge,
and must not be read as sustained submission or fleet throughput.

The original parent-lock SELECT ran 11,160 times and accumulated **286.817s** of
database execution time, including concurrent lock waits. The candidate's
nonblocking parent SELECT ran 4,085 times and accumulated **0.0718s**. Those sums
are neither wall-clock durations nor isolated lock-wait measurements. The lock
sampler observed at most **24 versus 7** simultaneous lock waiters. For the
twenty-case/three-delivery cell, median cumulative Go pool wait fell from
41.825s to 12.409s per 30-batch repetition. Counts of sampled waits are retained,
but sampling intervals/total run lengths differ, so no exact wait-duration claim
is made from those samples.

**Tradeoff:** the ordinary single-case callback is slower locally because of the
additional transaction. This regression is retained. Its effect over the real
database network and on end-to-end receipt/verdict latency needs native testing.

## Separate configuration-only comparison

On the unchanged final candidate, only the local test pool's idle limit changed
from five to 25; its open ceiling stayed 25. No application or production config
file was edited. The same repetitions, fixtures and instrumentation were used.

| Cases / deliveries | Candidate, idle 5 p95 | Candidate, idle 25 p95 |
| --- | ---: | ---: |
| 1 / 1 | 3.09 ms | 3.42 ms |
| 10 / 1 | 10.25 ms | 4.78 ms |
| 10 / 3 | 18.29 ms | 5.86 ms |
| 20 / 1 | 19.44 ms | 5.52 ms |
| 20 / 3 | 22.57 ms | 8.17 ms |

The twenty-case/three-delivery pool-wait sum fell to 4.715s per repetition, while
the peak sampled lock-waiter count increased from seven to 17. More retained
connections permit more concurrent work; the latency improvement does not prove
every contention measure improves. This supports an isolated deployment-level
comparison, not an unapproved production change. Callback timeout 10s and native
worker counts 8/12/16 were not exercised.

## Local compiler/cache comparison

Three repetitions per language/mode executed ten distinct input/output cases in
fresh directories. Baseline and cache order alternated. Fixture database rows
were prepared outside timing; compilation, artifact storage/restoration and all
ten program executions are included. All **120 expected outputs** were verified.

| Ten-case group | Before elapsed | After elapsed | Before CPU | After CPU | Compiler calls |
| --- | ---: | ---: | ---: | ---: | --- |
| C++ | 1,936.23 ms | 222.36 ms | 1,948.69 ms | 217.26 ms | 10 → 1 |
| Java | 2,560.92 ms | 577.51 ms | 8,000.63 ms | 1,374.46 ms | 10 → 1 |

Values are medians of three runs. CPU is the Ruby process plus child-process CPU,
and can exceed elapsed time. Across the comparison, baseline/cache performed
60/6 actual compiler invocations while each executed 60 testcase programs.

These are host GCC 16.2 and OpenJDK 25 compiler/cache measurements with Ruby 3.3.8,
ActiveRecord 7.2.3.1 and PostgreSQL 18.6. The installed JVM compiler module supplied
`javac`. They are **not** measurements of Judge0's pinned Rails/toolchain, Isolate,
deployment cgroups, queues, callbacks, limits or API scoring. The candidate's
earlier native CI results remain historical functional evidence only.

## Validation achieved and stages not started

- `go test -race -count=1 ./...` with the disposable PostgreSQL database and the
  opt-in contest-load test enabled: **444 passing test/subtest events**, no
  failures. Four explicit skips were the separately measured microbenchmark and
  three real-Judge0 tests requiring a native endpoint.
- All three 600-submission/ten-case integration scenarios passed: polling,
  duplicate callbacks racing three recovery workers, and a 100-job admission
  queue. Together they finalized **1,800 submissions/18,000 testcase results**
  with correct rankings. The capacity scenario rejected 600 optional Run calls
  into the scored backlog and recovered all saved work. S3 and Judge0 were
  simulated; these are regression checks, not 600 authenticated participants.
- Final simulated scenario drain times were 6.05s, 16.78s and 28.23s respectively,
  within the existing 90-second regression deadline. Native queue depth/age and
  production recovery were not measured by these runs.
- New lock-isolation, durable-result rollback, interrupted scoring, busy-parent
  deferral and concurrent repair checks passed; selected callback checks also
  passed five race-enabled repetitions.
- Go build and vet passed. Ruby artifact checks: **4 tests/11 assertions**;
  PostgreSQL/cache and real host compiler checks: **6/35**; compiler benchmark:
  **1/372**; all passed with no skips/errors.
- Official SHA-verified k6 2.3.0 inspected all **19 profile/workload combinations**.
  Five Python harness tests and Node arithmetic/encoding/phase-pacing checks
  passed. The local k6 HTTP stub executes no submitted program.

The local Docker socket denied access and passwordless local sudo was unavailable;
there was no usable isolated native API/Judge0 deployment. Consequently the new
native smoke, baseline, controlled average arrivals and corrected participant
journey **were not started**. Neither were deadline/spike/stress/breakpoint,
worker/configuration comparisons, native faults/interruption, large-volume
checks or the representative 120-minute soak. No failed threshold was raised,
no load was escalated on production, and no submission POST was retried.
The runnable native profiles, fault proxy, worker interruption guard and pinned
native compile-once CI workflow are preserved.

## Production observations, cleanup and recommendation

Read-only inspection found the API still using image
`sha256:fecb5ce816090a3d497848e1bfaa3665c462d6e783e4530086d473b105c0639a`,
started 6 November at 18:19:23 UTC with zero restarts, matching the historical
retest. Its binary contains no VCS revision, so the historical attribution to
`2efc783` is image provenance, not a newly extracted commit SHA. The server
checkout is newer main `d32cbfb`; it is not the effective deployed binary.
Judge0 web/workers retain the historical image digest. Mounted API configuration
retains 25 open/5 idle connections, the 10,000ms client timeout and configured
callback base/secret. Judge0's callback timeout/COUNT entries remain blank and
queue limit remains 1,000; their documented effective defaults are not new tuning
results. No secrets were printed or copied into the branch.

The public API now lists **11** contests, none with a listed 120-minute duration.
Eight problem lists were readable and three returned 403. Hidden testcase costs
and the actual recruitment contest remain unconfirmed; no authentication was
bypassed. Historical 10-contest observations were not relabeled as current data.

One boundary deviation occurred before the handoff was read: a temporary source
worktree and branch were created on the server to locate/read it. Both were
removed. Subsequent server inspection was read-only. The production checkout
remains clean, and no production deployment, restart, scaling, configuration or
application data change occurred.

No production identities, contests, submissions, object keys or native jobs were
created. The shared production `run:capacity` limiter and native retention were
untouched; historical cleanup inventories were not re-audited. Every local test
schema was verified absent. The test-owned PostgreSQL process was stopped and
its exact `perf/.private/pgdata` directory removed; protected aggregate inputs
and logs remain ignored locally.

Historical normal-paced production evidence still stands: 0.83 submissions/s in
the 600-user hold, receipt p95 2.69s and verdict p95 12.61s, with 355 failed callback
attempts followed by eventual delivery. The separate constant-arrival 0.833/s run
still failed its retained 3s receipt target at 3.085s. **No new end-to-end "after"
latency, callback HTTP failure rate or native capacity result is available.**

Proceed with isolated full-stack smoke and correctness, then identical baseline,
average arrival and corrected journey comparisons. Preserve receipt <3s,
verdict <60s, read <2s, correctness and recovery gates. Before production activation,
complete native limits/fault/recovery checks, deadline capacity and the 120-minute
soak with confirmed contest costs. Any production rollout/configuration experiment
still requires Hemang's explicit authorization, a drained queue and controlled
service changes under the original handoff boundaries.
