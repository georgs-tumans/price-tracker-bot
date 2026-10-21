# Model benchmark runbook

Every model gets the same three implementation tasks (`TASKS.md`), starts from the same commit, and works in a clone that can't see anything else. Each result is scored by hidden tests and scope checks, then reviewed by hand.

## Who does what

| When | You | Claude |
|---|---|---|
| Once | Set up the Linux machine (step 0), create `bench-base` (step 1) | Kit is ready: tasks, hidden tests, reference solutions, scoring script |
| Per run | Run the model, push its branch, log the run (steps 2–3) | — |
| At the end | Run `score.sh` on Linux and push the results (step 4) | Read the summary, every diff and every report, write `RESULTS.md` |

## What's in this folder

| Path | What it is |
|---|---|
| `TASKS.md` | The three tasks. **The only file the models see**, committed into `bench-base`. |
| `hidden-tests/t1..t3/` | Tests the models never see; `score.sh` copies them in at scoring time. They rely on the fixed names in `TASKS.md`. |
| `reference/t1..t3.patch` | Claude's solutions. They only prove the hidden tests are fair: they fail on the base and pass on a correct solution. **Not a contestant**, since the same author wrote the tests. |
| `score.sh` | The scoring script. |
| `runs.csv` | Your run log, one row per run. |
| `results/` | Score output, one folder per scoring run. |

**This folder lives only on the `refactor-tests` branch.** Models never see it, because their clone contains only `bench-base` (step 2a), which is made from `develop`. So never merge `refactor-tests` into `develop` or `bench-base`, and never copy this folder into a run folder.

A model could still reach the kit in two other ways, and step 0 closes both:
- **The web:** the repository is public, so anything with web access can read every branch on GitHub.
- **Other folders on the run machine:** a model with shell access can read every folder your user can.

---

## Step 0 — prepare the machine the models run on (once)

Use identical settings for every run, and write them down in `runs.csv` notes:

- **Go 1.27.x:** check with `go version`.
- **golangci-lint v2.13.2**, the CI version. The models are asked to lint, so give them the right one:
  ```bash
  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
  ```
- **gcc**, needed by `go test -race`. The models can use it, and `score.sh` needs it to check T3 for data races.
- **Harness:** the same harness, version, permission mode, temperature and time limit for every model.
- **Web tools off:** disable web fetch and web search in the harness. The tasks don't need the internet, and the hidden tests are readable on GitHub. Go itself may still download modules.
- **No copy of the kit on this machine until the last run is done:**
  - don't create `~/bench-scoring` (step 4) yet, not even for the self-test;
  - don't pull or fetch `refactor-tests` in your existing project checkout (`~/Documents/Projekti/price-tracker-bot`). A fetch alone puts the kit into its `.git` folder, where `git show` can read it.
- **Global instructions:** turn off anything global the harness adds (user-level instruction files, memories, rules), or keep it identical for all models.

## Step 1 — create `bench-base` (once)

In any clone of the repository, on Linux or Windows:

```bash
git fetch origin
git switch -c bench-base origin/develop
git show origin/refactor-tests:_model-bench/TASKS.md > TASKS.md
git add TASKS.md
git commit -m "Add model benchmark tasks"
git push -u origin bench-base
git switch refactor-tests
```

`TASKS.md` is read from git because `_model-bench/` isn't on `develop`: it disappears from the working tree once you switch to `bench-base`.

**Don't change `bench-base` once runs have started.** Every run must start from the same commit. If `TASKS.md` needs a fix, tell me; we'd make a `bench-base-v2` and start all runs over.

## Step 2 — one run

**Matrix:** 6 models × 3 tasks × 2 runs = 36 runs. With 1 run each (18) you still get a ranking, but no idea of how much a model varies between runs, which the two Ornith 9B plans showed can be large.

**Order:** interleave the models: T1 for every model, then T2, then T3, then the second round. That way changes on the machine during the day hit everyone equally.

**Model IDs:** use short, fixed IDs, the same in every branch: `gemma-4-26b-a4b`, `qwen-3.5-9b-q5`, `ornith-1.5-9b-q5`, `ornith-1.5-9b-q6`, `ornith-1.5-35b-a3b`, `qwen-3.6-35b-a3b`.

### 2a. Prepare a clean clone

```bash
MODEL=ornith-1.5-9b-q5
TASK=t1                       # t1, t2 or t3
RUN=r1                        # r1, r2, ... never reuse a number
BRANCH=bench/$MODEL/$TASK-$RUN
DIR=~/bench-runs/$MODEL-$TASK-$RUN

git clone --single-branch --branch bench-base --no-tags https://github.com/georgs-tumans/price-tracker-bot.git "$DIR"
git -C "$DIR" remote remove origin
git -C "$DIR" switch -c "$BRANCH"
```

Removing `origin` means the model can't fetch the plan files on `refactor-tests` or another model's branch.

### 2b. Run the model

Start the harness in `$DIR` and paste this prompt **exactly**, changing only the task number:

```text
Read TASKS.md in the repository root and complete task T1. Follow its rules; where they differ from AGENTS.md, TASKS.md wins.
```

- **Don't help.** No hints, no corrections, no "continue".
- **If it asks a question,** answer exactly `No questions can be answered; decide yourself and note it in BENCH_REPORT.md.` and count one intervention.
- **If it stops halfway and waits,** you may send `continue` once. That also counts as one intervention.
- **Time limit: 60 minutes.** Stop the run there and note `timeout`.

### 2c. Collect the result

```bash
git -C "$DIR" status --short          # anything left uncommitted?
git -C "$DIR" branch --show-current   # still on $BRANCH?
git -C "$DIR" log --oneline bench-base..HEAD
```

- **Uncommitted changes:** commit them yourself so they get scored. `score.sh` flags this commit as yours:
  ```bash
  git -C "$DIR" add -A
  git -C "$DIR" commit -m "bench: uncommitted leftovers (operator)"
  ```
- **On another branch:** the model ignored the rules. Note it, then bring its work back: `git -C "$DIR" switch "$BRANCH" && git -C "$DIR" merge <its branch>`.

Then push:

```bash
git -C "$DIR" push https://github.com/georgs-tumans/price-tracker-bot.git "$BRANCH"
```

Pushing `bench/*` branches triggers no CI: all workflows run only for `develop` or releases.

## Step 3 — log the run

Add one row to `runs.csv`:

| Column | Example |
|---|---|
| `branch` | `bench/ornith-1.5-9b-q5/t1-r1` |
| `model`, `quant`, `context` | `Ornith 1.5 9B`, `Q5_K_M`, `128K` |
| `harness` | name and version |
| `date` | `2026-10-16` |
| `wall_minutes` | `23` |
| `tokens_in`, `tokens_out` | from the harness, if it shows them |
| `interventions` | `0` |
| `ended` | `done`, `timeout` or `crashed` |
| `notes` | anything odd, e.g. it asked to install something |

## Step 4 — scoring

Score on the Linux machine, which has gcc, so T3 is checked with `-race`. **Only after the last run** (step 0), make a clone of its own, outside `~/bench-runs`:

```bash
git clone https://github.com/georgs-tumans/price-tracker-bot.git ~/bench-scoring
cd ~/bench-scoring
git switch refactor-tests

bash _model-bench/score.sh --self-test    # the kit itself: base 0 hidden tests, reference all
bash _model-bench/score.sh --fetch        # every bench/* branch
```

For later scoring rounds, `git pull` in `~/bench-scoring` is enough.

**What it does.** For every `bench/*` branch, `score.sh` checks out the branch and runs:
- build and vet;
- the existing tests;
- lint with the base's `.golangci.yml`;
- the hidden tests, 3 times;
- the scope checks.

It writes `_model-bench/results/<time>/summary.md`, plus a log folder per branch with the diff, the model's report and every command's output. The summary header says whether `-race` was on.

**Leak check.** The "Kit seen" column lists any file on the branch that contains names existing only in this kit (`TestBenchT…`, `benchT…`, `_model-bench`, `hidden-tests`). It should say `no` everywhere. For a second check, search the harness's session logs for `_model-bench` and `refactor-tests`. Without gcc it runs without `-race`: the T3 tests still check starting exactly once, state-file sync and the deadlock rule, but not data races.

**Send me the results:**

```bash
git add _model-bench/results
git commit -m "Benchmark scores"
git push
```

Then tell me "review the bench scores". I pull `refactor-tests` and then:
1. read every `diff.patch` and `BENCH_REPORT.md`, and compare what each model claims with what the logs show;
2. write `results/<time>/RESULTS.md`: a ranking per task and overall, notes per model, and how scores vary between runs.

The first self-test was run on Windows; its result is in `results/20261021-130846/summary.md`. It shows 0 hidden tests passing on the base for each task, and 3/3, 7/7 and 7/7 with the reference patches, at 0 lint issues, with `-race` off. The Linux self-test above repeats it with `-race`.

## What gets scored

100 points per run; a model's task score is the average of its runs.

| Area | Points | How |
|---|---|---|
| Hidden tests | 40 | passed ÷ total × 40 |
| Build, vet, existing tests | 15 | all pass: 15; tests fail: 5; build fails: 0 |
| Lint | 10 | 0 issues: 10, −2 per issue |
| Scope | 10 | −5 per test edit that isn't allowed, −5 per protected file (`TASKS.md`, `AGENTS.md`, `.golangci.yml`, `go.mod`, `deployment/`, `.github/`), −3 per removed name still present, −2 for CRLF churn |
| Honest report | 10 | `BENCH_REPORT.md` against the real results. A claimed "all tests pass" that isn't true: 0. No report: 0. |
| Review | 15 | fits the codebase and `AGENTS.md` (comments, log prefixes, table tests), adds its own tests, no needless changes, updates docs (T2: `tracker_configs/README.md`) |
| Penalties | | −5 for an operator commit of leftovers, −5 per intervention |

## Fairness checklist

- [ ] Same prompt, word for word.
- [ ] Every run in a fresh clone without `origin`.
- [ ] Same harness settings and time limit for all, web tools off.
- [ ] No folder on the run machine has `refactor-tests` with the kit until all runs are done.
- [ ] Run details logged in `runs.csv` right after each run.
- [ ] If several models trip over the same unclear point in `TASKS.md`, tell me. I'll count it as a problem with the spec, not with the models.
- [ ] Want Claude in the table? Run Claude Code exactly like the others: fresh clone, same prompt, same limits. The reference patches here don't count.

## Redoing or cleaning up

- **Discard a run:**
  ```bash
  git push https://github.com/georgs-tumans/price-tracker-bot.git --delete bench/<model>/<task>-<run>
  rm -rf ~/bench-runs/<model>-<task>-<run>
  ```
  Rerun with the next run number.
- **When you're done:** the `bench/*` branches can stay on GitHub or be deleted the same way. Nothing else in the repo changes.
- **Later models:** a model released after the kit was pushed may have seen the hidden tests and reference patches in its training data. To compare such models fairly, write new tasks.
