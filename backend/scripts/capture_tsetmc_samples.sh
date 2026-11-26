#!/usr/bin/env bash
# Saves real TSETMC responses under backend/testdata/ for the adapter's tests.
# Run from a network that can reach cdn.tsetmc.com (Iran, or a VPN).
#
#   bash backend/scripts/capture_tsetmc_samples.sh
set -u

cd "$(dirname "$0")/.." || exit 1
OUT=testdata
mkdir -p "$OUT"

BASE=https://cdn.tsetmc.com/api
UA='Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36'

fetch() { # fetch <url> <outfile>
  local url=$1 file=$2
  echo "-> $url"
  if ! curl -sS --compressed -m 60 -f \
    -H "User-Agent: $UA" \
    -H 'Referer: https://www.tsetmc.com/' \
    -H 'Origin: https://www.tsetmc.com' \
    -H 'Accept: application/json, text/plain, */*' \
    -o "$file" "$url"; then
    echo "   FAILED (network blocked, timeout or HTTP error)"
    rm -f "$file"
    return 1
  fi
  echo "   saved $file ($(wc -c <"$file") bytes)"
}

# 1) Full option chain: the main sample the adapter is built from.
fetch "$BASE/Instrument/GetInstrumentOptionMarketWatch/0" "$OUT/tsetmc_option_watch.json" || exit 1

# 2) Optional extras (اخابر, from data-pipeline/output/underlyings.csv).
fetch "$BASE/Instrument/GetInstrumentInfo/22811176775480091" "$OUT/tsetmc_instrument_info.json" || true

# 3) Enrichment sources (used by FetchMarket for names, volume and trade times).
#    The market watch is ~3.7 MB; the adapter's tests use a copy trimmed to the
#    instruments of the option chain (see docs/api.md).
PT='paperTypes[0]=1&paperTypes[1]=2&paperTypes[2]=3&paperTypes[3]=4&paperTypes[4]=5&paperTypes[5]=6&paperTypes[6]=7&paperTypes[7]=8&paperTypes[8]=9'
fetch "$BASE/ClosingPrice/GetMarketWatch?market=0&industrialGroup=&$PT&showTraded=false&withBestLimits=false&hEven=0&RefID=0" "$OUT/tsetmc_market_watch_full.json" || true
fetch "$BASE/ClosingPrice/GetClosingPriceInfo/17914401175772326" "$OUT/tsetmc_closing_price_info.json" || true

# Sanity check on the main file.
python3 - "$OUT/tsetmc_option_watch.json" <<'EOF'
import json, sys
try:
    d = json.load(open(sys.argv[1], encoding="utf-8"))
except Exception as e:
    print("NOT VALID JSON:", e); sys.exit(1)
print("top-level keys:", list(d))
rows = d.get("instrumentOptMarketWatch", [])
print("rows:", len(rows), "(expect roughly 700-1200; each row = one call + one put)")
if rows:
    print("first row keys:", sorted(rows[0])[:12], "...")
EOF
echo "Done. Tell Claude the files are in backend/testdata/."
