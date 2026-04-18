#!/usr/bin/env bash
# seq-logs.sh — query Seq structured logs for Polvo dev debugging
#
# Usage:
#   ./scripts/seq-logs.sh [options]
#
# Options:
#   -s, --service <name>     Filter by service: polvo-tui | polvo-server (default: all)
#   -l, --level <level>      Min level: Debug | Information | Warning | Error (default: Debug)
#   -e, --event <name>       Filter by event name, e.g. llm_request, tool_call, agent_stuck
#   -S, --session <id>       Filter by session ID
#   -m, --model <name>       Filter by model name
#   -t, --tool <name>        Filter by tool name (for tool_call/tool_result events)
#   -n, --count <n>          Number of events to fetch (default: 30)
#   -f, --from <datetime>    Start time, e.g. "2026-04-17T16:00:00" (default: last 30min)
#   -F, --filter <expr>      Raw Seq filter expression (overrides other filters)
#   -j, --json               Output raw JSON instead of formatted table
#   -w, --watch              Stream events continuously (live tail)
#   -u, --url <url>          Seq URL (default: $SEQ_URL or http://localhost:5341)
#   -h, --help               Show this help

set -euo pipefail

SEQ="${SEQ_URL:-http://localhost:5341}"
COUNT=30
SERVICE=""
LEVEL="Debug"
EVENT=""
SESSION=""
MODEL=""
TOOL=""
FROM=""
FILTER=""
JSON_OUTPUT=false
WATCH=false

usage() {
  sed -n '/^# Usage:/,/^[^#]/{ /^[^#]/d; s/^# \{0,3\}//; p }' "$0"
  exit 0
}

# ── arg parsing ──────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    -s|--service)  SERVICE="$2";  shift 2 ;;
    -l|--level)    LEVEL="$2";    shift 2 ;;
    -e|--event)    EVENT="$2";    shift 2 ;;
    -S|--session)  SESSION="$2";  shift 2 ;;
    -m|--model)    MODEL="$2";    shift 2 ;;
    -t|--tool)     TOOL="$2";     shift 2 ;;
    -n|--count)    COUNT="$2";    shift 2 ;;
    -f|--from)     FROM="$2";     shift 2 ;;
    -F|--filter)   FILTER="$2";   shift 2 ;;
    -j|--json)     JSON_OUTPUT=true; shift ;;
    -w|--watch)    WATCH=true;    shift ;;
    -u|--url)      SEQ="$2";      shift 2 ;;
    -h|--help)     usage ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

# ── build filter expression ──────────────────────────────────────────────────
if [[ -z "$FILTER" ]]; then
  parts=()

  [[ -n "$SERVICE" ]]  && parts+=("service == '${SERVICE}'")
  [[ -n "$EVENT" ]]    && parts+=("@mt == '${EVENT}'")
  [[ -n "$SESSION" ]]  && parts+=("session == '${SESSION}'")
  [[ -n "$MODEL" ]]    && parts+=("model like '%${MODEL}%'")
  [[ -n "$TOOL" ]]     && parts+=("tool == '${TOOL}'")

  # level mapping
  case "$LEVEL" in
    Debug|debug)       ;;  # no filter needed, Seq returns all levels
    Information|info)  parts+=("@Level in ['Information','Warning','Error']") ;;
    Warning|warn)      parts+=("@Level in ['Warning','Error']") ;;
    Error|error)       parts+=("@Level == 'Error'") ;;
  esac

  if [[ ${#parts[@]} -gt 0 ]]; then
    FILTER=$(IFS=" and "; echo "${parts[*]}")
  fi
fi

# ── default from: last 30 min ────────────────────────────────────────────────
if [[ -z "$FROM" ]]; then
  if date --version &>/dev/null 2>&1; then
    FROM=$(date -u --date="30 minutes ago" +"%Y-%m-%dT%H:%M:%S" 2>/dev/null || \
           python3 -c "from datetime import datetime,timedelta,timezone; print((datetime.now(timezone.utc)-timedelta(minutes=30)).strftime('%Y-%m-%dT%H:%M:%S'))")
  else
    # macOS
    FROM=$(python3 -c "from datetime import datetime,timedelta,timezone; print((datetime.now(timezone.utc)-timedelta(minutes=30)).strftime('%Y-%m-%dT%H:%M:%S'))")
  fi
fi

# ── build URL ────────────────────────────────────────────────────────────────
if $WATCH; then
  ENDPOINT="${SEQ}/api/events/stream"
  PARAMS="render=true"
  [[ -n "$FILTER" ]] && PARAMS+="&filter=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$FILTER")"
  echo "► Streaming from ${SEQ} | filter: ${FILTER:-none}" >&2
  echo "  Ctrl+C to stop" >&2
  echo "" >&2
  curl -sN "${ENDPOINT}?${PARAMS}" | python3 -u -c "
import sys, json
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        e = json.loads(line)
        ts   = e.get('Timestamp','')[:19]
        lvl  = e.get('Level','Info')[:4].upper()
        msg  = e.get('RenderedMessage') or ''.join(t.get('Text','') + t.get('PropertyName','') for t in e.get('MessageTemplateTokens',[]))
        props = {p['Name']: p['Value'] for p in e.get('Properties',[])}
        svc  = props.pop('service', '?')
        exc  = e.get('Exception','')
        line_out = f'[{ts}] [{lvl}] [{svc}] {msg}'
        if props:
            kv = '  '.join(f'{k}={v}' for k,v in props.items() if k not in ('service',))
            line_out += f'  |  {kv}'
        if exc:
            line_out += f'\n    !! {exc}'
        print(line_out, flush=True)
    except Exception:
        print(line, flush=True)
"
  exit 0
fi

ENDPOINT="${SEQ}/api/events"
PARAMS="count=${COUNT}&render=true&fromDateUtc=${FROM}"
[[ -n "$FILTER" ]] && PARAMS+="&filter=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$FILTER")"

# ── fetch & display ───────────────────────────────────────────────────────────
RESPONSE=$(curl -sf "${ENDPOINT}?${PARAMS}" || { echo "ERROR: could not reach Seq at ${SEQ}" >&2; exit 1; })

if $JSON_OUTPUT; then
  echo "$RESPONSE" | python3 -m json.tool
  exit 0
fi

echo "$RESPONSE" | python3 -c "
import json, sys

COLORS = {
    'DEBU': '\033[36m',   # cyan
    'INFO': '\033[32m',   # green
    'WARN': '\033[33m',   # yellow
    'ERRO': '\033[31m',   # red
    'RESET': '\033[0m',
}

events = json.load(sys.stdin)
if not events:
    print('  (no events found)')
    sys.exit(0)

# reverse to chronological order
events = list(reversed(events))

for e in events:
    ts    = e.get('Timestamp','')[:19].replace('T',' ')
    lvl   = e.get('Level','Information')[:4].upper()
    msg   = e.get('RenderedMessage') or ''.join(t.get('Text','') + t.get('PropertyName','') for t in e.get('MessageTemplateTokens',[]))
    props = {p['Name']: p['Value'] for p in e.get('Properties',[])}
    svc   = props.pop('service', '?')
    exc   = e.get('Exception','')

    color = COLORS.get(lvl, '')
    reset = COLORS['RESET']

    # skip noisy props already in the message template
    skip = {'service'}
    kv = '  '.join(f'{k}=\033[97m{v}{reset}' for k,v in props.items() if k not in skip)

    print(f'{color}[{ts}] [{lvl}] [{svc}]{reset}  {msg}')
    if kv:
        print(f'   {kv}')
    if exc:
        print(f'   \033[31m!! {exc}{reset}')
    print()
"
