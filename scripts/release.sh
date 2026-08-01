#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

COVER_HISTORY=".cover/history.json"
COVER_PROFILE="coverage.out"
EVENTS_FILE=".roady/events.jsonl"
mkdir -p $(dirname "$COVER_HISTORY")

# Governance events are NOT appended here.
#
# This script used to write raw JSON straight into .roady/events.jsonl. That
# log is hash-chained: every entry commits to its predecessor, which is what
# makes it tamper-evident. Entries appended by hand carry no hash and no
# prev_hash, so they sit outside the chain and `roady audit verify` reports
# them — indistinguishable, at a glance, from actual tampering. Twelve such
# entries accumulated in this repository before anyone noticed.
#
# Anything that needs to be in the audit log must go through roady, which
# chains it. A release is already recorded by its git tag and its GitHub
# release; it does not need a second, weaker record here.
log_event() {
  : # intentionally a no-op; see above
}

echo "=== Roady Release Pipeline ==="
echo ""

# Check dependencies
if ! command -v coverctl >/dev/null 2>&1; then
  echo "coverctl is required" >&2
  exit 1
fi
if ! command -v relicta >/dev/null 2>&1; then
  echo "relicta is required" >&2
  exit 1
fi

# Log release started
log_event "release.started" "release-script" "{\"stage\":\"init\"}"

echo "Step 1: Coverage check..."
coverctl record --history "$COVER_HISTORY" -p "$COVER_PROFILE"
coverctl check --ratchet --history "$COVER_HISTORY" -p "$COVER_PROFILE"
log_event "release.coverage_passed" "coverctl" "{\"profile\":\"$COVER_PROFILE\"}"

echo ""
echo "Step 2: Running tests..."
go test ./...
log_event "release.tests_passed" "go-test" "{}"

echo ""
echo "Step 3: Regenerating plan..."
./roady plan generate --ai 2>/dev/null || ./cmd/roady/main.go plan generate --ai 2>/dev/null || true
log_event "release.plan_generated" "roady" "{}"

echo ""
echo "Step 4: Version bump..."
relicta bump
VERSION=$(relicta status 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
log_event "release.version_bumped" "relicta" "{\"version\":\"$VERSION\"}"

echo ""
echo "Step 5: Generating release notes..."
relicta notes
log_event "release.notes_generated" "relicta" "{}"

echo ""
echo "Step 6: Approving release..."
relicta approve
log_event "release.approved" "release-script" "{\"version\":\"$VERSION\"}"

echo ""
echo "Step 7: Publishing release..."
relicta publish
log_event "release.published" "relicta" "{\"version\":\"$VERSION\"}"

echo ""
echo "=== Release Complete ==="
log_event "release.completed" "release-script" "{\"version\":\"$VERSION\"}"
