# Benchmark scores

- Base: `develop` (e86ccd3)
- Race detector: **off** (no C compiler found; see RUNBOOK.md)
- golangci-lint: v2.13.2
- Hidden tests: each run 3 times; a test counts only if it passed every time

| Branch | Task | Build | Vet | Existing tests | Hidden | Lint issues | Removed names left | Test edits not allowed | Allowed test edits | Protected files | CRLF files | New test funcs | Lines | Files | Report | Kit seen | Commits |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| self-test/base-t1 | T1 | pass | pass | pass | 0/3 (build failed) | 0 | 10 | none | none | none | none | 0 | +0 −0 | 0 | no | no | - |
| self-test/reference-t1 | T1 | pass | pass | pass | 3/3 | 0 | 0 | none | none | none | none | 1 | +64 −31 | 4 | no | no | - |
| self-test/base-t2 | T2 | pass | pass | pass | 0/7 (build failed) | 0 | 25 | none | none | none | none | 0 | +0 −0 | 0 | no | no | - |
| self-test/reference-t2 | T2 | pass | pass | pass | 7/7 | 0 | 0 | none | botfixer/bot_fixer_test.go config/config_test.go handlers/tracker_test.go  | none | none | 1 | +99 −81 | 7 | no | no | - |
| self-test/base-t3 | T3 | pass | pass | pass | 0/7 (build failed) | 0 | 6 | none | none | none | none | 0 | +0 −0 | 0 | no | no | - |
| self-test/reference-t3 | T3 | pass | pass | pass | 7/7 | 0 | 0 | none | botfixer/bot_fixer_test.go  | none | none | 0 | +298 −200 | 5 | no | no | - |
