#!/usr/bin/env bash
# Scores model benchmark branches (bench/<model>/t<n>-r<run>) against the hidden tests and scope rules.
# See RUNBOOK.md. Works in Git Bash on Windows and on Linux; needs git, go and golangci-lint on PATH.
set -uo pipefail

KIT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(git -C "$KIT/.." rev-parse --show-toplevel)"
LINT_VERSION="2.13.2" # Must match .github/workflows/golangci-lint.yml
HIDDEN_RUNS=3         # Hidden tests run this many times; a test only counts as passed if it passed every run

BASE=""
FETCH=0
SELF_TEST=0
BRANCHES=()

usage() {
	cat <<'EOF'
Usage: score.sh [--fetch] [--base <ref>] [branch ...]
       score.sh --self-test [--base <ref>]

  --fetch      fetch bench-base and all bench/* branches from origin first
  --base       the commit the models started from (default: bench-base, then origin/bench-base)
  --self-test  score the untouched base and the reference patches, to check the kit itself
  branch ...   branches to score (default: every bench/* branch, local and origin)
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--fetch) FETCH=1 ;;
	--base)
		BASE="$2"
		shift
		;;
	--self-test) SELF_TEST=1 ;;
	-h | --help)
		usage
		exit 0
		;;
	*) BRANCHES+=("$1") ;;
	esac
	shift
done

cd "$REPO" || exit 1

if [ "$FETCH" = 1 ]; then
	git fetch origin '+refs/heads/bench-base:refs/remotes/origin/bench-base' '+refs/heads/bench/*:refs/remotes/origin/bench/*' || exit 1
fi

if [ -z "$BASE" ]; then
	for candidate in bench-base origin/bench-base; do
		if git rev-parse --verify --quiet "$candidate^{commit}" >/dev/null; then
			BASE="$candidate"
			break
		fi
	done
fi

if [ -z "$BASE" ] || ! BASE_SHA="$(git rev-parse --verify --quiet "$BASE^{commit}")"; then
	echo "No base commit found; create bench-base (RUNBOOK.md step 1) or pass --base <ref>." >&2
	exit 1
fi

# The race detector needs cgo and a C compiler; without one the tests run without -race and the summary says so.
RACE_FLAG=""
if command -v "$(go env CC)" >/dev/null 2>&1; then
	export CGO_ENABLED=1
	RACE_FLAG="-race"
fi

LINT_OK=0
if golangci-lint --version 2>/dev/null | grep -q "version $LINT_VERSION "; then
	LINT_OK=1
fi

RESULTS="$KIT/results/$(date +%Y%m%d-%H%M%S)"
WT_ROOT="$(mktemp -d)"
mkdir -p "$RESULTS"
SUMMARY="$RESULTS/summary.md"
TSV="$RESULTS/summary.tsv"

cleanup() {
	for wt in "$WT_ROOT"/*; do
		[ -d "$wt" ] && git worktree remove --force "$wt" >/dev/null 2>&1
	done

	rm -rf "$WT_ROOT"
	git worktree prune
}
trap cleanup EXIT

# Per task: removed names that must be gone (extended regex over *.go), and existing test files the task may edit.
removed_names() {
	case "$1" in
	1) echo '\b(New)?(API|Scraper)TrackerBehavior\b' ;;
	2) echo '\b(DetermineTrackerType|GetAPITrackerData|GetScraperTrackerData|APITrackers|ScraperTrackers)\b|^[[:space:]]*(API|Scraper)[[:space:]]*=' ;;
	3) echo 'func \(ch \*CommandHandler\) (ResumeTrackers|StopAllTrackers|GetActiveTracker|AddRunningTracker|RemoveRunningTracker|saveState)\b' ;;
	esac
}

allowed_test_edits() {
	case "$1" in
	1) echo "" ;;
	2) echo "handlers/tracker_test.go botfixer/bot_fixer_test.go config/config_test.go" ;;
	3) echo "botfixer/bot_fixer_test.go" ;;
	esac
}

# Files no task may touch; .golangci.yml is listed so that lint can't be passed by disabling linters.
PROTECTED='^(TASKS\.md|AGENTS\.md|CLAUDE\.md|\.golangci\.yml|go\.mod|go\.sum|deployment/|\.github/)'

pass_fail() { if [ "$1" = 0 ]; then echo "pass"; else echo "FAIL"; fi; }

# Scores the checked-out tree in $2 for task $3 and appends a summary row. $4 is the branch, empty for self-test rows.
score() {
	local label="$1" wt="$2" task="$3" branch="$4"
	local out="$RESULTS/${label//\//__}"
	mkdir -p "$out"
	echo "== $label (T$task)"

	# --- Scope, from the diff against the base (BENCH_REPORT.md doesn't count as a change)
	# autocrlf off, so that line endings are compared exactly as committed
	local g=(git -C "$wt" -c core.autocrlf=false)
	"${g[@]}" add -A >/dev/null 2>&1
	"${g[@]}" diff --cached "$BASE_SHA" >"$out/diff.patch"
	local status_list
	status_list="$("${g[@]}" diff --cached --name-status "$BASE_SHA" -- . ':(exclude)BENCH_REPORT.md')"

	local files plus minus
	files="$(printf '%s\n' "$status_list" | grep -c . )"
	read -r plus minus < <("${g[@]}" diff --cached --numstat "$BASE_SHA" -- . ':(exclude)BENCH_REPORT.md' | awk '{p+=$1; m+=$2} END {print p+0, m+0}')

	local test_edits_bad="" test_edits_ok="" protected="" crlf=""
	local allowed
	allowed=" $(allowed_test_edits "$task") "
	while IFS=$'\t' read -r st path rest; do
		[ -z "$st" ] && continue
		[ -n "$rest" ] && path="$rest" # Renames: the new path
		if [[ "$st" =~ ^[MDR] ]] && [[ "$path" == *_test.go ]] && git cat-file -e "$BASE_SHA:$path" 2>/dev/null; then
			if [[ "$allowed" == *" $path "* ]]; then test_edits_ok+="$path "; else test_edits_bad+="$path "; fi
		fi
		if [[ "$path" =~ $PROTECTED ]]; then protected+="$path "; fi
		# -U: grep on Windows strips CRs from text files without it
		if [[ "$st" != D ]] && [ -f "$wt/$path" ] && grep -qU $'\r' "$wt/$path"; then crlf+="$path "; fi
	done <<<"$status_list"

	local new_tests
	new_tests="$("${g[@]}" diff --cached "$BASE_SHA" -- '*_test.go' | grep -cE '^\+func Test')"

	local leftovers
	leftovers="$(grep -rnE --include='*.go' "$(removed_names "$task")" "$wt" | sed "s|$wt/||")"
	printf '%s\n' "$leftovers" >"$out/removed-names-left.txt"

	# Names that exist only in this kit; finding one on a branch means the model saw the kit during its run
	local kit_seen
	kit_seen="$(grep -rlE --exclude=.git --exclude-dir=.git 'TestBenchT[123]|benchT[123]|_model-bench|hidden-tests' "$wt" | sed "s|$wt/||" | tr '\n' ' ')"

	local report="no"
	if [ -f "$wt/BENCH_REPORT.md" ]; then
		report="yes"
		cp "$wt/BENCH_REPORT.md" "$out/BENCH_REPORT.md"
	fi

	local commits="-"
	if [ -n "$branch" ]; then
		git -C "$REPO" log --format='%h %s' "$BASE_SHA..$branch" >"$out/commits.txt"
		local total bench operator
		total="$(grep -c . "$out/commits.txt")"
		bench="$(grep -cE '^[0-9a-f]+ bench: T' "$out/commits.txt")"
		operator="$(grep -c 'uncommitted leftovers' "$out/commits.txt")"
		commits="$total ($bench bench:"
		[ "$operator" -gt 0 ] && commits+=", operator commit"
		commits+=")"
	fi

	# --- Build, vet, existing tests and lint, before the hidden tests are added
	(cd "$wt" && go build ./...) >"$out/build.log" 2>&1
	local build=$?
	(cd "$wt" && go vet ./...) >"$out/vet.log" 2>&1
	local vet=$?
	(cd "$wt" && go test -count=1 $RACE_FLAG -timeout 300s ./...) >"$out/tests.log" 2>&1
	local tests=$?

	local lint="skipped"
	if [ "$LINT_OK" = 1 ]; then
		git -C "$wt" show "$BASE_SHA:.golangci.yml" >"$wt/.golangci.yml"
		(cd "$wt" && golangci-lint run --timeout=5m ./...) >"$out/lint.log" 2>&1
		# The last line reads "0 issues." when clean and "N issues:" followed by a breakdown otherwise
		lint="$(grep -Eo '^[0-9]+ issues?[.:]' "$out/lint.log" | grep -Eo '^[0-9]+' | head -1)"
		[ -z "$lint" ] && lint="error"
	fi

	# --- Hidden tests
	cp -r "$KIT/hidden-tests/t$task/." "$wt/"
	(cd "$wt" && go test -count=$HIDDEN_RUNS $RACE_FLAG -timeout 300s -run "^TestBenchT$task" -v ./...) >"$out/hidden.log" 2>&1

	local names total_hidden passed=0 failed_names=""
	names="$(grep -rhoE "^func TestBenchT${task}[A-Za-z0-9_]+" "$KIT/hidden-tests/t$task" | sed 's/^func //')"
	total_hidden="$(printf '%s\n' "$names" | grep -c .)"
	for name in $names; do
		local passes fails
		passes="$(grep -cE "^--- PASS: $name " "$out/hidden.log")"
		fails="$(grep -cE "^--- FAIL: $name " "$out/hidden.log")"
		if [ "$passes" -eq "$HIDDEN_RUNS" ] && [ "$fails" -eq 0 ]; then
			passed=$((passed + 1))
		else
			failed_names+="$name "
		fi
	done

	local hidden="$passed/$total_hidden"
	if grep -qE '\[(build|setup) failed\]' "$out/hidden.log"; then hidden+=" (build failed)"; fi
	printf '%s\n' "$failed_names" >"$out/hidden-failed.txt"

	local left_count
	left_count="$(printf '%s\n' "$leftovers" | grep -c .)"

	printf '| %s | T%s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | +%s −%s | %s | %s | %s | %s |\n' \
		"$label" "$task" "$(pass_fail $build)" "$(pass_fail $vet)" "$(pass_fail $tests)" "$hidden" "$lint" \
		"$left_count" "${test_edits_bad:-none}" "${test_edits_ok:-none}" "${protected:-none}" "${crlf:-none}" \
		"$new_tests" "$plus" "$minus" "$files" "$report" "${kit_seen:-no}" "$commits" >>"$SUMMARY"
	printf '%s\tT%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
		"$label" "$task" "$(pass_fail $build)" "$(pass_fail $vet)" "$(pass_fail $tests)" "$passed" "$total_hidden" "$lint" \
		"$left_count" "${test_edits_bad:-none}" "${test_edits_ok:-none}" "${protected:-none}" "${crlf:-none}" \
		"$new_tests" "$plus" "$minus" "$files" "$report" "${kit_seen:-no}" >>"$TSV"
}

new_worktree() {
	local dir="$WT_ROOT/$1"
	git -c core.autocrlf=false worktree add --detach "$dir" "$2" >/dev/null 2>&1 || {
		echo "   could not check out $2" >&2
		return 1
	}
	echo "$dir"
}

{
	echo "# Benchmark scores"
	echo
	echo "- Base: \`$BASE\` ($(git rev-parse --short "$BASE_SHA"))"
	echo "- Race detector: $([ -n "$RACE_FLAG" ] && echo "on" || echo "**off** (no C compiler found; see RUNBOOK.md)")"
	echo "- golangci-lint: $([ "$LINT_OK" = 1 ] && echo "v$LINT_VERSION" || echo "**skipped** (v$LINT_VERSION not on PATH)")"
	echo "- Hidden tests: each run $HIDDEN_RUNS times; a test counts only if it passed every time"
	echo
	echo "| Branch | Task | Build | Vet | Existing tests | Hidden | Lint issues | Removed names left | Test edits not allowed | Allowed test edits | Protected files | CRLF files | New test funcs | Lines | Files | Report | Kit seen | Commits |"
	echo "|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|"
} >"$SUMMARY"
printf 'branch\ttask\tbuild\tvet\ttests\thidden_passed\thidden_total\tlint_issues\tremoved_names_left\ttest_edits_not_allowed\tallowed_test_edits\tprotected_files\tcrlf_files\tnew_test_funcs\tplus\tminus\tfiles\treport\tkit_seen\n' >"$TSV"

if [ "$SELF_TEST" = 1 ]; then
	for task in 1 2 3; do
		if wt="$(new_worktree "base-t$task" "$BASE_SHA")"; then
			score "self-test/base-t$task" "$wt" "$task" ""
		fi

		if wt="$(new_worktree "ref-t$task" "$BASE_SHA")"; then
			# autocrlf off: git apply would otherwise rewrite the patched files with CRLF
			if git -C "$wt" -c core.autocrlf=false apply "$KIT/reference/t$task.patch"; then
				score "self-test/reference-t$task" "$wt" "$task" ""
			else
				echo "   reference/t$task.patch does not apply to $BASE" >&2
			fi
		fi
	done
else
	if [ ${#BRANCHES[@]} -eq 0 ]; then
		mapfile -t BRANCHES < <(git for-each-ref --format='%(refname:short)' 'refs/heads/bench/' 'refs/remotes/origin/bench/' | sort -u)
	fi

	if [ ${#BRANCHES[@]} -eq 0 ]; then
		echo "No bench/* branches found; push them (RUNBOOK.md step 3) and use --fetch." >&2
		exit 1
	fi

	declare -A seen=()
	for branch in "${BRANCHES[@]}"; do
		label="${branch#origin/}"
		[ -n "${seen[$label]:-}" ] && continue # Local and origin copies of the same branch
		seen[$label]=1

		if ! [[ "$label" =~ /t([123])(-|$) ]]; then
			echo "Skipping $branch: no task (t1, t2 or t3) in the name" >&2
			continue
		fi

		task="${BASH_REMATCH[1]}"
		if wt="$(new_worktree "$(echo "$label" | tr '/' '_')" "$branch")"; then
			score "$label" "$wt" "$task" "$branch"
		fi
	done
fi

echo
echo "Summary: $SUMMARY"
echo "Logs per branch: $RESULTS/<branch>/"
