#!/bin/sh
# Mirror a GitHub release to Gitee (gitee.com/cg33/myco by default):
#
#   GITEE_TOKEN=... scripts/gitee-sync.sh vX.Y.Z
#   GITEE_TOKEN=... scripts/gitee-sync.sh vX.Y.Z --dry-run
#
# Gitee gives a repository 1 GB of release attachments in total, so only the
# newest release keeps its binaries: this removes the attachments of every
# other release and points their notes at GitHub, then uploads this release's
# assets and checks names, sizes and SHA256SUMS.
#
# The release workflow runs it when the repository has a GITEE_TOKEN secret.
# Locally: GITEE_TOKEN=$(...) scripts/gitee-sync.sh vX.Y.Z
#
# Environment: GITEE_TOKEN, GH_TOKEN (or a signed-in gh), MYCO_REPO,
# MYCO_GITEE_REPO, MYCO_GITEE_LOGIN (default cg33, the token's owner).
set -eu
V=${1:?usage: GITEE_TOKEN=... scripts/gitee-sync.sh vX.Y.Z [--dry-run]}
DRY=0
if [ "${2:-}" = --dry-run ]; then DRY=1; fi

REPO=${MYCO_REPO:-chenhg5/myco}
GREPO=${MYCO_GITEE_REPO:-cg33/myco}
LOGIN=${MYCO_GITEE_LOGIN:-cg33}
API=https://gitee.com/api/v5/repos/$GREPO
GH_URL=https://github.com/$REPO/releases/tag/$V
here=$(cd "$(dirname "$0")" && pwd)

if [ -z "${GITEE_TOKEN:-}" ] && [ "$DRY" = 0 ]; then
  echo "gitee-sync: set GITEE_TOKEN" >&2
  exit 2
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say() { echo "gitee-sync: $*"; }

api() { # method path [curl args...]; the token goes through a stdin config, never argv
  m=$1 p=$2
  shift 2
  if [ -n "${GITEE_TOKEN:-}" ]; then
    printf 'header = "Authorization: token %s"\n' "$GITEE_TOKEN" |
      curl -sS -K- -X "$m" "$@" "$API$p"
  else
    curl -sS -X "$m" "$@" "$API$p"
  fi
}

api_to() { # method path outfile [curl args...]; prints the HTTP code
  m=$1 p=$2 out=$3
  shift 3
  api "$m" "$p" -o "$out" -w '%{http_code}' "$@"
}

# 1. This release on GitHub: its notes and its assets.
"$here/gh-get.sh" "repos/$REPO/releases/tags/$V" >"$tmp/gh.json"
python3 - "$tmp/gh.json" >"$tmp/notes.md" <<'PY'
import json, sys
sys.stdout.write(json.load(open(sys.argv[1]))["body"])
PY
"$here/gh-get.sh" "repos/$REPO/commits/$V" >"$tmp/commit.json"
SHA=$(python3 - "$tmp/commit.json" <<'PY'
import json, sys
print(json.load(open(sys.argv[1]))["sha"])
PY
)
say "$V at $SHA"

mkdir "$tmp/assets"
if [ "$DRY" = 0 ]; then
  gh release download "$V" --repo "$REPO" -D "$tmp/assets"
  (cd "$tmp/assets" && sha256sum -c SHA256SUMS >/dev/null) ||
    { echo "gitee-sync: the downloaded assets fail SHA256SUMS" >&2; exit 1; }
fi

# 2. The tag on Gitee, so links from the release page work.
push_url="https://$LOGIN:${GITEE_TOKEN:-}@gitee.com/$GREPO.git"
if git ls-remote "$push_url" "refs/tags/$V" 2>/dev/null | grep -q .; then
  say "tag $V is on Gitee"
elif [ "$DRY" = 1 ]; then
  say "would push tag $V to Gitee"
else
  # Fetch the tag whole: a --depth=1 fetch makes it shallow, and Gitee
  # rejects pushing a shallow tag ("shallow update not allowed").
  if ! git rev-parse -q --verify "refs/tags/$V" >/dev/null; then
    git fetch -q origin "refs/tags/$V:refs/tags/$V" ||
      { echo "gitee-sync: could not fetch tag $V" >&2; exit 1; }
  fi
  if [ -f "$(git rev-parse --git-dir)/shallow" ]; then
    echo "gitee-sync: this clone is shallow; Gitee rejects a shallow tag (fetch --unshallow first)" >&2
    exit 1
  fi
  git push -q "$push_url" "refs/tags/$V:refs/tags/$V" ||
    { echo "gitee-sync: could not push tag $V to Gitee" >&2; exit 1; }
  say "pushed tag $V to Gitee"
fi
if [ "$DRY" = 0 ]; then
  # Keep Gitee's main in step too; a divergence there is a warning, not a
  # failure (the release is about the tag).
  git push -q "$push_url" "HEAD:refs/heads/main" 2>/dev/null ||
    say "warning: could not update Gitee main"
fi

# 3. The release: create it, or keep its notes in step with the CHANGELOG.
# A tag without a release is 404, or 200 with a null body (Gitee does the
# latter once the tag is pushed).
code=$(api_to GET "/releases/tags/$V" "$tmp/rel.json")
case $code in
  200)
    REL=$(python3 - "$tmp/rel.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
print(d["id"] if d else "")
PY
)
    if [ -n "$REL" ]; then action=update; else action=create; fi
    ;;
  404)
    REL=""
    action=create
    ;;
  *)
    echo "gitee-sync: reading the Gitee release: HTTP $code" >&2
    cat "$tmp/rel.json" >&2
    exit 1
    ;;
esac

python3 - "$V" "$tmp/notes.md" "$tmp/release.json" "$tmp/release-patch.json" <<'PY'
import json, sys
v, notes, full, patch = sys.argv[1], open(sys.argv[2]).read().strip(), sys.argv[3], sys.argv[4]
json.dump({"tag_name": v, "name": v, "body": notes, "target_commitish": "", "prerelease": False},
          open(full, "w"), ensure_ascii=False)
json.dump({"tag_name": v, "name": v, "body": notes}, open(patch, "w"), ensure_ascii=False)
PY
python3 - "$tmp/release.json" "$SHA" <<'PY'
import json, sys
p, sha = sys.argv[1], sys.argv[2]
d = json.load(open(p))
d["target_commitish"] = sha
json.dump(d, open(p, "w"), ensure_ascii=False)
PY

if [ "$DRY" = 1 ]; then
  say "would $action release $V"
else
  if [ "$action" = create ]; then
    code=$(api_to POST "/releases" "$tmp/rel.json" \
      -H 'Content-Type: application/json' -d @"$tmp/release.json")
  else
    code=$(api_to PATCH "/releases/$REL" "$tmp/rel.json" \
      -H 'Content-Type: application/json' -d @"$tmp/release-patch.json")
  fi
  case $code in
    200 | 201) ;;
    *)
      echo "gitee-sync: writing the Gitee release: HTTP $code" >&2
      cat "$tmp/rel.json" >&2
      exit 1
      ;;
  esac
  REL=$(python3 - "$tmp/rel.json" <<'PY'
import json, sys
print(json.load(open(sys.argv[1]))["id"])
PY
)
  say "$action release $V (id $REL)"
  # Gitee replaces notes it finds objectionable; keep a short pointer then.
  if ! python3 - "$tmp/rel.json" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
sys.exit(0 if "违规信息" not in (d.get("body") or "") else 1)
PY
  then
    say "Gitee objected to the notes; leaving a pointer to GitHub instead"
    python3 - "$V" "$GH_URL" >"$tmp/fallback.json" <<'PY'
import json, sys
v, url = sys.argv[1], sys.argv[2]
body = f"本次更新见 GitHub Release：{url}\n\nGitee 每个仓库的附件空间共 1 GB，这里只保留最新版的安装包。"
print(json.dumps({"tag_name": v, "name": v, "body": body}, ensure_ascii=False))
PY
    api PATCH "/releases/$REL" -H 'Content-Type: application/json' -d @"$tmp/fallback.json" >/dev/null
  fi
fi

# 4. Keep only this release's binaries: Gitee allows 1 GB per repository.
page=1
while [ "$page" -le 5 ]; do
  code=$(api_to GET "/releases?per_page=100&page=$page" "$tmp/rels.json")
  [ "$code" = 200 ] || { echo "gitee-sync: listing releases: HTTP $code" >&2; exit 1; }
  total=$(python3 - "$tmp/rels.json" <<'PY'
import json, sys
print(len(json.load(open(sys.argv[1]))))
PY
)
  python3 - "$tmp/rels.json" "$V" >"$tmp/old.txt" <<'PY'
import json, sys
for r in json.load(open(sys.argv[1])):
    if r.get("tag_name") != sys.argv[2]:
        print(r["id"])
PY
  [ -s "$tmp/old.txt" ] || break
  while read -r old; do
    code=$(api_to GET "/releases/$old/attach_files?per_page=100" "$tmp/att.json")
    [ "$code" = 200 ] || continue
    python3 - "$tmp/att.json" >"$tmp/old-att.txt" <<'PY'
import json, sys
for a in json.load(open(sys.argv[1])):
    print(a["id"])
PY
    [ -s "$tmp/old-att.txt" ] || continue
    if [ "$DRY" = 1 ]; then
      say "would remove $(wc -l <"$tmp/old-att.txt") attachments from release $old"
      continue
    fi
    while read -r aid; do
      code=$(api_to DELETE "/releases/$old/attach_files/$aid" /dev/null)
      [ "$code" = 204 ] || [ "$code" = 200 ] ||
        { echo "gitee-sync: removing attachment $aid: HTTP $code" >&2; exit 1; }
    done <"$tmp/old-att.txt"
    # Say where the binaries went; leave notes Gitee already shortened alone.
    code=$(api_to GET "/releases/$old" "$tmp/old-rel.json") || true
    if [ "$code" = 200 ] && ! python3 - "$tmp/old-rel.json" <<'PY'
import json, sys
sys.exit(0 if "安装包已移到 GitHub" in (json.load(open(sys.argv[1])).get("body") or "") else 1)
PY
    then
      python3 - "$tmp/old-rel.json" "$tmp/old-patch.json" <<'PY'
import json, sys
r = json.load(open(sys.argv[1]))
body = (r.get("body") or "").rstrip() + "\n\n安装包已移到 GitHub（Gitee 每个仓库的附件空间共 1 GB，这里只保留最新版的安装包）。"
json.dump({"tag_name": r["tag_name"], "name": r.get("name") or r["tag_name"], "body": body},
          open(sys.argv[2], "w"), ensure_ascii=False)
PY
      api PATCH "/releases/$old" -H 'Content-Type: application/json' -d @"$tmp/old-patch.json" >/dev/null
    fi
    say "removed the binaries of release $old"
  done <"$tmp/old.txt"
  if [ "$total" -lt 100 ]; then break; fi
  page=$((page + 1))
done

if [ "$DRY" = 1 ]; then
  say "dry run: nothing was changed"
  exit 0
fi

# 5. Upload what the release does not have yet (same name and size: skip).
code=$(api_to GET "/releases/$REL/attach_files?per_page=100" "$tmp/have.json")
[ "$code" = 200 ] || { echo "gitee-sync: listing attachments: HTTP $code" >&2; exit 1; }
python3 - "$tmp/have.json" "$tmp/assets" >"$tmp/upload.txt" <<'PY'
import json, os, sys
have = {a["name"]: (a["id"], a["size"]) for a in json.load(open(sys.argv[1]))}
for f in sorted(os.listdir(sys.argv[2])):
    p = os.path.join(sys.argv[2], f)
    size = os.path.getsize(p)
    if f in have and have[f][1] == size:
        continue
    if f in have:
        print("replace", have[f][0], p)
    else:
        print("upload", "-", p)
PY
while read -r what id path; do
  if [ "$what" = replace ]; then
    api DELETE "/releases/$REL/attach_files/$id" >/dev/null
    say "replaced $path"
  fi
  code=$(api_to POST "/releases/$REL/attach_files" "$tmp/up.json" -F "file=@$path")
  case $code in
    200 | 201) say "uploaded $(basename "$path")" ;;
    *)
      echo "gitee-sync: uploading $(basename "$path"): HTTP $code" >&2
      cat "$tmp/up.json" >&2
      exit 1
      ;;
  esac
done <"$tmp/upload.txt"

# 6. Check what Gitee now has against the files.
code=$(api_to GET "/releases/$REL/attach_files?per_page=100" "$tmp/have.json")
[ "$code" = 200 ] || { echo "gitee-sync: listing attachments: HTTP $code" >&2; exit 1; }
python3 - "$tmp/have.json" "$tmp/assets" <<'PY'
import json, os, sys
have = {a["name"]: a["size"] for a in json.load(open(sys.argv[1]))}
base = sys.argv[2]
missing, wrong = [], []
for f in sorted(os.listdir(base)):
    size = os.path.getsize(os.path.join(base, f))
    if f not in have:
        missing.append(f)
    elif have[f] != size:
        wrong.append(f"{f} (Gitee {have[f]}, local {size})")
if missing or wrong:
    if missing:
        print("missing on Gitee: " + ", ".join(missing), file=sys.stderr)
    if wrong:
        print("wrong size on Gitee: " + ", ".join(wrong), file=sys.stderr)
    sys.exit(1)
print(f"{len(have)} files on Gitee, all sizes match")
PY
curl -fsSL -A 'Mozilla/5.0' -o "$tmp/sums" "https://gitee.com/$GREPO/releases/download/$V/SHA256SUMS" ||
  { echo "gitee-sync: could not download SHA256SUMS from Gitee" >&2; exit 1; }
diff -q "$tmp/sums" "$tmp/assets/SHA256SUMS" >/dev/null ||
  { echo "gitee-sync: Gitee's SHA256SUMS differs from GitHub's" >&2; exit 1; }
say "$V is on Gitee: https://gitee.com/$GREPO/releases/tag/$V"
