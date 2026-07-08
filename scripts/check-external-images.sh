#!/usr/bin/env bash
# Verify every EXTERNAL image referenced by a markdown file resolves to a real
# image (HTTP 2xx + an image/* content-type). Catches badge breakage such as the
# hits.sh Let's Encrypt "Gen-Y" TLS chain that GitHub's Camo proxy rejects (502).
#
# Origin-direct check: NEVER pass `-k`/`--insecure` — a no-verify curl would turn a
# genuine TLS-chain failure into a false green, defeating the whole point.
#
# Posture: MANUAL / scheduled only — external hosts (shields, visitor-badge) are
# transiently flaky, so this is deliberately NOT wired into `make static-check`.
# A RED run reliably means the image is broken for everyone; a GREEN run does not
# guarantee Camo renders it (different IP / rate-limit bucket), so still eyeball
# the live page for the definitive check.
set -euo pipefail

README="${1:-README.md}"
CURL_MAX_TIME_SECONDS="${CURL_MAX_TIME_SECONDS:-20}"
CURL_RETRIES="${CURL_RETRIES:-2}"

# External image URLs: markdown ![alt](http..) and HTML <img src="http..">.
urls=$(
  {
    # `|| true`: a no-match grep exits 1, which under `set -e`/`pipefail` would
    # otherwise abort the command substitution (e.g. when a README has only
    # markdown badges and no HTML <img src="http..."> images, or vice versa).
    grep -oE '!\[[^]]*\]\(https?://[^) ]+\)' "$README" | sed -E 's/^!\[[^]]*\]\((https?:\/\/[^) ]+)\)$/\1/' || true
    grep -oE '<img[^>]+src="https?://[^"]+"' "$README" | sed -E 's/.*src="(https?:\/\/[^"]+)".*/\1/' || true
  } | sort -u
)

if [ -z "$urls" ]; then
  echo "No external images in $README (relative images are checked by diagrams-check)."
  exit 0
fi

rc=0
while IFS= read -r u; do
  [ -n "$u" ] || continue
  # `\n` in -w is load-bearing: without a trailing newline `read` returns exit 1
  # at EOF (vars still set), which `set -e` treats as fatal on the success path.
  read -r code ctype < <(curl -sSL --retry "$CURL_RETRIES" --max-time "$CURL_MAX_TIME_SECONDS" \
    -o /dev/null -w '%{http_code} %{content_type}\n' "$u" 2>/dev/null || echo "000 -")
  if [[ "$code" =~ ^2 ]] && [[ "$ctype" =~ ^image/ ]]; then
    echo "OK      $code  $ctype  $u"
  else
    echo "BROKEN  $code  $ctype  $u"
    rc=1
  fi
done <<< "$urls"

exit $rc
