#!/usr/bin/env bash
# Checks the real adapter against the live TSETMC. Run it from a network that
# can reach cdn.tsetmc.com (Iran, or a VPN):
#
#   bash backend/scripts/verify_live.sh
#
# 1. runs cmd/fetchdemo (the same Go client and headers the app uses) and checks
#    the counts and that the two enrichment requests worked;
# 2. checks the one thing nobody could verify on a trading day: on a closed day
#    (Thursday/Friday or a holiday) the session date from GetClosingPriceInfo
#    must stay at the LAST trading day, not turn into today.
# Exit code 0 = all good, 1 = something is wrong, 2 = could not run.
set -u
cd "$(dirname "$0")/.." || exit 2
fail=0
say() { printf '%s\n' "$*"; }
bad() { say "FAIL: $*"; fail=1; }

say "== 1. fetchdemo against the live source"
out=$(go run ./cmd/fetchdemo 2>&1) || { say "$out"; say "FAIL: fetchdemo did not run"; exit 2; }
say "$out"
contracts=$(sed -n 's/^contracts: *\([0-9]*\).*/\1/p' <<<"$out" | head -1)
withtime=$(sed -n 's/^contracts with a last trade time: *\([0-9]*\).*/\1/p' <<<"$out" | head -1)
[ -n "$contracts" ] && [ "$contracts" -ge 1500 ] && [ "$contracts" -le 2500 ] \
  && say "ok: $contracts contracts (expected 1500-2500)" || bad "contracts=$contracts is outside 1500-2500"
grep -q "enrichment failed" <<<"$out" && bad "enrichment failed: the market watch or closing-info request was rejected" \
  || say "ok: both enrichment requests worked"

say; say "== 2. trade date on a closed day"
today=$(TZ=Asia/Tehran date +%Y%m%d)
dow=$(TZ=Asia/Tehran date +%u) # 1=Mon ... 4=Thu 5=Fri
info=$(curl -sS --compressed -m 30 \
  -H 'User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36' \
  -H 'Referer: https://www.tsetmc.com/' -H 'Origin: https://www.tsetmc.com' \
  https://cdn.tsetmc.com/api/ClosingPrice/GetClosingPriceInfo/17914401175772326) || { bad "could not fetch closing info"; exit 1; }
dEven=$(python3 -I -c 'import json,sys; print(json.load(sys.stdin)["closingPriceInfo"]["dEven"])' <<<"$info")
say "today (Tehran) = $today, weekday = $dow, session date dEven = $dEven"
if [ "$dow" = 4 ] || [ "$dow" = 5 ]; then
  if [ "$dEven" = "$today" ]; then
    bad "dEven equals today on a Thursday/Friday. The trade-date assumption is WRONG; the snapshot's identical-figures guard still protects it, but tell the maintainers."
  else
    say "ok: on a closed day dEven ($dEven) stays at the last session, not today"
  fi
else
  say "note: today is a normal trading weekday, so this check says nothing. Run it again on a Thursday or Friday (or a public holiday)."
fi

[ "$fail" = 0 ] && say && say "ALL CHECKS PASSED" || { say; say "SOME CHECKS FAILED"; }
exit "$fail"
