# Sourced by demo.tape, hidden, inside the VHS container: makes a throwaway
# home directory with sessions run by the fake claude from test/e2e, so the demo
# never touches real sessions and comes out the same every time.
# shellcheck shell=bash

demo=/tmp/demo
rm -rf "$demo"
mkdir -p "$demo/bin" "$demo/claude/jobs" "$demo/state" "$demo/config"
cp /vhs/docs/demo/bin/claudes /vhs/docs/demo/bin/claude "$demo/bin/"

export HOME=$demo PATH=$demo/bin:$PATH
export CLAUDE_CONFIG_DIR=$demo/claude XDG_STATE_HOME=$demo/state XDG_CONFIG_HOME=$demo/config
export FAKE_CLAUDE_STATE=$demo/fake.json FAKE_CLAUDE_LOG=$demo/fake.log
unset CLAUDE_CODE_SESSION_ID CLAUDE_JOB_DIR CLAUDECODE

now=$(date +%s)
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

cat >"$FAKE_CLAUDE_STATE" <<EOF
{"version":"2.1.325","nextPid":9100000,"sessions":[
$(session api-refactor 3f2a9c41 busy 95),
$(session docs-site 8b7e1d02 idle 260),
$(session release-notes c41d7e93 busy 40),
$(session flaky-tests 5e9b2a64 idle 15),
$(session db-migration 72ac5f15 - 1800)
]}
EOF
printf '[\n  "8b7e1d02"\n]' >"$CLAUDE_CONFIG_DIR/jobs/pins.json"
for id in 3f2a9c41 8b7e1d02 c41d7e93 5e9b2a64 72ac5f15; do
  mkdir -p "$CLAUDE_CONFIG_DIR/jobs/$id"
  printf '{"cliVersion":"2.1.325"}' >"$CLAUDE_CONFIG_DIR/jobs/$id/state.json"
done
printf '{"cliVersion":"2.1.322"}' >"$CLAUDE_CONFIG_DIR/jobs/c41d7e93/state.json"

# transcript NAME ID DIR KB DAYS_AGO writes a transcript of about KB
# kilobytes, last active DAYS_AGO days ago. An empty NAME writes a session that
# never got a prompt.
pad=$(head -c 1000 /dev/zero | tr '\0' 'x')
transcript() {
  local sid="$2-4a1b-4c2d-8e3f-${2}0000" dir path
  dir=$CLAUDE_CONFIG_DIR/projects/$(printf '%s' "$3" | tr -c 'A-Za-z0-9' '-')
  path=$dir/$sid.jsonl
  mkdir -p "$dir" "$3"
  {
    if [[ -n $1 ]]; then
      printf '{"type":"user","sessionId":"%s","cwd":"%s","gitBranch":"main","version":"2.1.325"}\n' "$sid" "$3"
      printf '{"type":"custom-title","customTitle":"%s","sessionId":"%s"}\n' "$1" "$sid"
      for _ in $(seq "$4"); do printf '{"type":"assistant","message":{"content":"%s"}}\n' "$pad"; done
    else
      printf '{"type":"system","sessionId":"%s","cwd":"%s"}\n' "$sid" "$3"
    fi
  } >"$path"
  touch -d "$5 days ago" "$path"
}
transcript api-refactor 3f2a9c41 "$demo/src/api-refactor" 9000 0
transcript docs-site 8b7e1d02 "$demo/src/docs-site" 2600 0
transcript release-notes c41d7e93 "$demo/src/release-notes" 4100 0
transcript flaky-tests 5e9b2a64 "$demo/src/flaky-tests" 700 0
transcript db-migration 72ac5f15 "$demo/src/db-migration" 12000 2
transcript spike-auth 1d4e8b26 "$demo/src/spike-auth" 31000 40
transcript spike-auth 1d4e8b26 "$demo/src/old-auth" 30000 45
transcript "" 9a0c3e37 "$demo/src/api-refactor" 0 6
transcript "" 6f1d2b48 "$demo/src/docs-site" 0 9
# Subagent transcripts and file history, so those columns have something.
sidecar=$CLAUDE_CONFIG_DIR/projects/-tmp-demo-src-api-refactor/3f2a9c41-4a1b-4c2d-8e3f-3f2a9c410000/subagents
mkdir -p "$sidecar" "$CLAUDE_CONFIG_DIR/file-history/8b7e1d02-4a1b-4c2d-8e3f-8b7e1d020000"
head -c 5200000 /dev/zero >"$sidecar/agent-1.jsonl"
head -c 1400000 /dev/zero >"$CLAUDE_CONFIG_DIR/file-history/8b7e1d02-4a1b-4c2d-8e3f-8b7e1d020000/index.md@v1"
mkdir -p "$CLAUDE_CONFIG_DIR/projects/-tmp-demo-src-docs-site/0b3e9f59-4a1b-4c2d-8e3f-0b3e9f590000/tool-results"
head -c 3000000 /dev/zero >"$CLAUDE_CONFIG_DIR/projects/-tmp-demo-src-docs-site/0b3e9f59-4a1b-4c2d-8e3f-0b3e9f590000/tool-results/r.txt"
touch -d "20 days ago" "$CLAUDE_CONFIG_DIR/projects/-tmp-demo-src-docs-site/0b3e9f59-4a1b-4c2d-8e3f-0b3e9f590000" \
  "$CLAUDE_CONFIG_DIR/projects/-tmp-demo-src-docs-site/0b3e9f59-4a1b-4c2d-8e3f-0b3e9f590000/tool-results" \
  "$CLAUDE_CONFIG_DIR/projects/-tmp-demo-src-docs-site/0b3e9f59-4a1b-4c2d-8e3f-0b3e9f590000/tool-results/r.txt"

cd "$demo/src/api-refactor" || return
PS1='\[\e[1;35m\]❯\[\e[0m\] '
