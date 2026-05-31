#!/usr/bin/env bash
# scripts/release.sh — Eidolon tag-only release pipeline
#
# Eidolon is a Go LIBRARY (no binary). A release = a git tag pushed to both
# remotes + a GitHub release entry. Consumers pin via module path + tag:
#   require github.com/ologos-repos/eidolon vMAJOR.MINOR.PATCH.VOLATILE
#
# Usage:
#   ./scripts/release.sh <major|minor|patch|volatile> [--note="..."|--note-file=path] [--yes] [--dry-run]
#   ./scripts/release.sh --repair[=vX.Y.Z.W] [--yes]   # complete a partial release without bumping
#
# Versioning: vMAJOR.MINOR.PATCH.VOLATILE
#   major   → v1.0.0.0  (resets minor, patch, volatile)
#   minor   → v0.11.0.0 (resets patch, volatile)
#   patch   → v0.10.4.0 (resets volatile)
#   volatile→ v0.10.3.1 (no resets)

set -euo pipefail

# ──────────────────────────────────────────────
# Remote configuration
# Detect from `git remote -v`; only push to remotes that exist.
# ──────────────────────────────────────────────
GITEA_REMOTE="origin"
GITHUB_REMOTE="github"

# ──────────────────────────────────────────────
# Colours
# ──────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

info()    { echo -e "${CYAN}${BOLD}[release]${RESET} $*"; }
success() { echo -e "${GREEN}${BOLD}[release]${RESET} $*"; }
warn()    { echo -e "${YELLOW}${BOLD}[release]${RESET} $*"; }
die()     { echo -e "${RED}${BOLD}[release] ERROR:${RESET} $*" >&2; exit 1; }

# ──────────────────────────────────────────────
# Argument parsing
# ──────────────────────────────────────────────
BUMP=""
DRY_RUN=false
NOTE_TEXT=""
NOTE_FILE=""
ASSUME_YES=false
REPAIR=false
REPAIR_TAG=""

for arg in "$@"; do
  case "$arg" in
    major|minor|patch|volatile) BUMP="$arg" ;;
    --dry-run) DRY_RUN=true ;;
    --yes|-y) ASSUME_YES=true ;;
    --repair) REPAIR=true ;;
    --repair=*) REPAIR=true; REPAIR_TAG="${arg#--repair=}" ;;
    --note=*) NOTE_TEXT="${arg#--note=}" ;;
    --note-file=*) NOTE_FILE="${arg#--note-file=}" ;;
    --help|-h)
      echo "Usage: $0 <major|minor|patch|volatile> [--note=\"...\"|--note-file=path] [--yes] [--dry-run]"
      echo "       $0 --repair[=vX.Y.Z.W] [--yes]   # complete a partial release without bumping"
      exit 0
      ;;
    *) die "Unknown argument: $arg" ;;
  esac
done

# Idempotency: --repair completes an existing tag's release (no bump). A bump
# type is required ONLY when not repairing.
if [[ -z "$BUMP" && "$REPAIR" == false ]]; then
  echo "Usage: $0 <major|minor|patch|volatile> [--note=\"...\"|--note-file=path] [--yes] [--dry-run]"
  echo "       $0 --repair[=vX.Y.Z.W] [--yes]"
  echo ""
  echo "  major    v0.10.3.0 → v1.0.0.0"
  echo "  minor    v0.10.3.0 → v0.11.0.0"
  echo "  patch    v0.10.3.0 → v0.10.4.0"
  echo "  volatile v0.10.3.0 → v0.10.3.1"
  echo ""
  echo "  --note=\"...\"         Inline release note (required for non-volatile)"
  echo "  --note-file=path     Read release note from file"
  echo "  RELEASE_NOTE file    Auto-read from repo root if present"
  echo "  --yes / -y           Skip the interactive confirmation (non-interactive)"
  echo "  --repair[=TAG]       Re-run push + GH release for an existing tag"
  echo "                       (latest tag if TAG omitted). Idempotent recovery"
  echo "                       from a partial/interrupted release."
  exit 1
fi

if $REPAIR && [[ -n "$BUMP" ]]; then
  die "--repair cannot be combined with a bump type (major/minor/patch/volatile)."
fi

# ──────────────────────────────────────────────
# Resolve release note (required for non-volatile)
# ──────────────────────────────────────────────
# Priority: --note flag > --note-file flag > RELEASE_NOTE file in repo root
RELEASE_NOTE=""
if [[ -n "$NOTE_TEXT" ]]; then
  RELEASE_NOTE="$NOTE_TEXT"
elif [[ -n "$NOTE_FILE" ]]; then
  [[ -f "$NOTE_FILE" ]] || die "Release note file not found: $NOTE_FILE"
  RELEASE_NOTE="$(cat "$NOTE_FILE")"
elif [[ -f "RELEASE_NOTE" ]]; then
  RELEASE_NOTE="$(cat RELEASE_NOTE)"
fi

if [[ -z "$RELEASE_NOTE" && "$BUMP" != "volatile" && "$REPAIR" == false ]]; then
  die "Release note required for $BUMP releases. Use --note=\"...\" or create a RELEASE_NOTE file."
fi

# ──────────────────────────────────────────────
# Dependency checks
# ──────────────────────────────────────────────
command -v git &>/dev/null || die "git is not installed or not on PATH"
command -v gh  &>/dev/null || die "gh (GitHub CLI) is not installed or not on PATH"

# ──────────────────────────────────────────────
# Must be inside a git repo
# ──────────────────────────────────────────────
git rev-parse --git-dir &>/dev/null || die "Not inside a git repository"

# Change to repo root so all git commands are relative to it
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

# ──────────────────────────────────────────────
# Detect which remotes actually exist
# ──────────────────────────────────────────────
HAS_GITEA=false
HAS_GITHUB=false
git remote | grep -q "^${GITEA_REMOTE}$"  && HAS_GITEA=true  || true
git remote | grep -q "^${GITHUB_REMOTE}$" && HAS_GITHUB=true || true

$HAS_GITEA  || warn "Remote '${GITEA_REMOTE}' not found — skipping Gitea push"
$HAS_GITHUB || warn "Remote '${GITHUB_REMOTE}' not found — skipping GitHub push"
($HAS_GITEA || $HAS_GITHUB) || die "No configured remotes found. Cannot push."

# ──────────────────────────────────────────────
# Branch check — must be on main
# ──────────────────────────────────────────────
CURRENT_BRANCH="$(git branch --show-current)"
[[ "$CURRENT_BRANCH" == "main" ]] || \
  die "Must be on the 'main' branch to release (currently on '$CURRENT_BRANCH')"

# ──────────────────────────────────────────────
# Working tree must be clean
# ──────────────────────────────────────────────
if ! git diff --quiet HEAD; then
  die "Working tree has uncommitted changes. Commit or stash them before releasing."
fi
if ! git diff --cached --quiet; then
  die "There are staged changes. Commit or stash them before releasing."
fi

# ──────────────────────────────────────────────
# Fetch current latest tag
# ──────────────────────────────────────────────
LATEST_TAG="$(git tag --sort=-v:refname | head -1)"
[[ -z "$LATEST_TAG" ]] && die "No tags found in this repository. Create an initial tag first (e.g. v0.0.0.0)."

info "Latest tag: ${BOLD}${LATEST_TAG}${RESET}"

# ──────────────────────────────────────────────
# Parse vMAJOR.MINOR.PATCH.VOLATILE
# ──────────────────────────────────────────────
VERSION_RE='^v([0-9]+)\.([0-9]+)\.([0-9]+)\.([0-9]+)$'
if [[ ! "$LATEST_TAG" =~ $VERSION_RE ]]; then
  die "Latest tag '${LATEST_TAG}' does not match vMAJOR.MINOR.PATCH.VOLATILE format"
fi

MAJOR="${BASH_REMATCH[1]}"
MINOR="${BASH_REMATCH[2]}"
PATCH="${BASH_REMATCH[3]}"
VOLATILE="${BASH_REMATCH[4]}"

# ──────────────────────────────────────────────
# Bump (or resolve repair target — no bump)
# ──────────────────────────────────────────────
if $REPAIR; then
  if [[ -n "$REPAIR_TAG" ]]; then
    NEW_TAG="$REPAIR_TAG"
  else
    NEW_TAG="$LATEST_TAG"
  fi
  git rev-parse -q --verify "refs/tags/${NEW_TAG}" >/dev/null \
    || die "Repair target tag '${NEW_TAG}' does not exist locally. Create it via a normal bump first."
  info "Repair mode: completing release ${BOLD}${NEW_TAG}${RESET} (no version bump)"
else
  case "$BUMP" in
    major)
      MAJOR=$((MAJOR + 1))
      MINOR=0; PATCH=0; VOLATILE=0
      ;;
    minor)
      MINOR=$((MINOR + 1))
      PATCH=0; VOLATILE=0
      ;;
    patch)
      PATCH=$((PATCH + 1))
      VOLATILE=0
      ;;
    volatile)
      VOLATILE=$((VOLATILE + 1))
      ;;
  esac

  NEW_TAG="v${MAJOR}.${MINOR}.${PATCH}.${VOLATILE}"
fi

# ──────────────────────────────────────────────
# Commits since last tag
# ──────────────────────────────────────────────
COMMIT_LOG="$(git log "${LATEST_TAG}..HEAD" --oneline --no-decorate 2>/dev/null || true)"

# ──────────────────────────────────────────────
# Summary
# ──────────────────────────────────────────────
echo ""
echo -e "${BOLD}════════════════════════════════════════${RESET}"
echo -e "${BOLD}  Eidolon Release Summary${RESET}"
echo -e "${BOLD}════════════════════════════════════════${RESET}"
echo -e "  Bump type : ${YELLOW}${BUMP:-repair (no bump)}${RESET}"
echo -e "  Old tag   : ${CYAN}${LATEST_TAG}${RESET}"
echo -e "  New tag   : ${GREEN}${NEW_TAG}${RESET}"
echo -e "  Branch    : ${CURRENT_BRANCH}"
echo -e "  HEAD      : $(git rev-parse --short HEAD)"
echo -e "  Type      : ${CYAN}tag-only (Go library — no binary build)${RESET}"
echo ""

if [[ -z "$COMMIT_LOG" ]]; then
  warn "No commits since ${LATEST_TAG} — releasing HEAD as-is."
else
  echo -e "${BOLD}Commits since ${LATEST_TAG}:${RESET}"
  echo "$COMMIT_LOG" | while IFS= read -r line; do
    echo -e "  ${CYAN}•${RESET} ${line}"
  done
fi

echo ""

if $DRY_RUN; then
  warn "--dry-run mode: no tag will be created, nothing will be pushed."
  echo ""
  if [[ -n "$RELEASE_NOTE" ]]; then
    echo -e "Release note:"
    echo -e "  ${CYAN}${RELEASE_NOTE}${RESET}"
    echo ""
  fi
  echo "Would execute:"
  $HAS_GITEA  && echo "  git push ${GITEA_REMOTE} main"
  $HAS_GITEA  && echo "  git push ${GITEA_REMOTE} ${NEW_TAG}"
  $HAS_GITHUB && echo "  git push ${GITHUB_REMOTE} ${NEW_TAG}"
  $HAS_GITHUB && echo "  git push ${GITHUB_REMOTE} main"
  $HAS_GITHUB && echo "  gh release create ${NEW_TAG} --title '${NEW_TAG}' (with release notes)"
  echo ""
  success "Dry run complete. No changes made."
  exit 0
fi

# ──────────────────────────────────────────────
# Confirmation prompt
# ──────────────────────────────────────────────
if $ASSUME_YES; then
  info "--yes given — proceeding with ${NEW_TAG} non-interactively"
elif [[ -t 0 ]]; then
  echo -e "${BOLD}Proceed with release ${GREEN}${NEW_TAG}${RESET}${BOLD}? [y/N]${RESET} "
  read -r CONFIRM
  case "$CONFIRM" in
    y|Y) ;;
    *) warn "Release aborted."; exit 1 ;;
  esac
else
  die "No TTY available for confirmation. Re-run with --yes to proceed non-interactively."
fi

echo ""

# ──────────────────────────────────────────────
# Tag
# ──────────────────────────────────────────────
if git rev-parse -q --verify "refs/tags/${NEW_TAG}" >/dev/null; then
  EXISTING_C="$(git rev-parse "refs/tags/${NEW_TAG}^{commit}")"
  HEAD_C="$(git rev-parse "HEAD^{commit}")"
  if $REPAIR || [[ "$EXISTING_C" == "$HEAD_C" ]]; then
    info "Tag ${NEW_TAG} already exists — skipping creation (idempotent)"
  else
    die "Tag ${NEW_TAG} already exists at ${EXISTING_C:0:9} but HEAD is ${HEAD_C:0:9}. Refusing to move a published tag. Use a new bump, or --repair=${NEW_TAG} to complete its release."
  fi
else
  info "Creating tag ${NEW_TAG}..."
  git tag "${NEW_TAG}"
  success "Tagged HEAD as ${NEW_TAG}"
fi

# ──────────────────────────────────────────────
# Push to Gitea (origin)
# ──────────────────────────────────────────────
if $HAS_GITEA; then
  info "Pushing main + tag to ${GITEA_REMOTE} (Gitea)..."
  git push "${GITEA_REMOTE}" main
  git push "${GITEA_REMOTE}" "${NEW_TAG}"
  success "Pushed to ${GITEA_REMOTE}"
fi

# ──────────────────────────────────────────────
# Push to GitHub
# Push tag FIRST, separately, so GitHub fires the
# push-tag event before the branch push arrives.
# ──────────────────────────────────────────────
if $HAS_GITHUB; then
  info "Pushing tag to ${GITHUB_REMOTE} (GitHub)..."
  git push "${GITHUB_REMOTE}" "${NEW_TAG}"
  success "Tag pushed to ${GITHUB_REMOTE}"

  info "Pushing main to ${GITHUB_REMOTE} (GitHub)..."
  git push "${GITHUB_REMOTE}" main
  success "Main pushed to ${GITHUB_REMOTE}"

  # Small delay — let GitHub register the tag push event
  # before we create the release (avoids event swallowing)
  sleep 2

  # ──────────────────────────────────────────────
  # GitHub release
  # ──────────────────────────────────────────────
  info "Creating GitHub release ${NEW_TAG}..."

  # Build release notes: human note first, then commit log
  RELEASE_BODY=""

  if [[ -n "$RELEASE_NOTE" ]]; then
    RELEASE_BODY="$(printf "## Release Notes\n\n%s\n" "$RELEASE_NOTE")"
  fi

  if [[ -n "$COMMIT_LOG" ]]; then
    COMMIT_SECTION="$(printf "## Changes since %s\n\n%s" "${LATEST_TAG}" \
      "$(echo "$COMMIT_LOG" | sed 's/^/- /')")"
    if [[ -n "$RELEASE_BODY" ]]; then
      RELEASE_BODY="$(printf "%s\n\n---\n\n%s" "$RELEASE_BODY" "$COMMIT_SECTION")"
    else
      RELEASE_BODY="$COMMIT_SECTION"
    fi
  elif [[ -z "$RELEASE_BODY" ]]; then
    RELEASE_BODY="Release ${NEW_TAG} (no new commits since ${LATEST_TAG})"
  fi

  # Idempotent: if the release already exists, update notes instead of failing.
  if gh release view "${NEW_TAG}" >/dev/null 2>&1; then
    info "GitHub release ${NEW_TAG} already exists — updating notes (idempotent)"
    gh release edit "${NEW_TAG}" \
      --title "${NEW_TAG}" \
      --notes "${RELEASE_BODY}"
  else
    gh release create "${NEW_TAG}" \
      --verify-tag \
      --title "${NEW_TAG}" \
      --notes "${RELEASE_BODY}"
  fi

  # Clean up RELEASE_NOTE file if it was consumed from repo root
  if [[ -f "RELEASE_NOTE" && -z "$NOTE_TEXT" && -z "$NOTE_FILE" ]]; then
    rm -f RELEASE_NOTE
    info "Removed RELEASE_NOTE file (consumed into release)"
  fi

  success "GitHub release created: ${NEW_TAG}"
else
  warn "No GitHub remote configured — skipping GH release creation."
  info "To create a GH release manually: gh release create ${NEW_TAG} --title '${NEW_TAG}'"
fi

# ──────────────────────────────────────────────
# Done — tag-only, no binary build needed
# ──────────────────────────────────────────────
echo ""
echo -e "${BOLD}════════════════════════════════════════${RESET}"
echo -e "${GREEN}${BOLD}  Release ${NEW_TAG} complete!${RESET}"
echo -e "${BOLD}════════════════════════════════════════${RESET}"
echo ""
echo -e "  ${CYAN}•${RESET} Tag        : ${NEW_TAG}"
echo -e "  ${CYAN}•${RESET} Module     : github.com/ologos-repos/eidolon"
echo -e "  ${CYAN}•${RESET} Consumers pin via: go get github.com/ologos-repos/eidolon@${NEW_TAG}"
$HAS_GITEA  && echo -e "  ${CYAN}•${RESET} Pushed to  : ${GITEA_REMOTE} (Gitea)"
$HAS_GITHUB && echo -e "  ${CYAN}•${RESET} Pushed to  : ${GITHUB_REMOTE} (GitHub)"
$HAS_GITHUB && echo -e "  ${CYAN}•${RESET} GH release : $(gh release view "${NEW_TAG}" --json url -q .url 2>/dev/null || echo 'created')"
echo ""
