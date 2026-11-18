# Sourced by demo.tape, hidden, inside the VHS container: makes a throwaway
# home directory with sessions run by the fake claude from test/e2e, so the demo
# never touches real sessions and comes out the same every time.
# shellcheck shell=bash

demo=/home/demo
rm -rf "$demo"
mkdir -p "$demo/bin" "$demo/.claude/jobs"
cp /vhs/docs/demo/bin/claudes /vhs/docs/demo/bin/claude "$demo/bin/"

export HOME=$demo PATH=$demo/bin:$PATH
export CLAUDE_CONFIG_DIR=$demo/.claude XDG_STATE_HOME=$demo/.local/state XDG_CONFIG_HOME=$demo/.config XDG_CACHE_HOME=$demo/.cache
export FAKE_CLAUDE_STATE=$demo/fake.json FAKE_CLAUDE_LOG=$demo/fake.log
unset CLAUDE_CODE_SESSION_ID CLAUDE_JOB_DIR CLAUDECODE

now=$(date +%s) day=1440
# The midnights that start -l today and -l week, which begins on Monday.
today=$(date -d 'today 00:00' +%s)
week=$((today - ($(date +%u) - 1) * 86400))
installed=2.1.325 old=2.1.322

# session NAME ID STATUS MINUTES_AGO prints one agent view entry; "-" for
# STATUS is a stopped session.
session() {
  local pid=$((9000000 + RANDOM)) state=working status=$3
  case $3 in
  idle) state="done" ;;
  -) pid=0 state=stopped status= ;;
  esac
  mkdir -p "$demo/src/$1"
  printf '{"id":"%s","sessionId":"%s-4a1b-4c2d-8e3f-%s","name":"%s","cwd":"%s","kind":"background","state":"%s","status":"%s","pid":%d,"startedAt":%d}' \
    "$2" "$2" "${2}0000" "$1" "$demo/src/$1" "$state" "$status" "$pid" $(((now - $4 * 60) * 1000))
}

# The installed Claude Code is $installed, so the sessions on $old show (old).
cat >"$FAKE_CLAUDE_STATE" <<EOF
{"version":"$installed","nextPid":9100000,"sessions":[
$(session api-refactor 3f2a9c41 busy 95),
$(session docs 8b7e1d02 idle 260),
$(session release-notes c41d7e93 idle 40),
$(session flaky-tests 5e9b2a64 busy 15),
$(session db-migration 72ac5f15 - 1800)
]}
EOF
printf '[\n  "c41d7e93"\n]' >"$CLAUDE_CONFIG_DIR/jobs/pins.json"
# job ID VERSION SUBAGENTS writes a job's state.json with SUBAGENTS running.
job() {
  local fan=() i
  for ((i = 0; i < $3; i++)); do
    fan+=("{\"id\":\"a$(printf '%016x' "$i")\",\"kind\":\"agent\",\"label\":\"subagent\",\"startedAt\":1}")
  done
  mkdir -p "$CLAUDE_CONFIG_DIR/jobs/$1"
  printf '{"cliVersion":"%s","inFlight":{"tasks":%d,"kinds":[]},"fan":[%s]}' "$2" "$3" "$(
    IFS=,
    echo "${fan[*]}"
  )" >"$CLAUDE_CONFIG_DIR/jobs/$1/state.json"
}
job 3f2a9c41 $old 3
job 8b7e1d02 $old 0
job c41d7e93 $old 0
job 5e9b2a64 $installed 0
job 72ac5f15 $installed 0

# transcript ID DIR MINUTES_AGO RECORD... writes the transcript of session ID,
# run in ~/src/DIR and last active MINUTES_AGO. A transcript under a day old
# stays within today, and one with floor=week stays within this week, so -l
# finds them whatever the time the demo renders. Each RECORD is a pair:
#
#   title NAME  a name given with /rename
#   ai NAME     a name Claude Code gave it
#   you TEXT    a prompt
#   claude TEXT a reply
#   tool NAME   a tool call
#   pad N       N replies of about a kilobyte each
#
# No RECORD writes a session that never got a prompt.
pad=$(head -c 1000 /dev/zero | tr '\0' 'x')
# project DIR is where Claude Code keeps the transcripts of ~/src/DIR.
project() { printf '%s/projects/%s' "$CLAUDE_CONFIG_DIR" "$(printf '%s' "$demo/src/$1" | tr -c 'A-Za-z0-9' '-')"; }
transcript() {
  local sid="$1-4a1b-4c2d-8e3f-${1}0000" cwd=$demo/src/$2 at=$((now - $3 * 60)) path i
  path=$(project "$2")/$sid.jsonl
  (($3 < day && at < today)) && at=$((today + 60))
  [[ ${floor-} == week ]] && ((at < week)) && at=$((week + 60))
  shift 3
  mkdir -p "$cwd" "${path%/*}"
  {
    printf '{"type":"system","sessionId":"%s","cwd":"%s"}\n' "$sid" "$cwd"
    while (($# >= 2)); do
      case $1 in
      title) printf '{"type":"custom-title","customTitle":"%s","sessionId":"%s"}\n' "$2" "$sid" ;;
      ai) printf '{"type":"ai-title","aiTitle":"%s","sessionId":"%s"}\n' "$2" "$sid" ;;
      you) printf '{"type":"user","sessionId":"%s","cwd":"%s","gitBranch":"main","version":"%s","message":{"role":"user","content":"%s"}}\n' "$sid" "$cwd" "$old" "$2" ;;
      claude) printf '{"type":"assistant","sessionId":"%s","message":{"role":"assistant","content":[{"type":"text","text":"%s"}]}}\n' "$sid" "$2" ;;
      tool) printf '{"type":"assistant","sessionId":"%s","message":{"role":"assistant","content":[{"type":"tool_use","id":"t","name":"%s","input":{}}]}}\n' "$sid" "$2" ;;
      pad) for ((i = 0; i < $2; i++)); do printf '{"type":"assistant","message":{"content":"%s"}}\n' "$pad"; done ;;
      esac
      shift 2
    done
  } >>"$path"
  touch -d "@$at" "$path"
}

# The running sessions, and the stopped db-migration.
transcript 3f2a9c41 api-refactor 1 title api-refactor \
  you "Split the session store out of the cli package and keep the tests green" tool Read tool Edit pad 4200
transcript 8b7e1d02 docs 3 title docs you "Rewrite the install guide for Windows" tool Edit pad 2600
transcript c41d7e93 release-notes 12 title release-notes you "Draft the notes for 0.7.0 from the merged pull requests" tool Bash pad 1900
transcript 5e9b2a64 flaky-tests 1 title flaky-tests you "TestPark fails one run in 50 on Windows. Find out why" tool Bash pad 700
transcript 72ac5f15 db-migration 190 title db-migration you "Write the migration that adds the sessions index" tool Write pad 3100

# Stopped today, run in a terminal rather than in the background.
transcript 2c8f4a17 billing 320 ai bump-deps you "Bump the AWS SDK and fix what breaks" tool Edit tool Bash pad 900

# Found by a search for a lock race this week.
floor=week transcript a1b2c3d4 api $day title cache-fix \
  you "The cache test hangs. I think it is a lock race in the writer" \
  claude "The writer takes the lock twice when the cache fills" tool Edit pad 1300
floor=week transcript b2c3d4e5 ingest $((2 * day)) ai ingest-perf \
  you "Why does ingest slow down under load" \
  claude "The lock is held across the network call. A race shows up only under load, when two batches flush at once" \
  tool Edit pad 800
# Too old for -l week.
transcript 4e7a9b51 api $((12 * day)) ai queue-worker you "Is there a lock race in the queue worker" tool Read pad 300

# For triage to sort: one prompt and no tool call to remove, one prompt and a
# few reads to review, more than one prompt to keep, and sessions that never
# got a prompt.
transcript 7d2e6f80 docs $((5 * day)) ai bg-flag you "What does --bg do"
transcript 3b9c1e62 docs $((4 * day)) ai readme-tone you "Is the README's opening too long" tool Read tool Read tool Grep
transcript 8e4f2a93 billing $((3 * day)) ai ci-windows you "Why did CI fail on Windows last night" tool Bash tool Read tool Bash tool Read
transcript 5a6b7c8d billing $((6 * day)) ai invoice-totals you "Sum the invoice totals by month" you "Now by customer" tool Bash pad 200
transcript 9a0c3e37 api-refactor $((6 * day))
transcript 6f1d2b48 docs $((9 * day))
# Too big to read for triage, and the older copy session mv left of it.
transcript 1d4e8b26 spike-auth $((40 * day)) title spike-auth you "Try OAuth device flow for the CLI" tool Edit pad 2600
transcript 1d4e8b26 old-auth $((45 * day)) title spike-auth you "Try OAuth device flow for the CLI" tool Edit pad 2500

# Subagent transcripts and file history, and tool results whose transcript is
# gone, which session gc takes as a leftover.
sidecar=$(project api-refactor)/3f2a9c41-4a1b-4c2d-8e3f-3f2a9c410000/subagents
mkdir -p "$sidecar" "$CLAUDE_CONFIG_DIR/file-history/8b7e1d02-4a1b-4c2d-8e3f-8b7e1d020000"
head -c 1200000 /dev/zero >"$sidecar/agent-1.jsonl"
head -c 400000 /dev/zero >"$CLAUDE_CONFIG_DIR/file-history/8b7e1d02-4a1b-4c2d-8e3f-8b7e1d020000/index.md@v1"
leftover=$(project docs)/0b3e9f59-4a1b-4c2d-8e3f-0b3e9f590000
mkdir -p "$leftover/tool-results"
head -c 3000000 /dev/zero >"$leftover/tool-results/r.txt"
touch -d "20 days ago" "$leftover" "$leftover/tool-results" "$leftover/tool-results/r.txt"

cd "$demo/src/api-refactor" || return
PS1='\[\e[1;35m\]❯\[\e[0m\] '
