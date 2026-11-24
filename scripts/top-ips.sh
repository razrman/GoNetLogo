#!/usr/bin/env bash
# Print the client IPs that hit the GoNetLogo load balancer, and the
# landing-page CloudFront distribution when its log bucket exists.
# Needs the AWS CLI and python3. Profile defaults to gonetlogo.
#
#   ./scripts/top-ips.sh
#   HOURS=6 ./scripts/top-ips.sh
#   AWS_PROFILE=gonetlogo HOURS=48 ./scripts/top-ips.sh
set -euo pipefail

HOURS="${HOURS:-24}"
export AWS_PROFILE="${AWS_PROFILE:-gonetlogo}"
export AWS_REGION="${AWS_REGION:-us-west-2}"
ACCOUNT="${AWS_ACCOUNT_ID:-457504029722}"
ALB_BUCKET="${ALB_LOG_BUCKET:-gonetlogo-alb-logs-${ACCOUNT}}"
ALB_PREFIX="${ALB_LOG_PREFIX:-alb}"
CF_BUCKET="${CF_LOG_BUCKET:-sapienexmachina-cf-logs-${ACCOUNT}}"

if ! [[ "$HOURS" =~ ^[0-9]+$ ]] || [[ "$HOURS" -lt 1 ]]; then
  echo "top-ips.sh: HOURS must be a positive integer" >&2
  exit 1
fi
if ! command -v aws >/dev/null 2>&1; then
  echo "top-ips.sh: aws is not on PATH" >&2
  exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
  echo "top-ips.sh: python3 is not on PATH" >&2
  exit 1
fi

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

sync_prefix() {
  local bucket="$1" prefix="$2" dest="$3"
  if ! aws s3api head-bucket --bucket "$bucket" >/dev/null 2>&1; then
    echo "top-ips.sh: bucket ${bucket} is not readable; skipping" >&2
    return 1
  fi
  mkdir -p "$dest"
  aws s3 sync "s3://${bucket}/${prefix}" "$dest" --only-show-errors
}

echo "top-ips.sh: last ${HOURS}h from ${ALB_BUCKET} (profile ${AWS_PROFILE})" >&2
if sync_prefix "$ALB_BUCKET" "${ALB_PREFIX}/AWSLogs/${ACCOUNT}/elasticloadbalancing/${AWS_REGION}/" "$workdir/alb"; then
  :
fi
if sync_prefix "$CF_BUCKET" "AWSLogs/${ACCOUNT}/CloudFront/" "$workdir/cf"; then
  echo "top-ips.sh: including CloudFront logs in ${CF_BUCKET}" >&2
fi

python3 - "$workdir" "$HOURS" <<'PY'
import datetime, gzip, json, os, sys
from collections import defaultdict

root, hours = sys.argv[1], int(sys.argv[2])
now = datetime.datetime.now(datetime.timezone.utc)
start = now - datetime.timedelta(hours=hours)

def open_log(path):
    if path.endswith(".gz"):
        return gzip.open(path, "rt", errors="replace")
    return open(path, "r", errors="replace")

def walk(folder):
    if not os.path.isdir(folder):
        return
    for dirpath, _, names in os.walk(folder):
        for name in names:
            if name.endswith((".log", ".log.gz", ".json", ".json.gz", ".gz")):
                yield os.path.join(dirpath, name)

def alb_client(field):
    field = field.strip()
    if field.startswith("["):
        end = field.find("]")
        return field[1:end] if end > 1 else ""
    if ":" in field:
        return field.rsplit(":", 1)[0]
    return field

def remember(stats, source, ip, when):
    if not ip or when < start or when > now + datetime.timedelta(minutes=5):
        return
    row = stats[(source, ip)]
    row["count"] += 1
    row["first"] = when if row["first"] is None or when < row["first"] else row["first"]
    row["last"] = when if row["last"] is None or when > row["last"] else row["last"]

stats = defaultdict(lambda: {"count": 0, "first": None, "last": None})

for path in walk(os.path.join(root, "alb")):
    with open_log(path) as fh:
        for line in fh:
            parts = line.split()
            if len(parts) < 4:
                continue
            try:
                when = datetime.datetime.fromisoformat(parts[1].replace("Z", "+00:00"))
            except ValueError:
                continue
            remember(stats, "alb", alb_client(parts[3]), when)

for path in walk(os.path.join(root, "cf")):
    with open_log(path) as fh:
        for line in fh:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            try:
                rec = json.loads(line)
            except json.JSONDecodeError:
                continue
            ip = rec.get("c-ip") or rec.get("client-ip") or ""
            raw = rec.get("timestamp") or rec.get("date")
            when = None
            if isinstance(raw, (int, float)) and raw > 10**12:
                when = datetime.datetime.fromtimestamp(raw / 1000, datetime.timezone.utc)
            elif isinstance(raw, str):
                try:
                    when = datetime.datetime.fromisoformat(raw.replace("Z", "+00:00"))
                except ValueError:
                    date, time = rec.get("date"), rec.get("time")
                    if date and time:
                        try:
                            when = datetime.datetime.fromisoformat(f"{date}T{time}+00:00")
                        except ValueError:
                            when = None
            if when is None:
                continue
            remember(stats, "cloudfront", ip, when)

rows = sorted(stats.items(), key=lambda item: (-item[1]["count"], item[0][1]))
print(f"{'source':<12} {'ip':<40} {'requests':>8}  {'first':<20} {'last':<20}")
if not rows:
    print("(no requests in this window)")
for (source, ip), row in rows:
    first = row["first"].strftime("%Y-%m-%dT%H:%M:%SZ")
    last = row["last"].strftime("%Y-%m-%dT%H:%M:%SZ")
    print(f"{source:<12} {ip:<40} {row['count']:>8}  {first:<20} {last:<20}")
print(f"{len(rows)} addresses", file=sys.stderr)
PY
