#!/bin/bash
set -e

# Generate skill markdown files from _src/ templates and _partials/.
#
# Output layout (Phase 2): each skill is a directory under skills/belmont/
# containing SKILL.md (frontmatter has `name:` injected) plus a references/
# subdir with the progressive-disclosure files that skill body references.
#
# This is the agentskills.io standard format auto-discovered by Codex 0.126+,
# Cursor, Gemini, Windsurf, and GitHub Copilot. The generated output is
# .gitignored — committed sources are `_src/` and `_partials/` only.
#
# Usage:
#   ./scripts/generate-skills.sh          Generate skills under skills/belmont/<name>/
#   ./scripts/generate-skills.sh --check  Verify no source file is missing its output

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"

PARTIALS_DIR="$ROOT/skills/belmont/_partials"
SRC_DIR="$ROOT/skills/belmont/_src"
REFS_SRC_DIR="$SRC_DIR/references"
DEST_DIR="$ROOT/skills/belmont"

CHECK_MODE=false
if [ "$1" = "--check" ]; then
    CHECK_MODE=true
fi

# Process a single template file, expanding @include directives.
process_file() {
    local src_file="$1"
    local out_file="$2"

    > "$out_file"

    while IFS= read -r line || [ -n "$line" ]; do
        if [[ "$line" =~ ^'<!-- @include '([^[:space:]]+) ]]; then
            local partial_name="${BASH_REMATCH[1]}"
            local partial_file="$PARTIALS_DIR/$partial_name"

            if [ ! -f "$partial_file" ]; then
                echo "Error: partial not found: $partial_file" >&2
                exit 1
            fi

            # Extract the portion after the partial name for key="value" pairs
            local args="${line#*"$partial_name"}"
            args="${args% -->}"

            # Read partial content
            local partial_content
            partial_content="$(cat "$partial_file")"

            # Replace {{key}} with value for each key="value" pair
            while [[ "$args" =~ ([a-zA-Z_]+)=\"([^\"]+)\" ]]; do
                local key="${BASH_REMATCH[1]}"
                local val="${BASH_REMATCH[2]}"
                partial_content="${partial_content//\{\{$key\}\}/$val}"
                args="${args#*"${BASH_REMATCH[0]}"}"
            done

            printf '%s\n' "$partial_content" >> "$out_file"
        else
            printf '%s\n' "$line" >> "$out_file"
        fi
    done < "$src_file"
}

# Check that _src/ directory exists and has templates
if [ ! -d "$SRC_DIR" ] || [ -z "$(ls -A "$SRC_DIR"/*.md 2>/dev/null)" ]; then
    echo "No templates found in $SRC_DIR"
    exit 0
fi

# In check mode, divert output into a temp dir so we can compare against the
# committed `skills/belmont/` afterwards. Trim the temp dir at exit.
if [ "$CHECK_MODE" = true ]; then
    DEST_DIR="$(mktemp -d)"
    trap 'rm -rf "$DEST_DIR"' EXIT
fi

# Collect source skill names so we can prune stale skill folders at the end.
declare -a source_names=()
for src_file in "$SRC_DIR"/*.md; do
    [ -f "$src_file" ] || continue
    source_names+=("$(basename "$src_file" .md)")
done

TMP_FLAT="$(mktemp -d)"
trap 'rm -rf "$TMP_FLAT"' EXIT

# Process each template into <name>/SKILL.md folder layout.
for src_file in "$SRC_DIR"/*.md; do
    [ -f "$src_file" ] || continue
    name="$(basename "$src_file" .md)"
    skill_dir="$DEST_DIR/$name"

    # Stage 1: expand @include partials into a temp flat file. Reuses the
    # existing process_file helper so partial substitution semantics are
    # unchanged.
    flat_tmp="$TMP_FLAT/$name.md"
    process_file "$src_file" "$flat_tmp"

    # Stage 2: rewrite frontmatter to inject `name: <skill-name>` and emit
    # SKILL.md inside the per-skill folder. Drops any pre-existing `name:`
    # line in the frontmatter to avoid duplicates. Errors if the source has no
    # frontmatter — every Belmont skill has one today.
    mkdir -p "$skill_dir"
    awk -v NAME="$name" '
    BEGIN { state="pre-fm" }
    state == "pre-fm" && $0 == "---" {
        state = "in-fm"
        print
        print "name: " NAME
        next
    }
    state == "in-fm" && $0 == "---" {
        state = "post-fm"
        print
        next
    }
    state == "in-fm" && /^name:[[:space:]]/ { next }
    { print }
    END {
        if (state == "pre-fm") {
            print "no frontmatter in " FILENAME > "/dev/stderr"
            exit 1
        }
    }
    ' "$flat_tmp" > "$skill_dir/SKILL.md"

    # Stage 3: per-skill references. Parse the skill body for
    # `references/<X>.md` paths and copy ONLY those files from
    # `_src/references/` into `<skill>/references/`. Robust to references
    # that don't follow the `<skill>-` prefix convention (e.g.
    # `models-yaml-format.md` referenced from tech-plan).
    rm -rf "$skill_dir/references"
    refs="$(grep -oE 'references/[A-Za-z0-9_-]+\.md' "$skill_dir/SKILL.md" | sort -u || true)"
    for ref in $refs; do
        ref_basename="$(basename "$ref")"
        ref_src="$REFS_SRC_DIR/$ref_basename"
        if [ -f "$ref_src" ]; then
            mkdir -p "$skill_dir/references"
            cp "$ref_src" "$skill_dir/references/$ref_basename"
        fi
    done

    echo "Generated $name/SKILL.md"
done

# Prune stale skill folders (target dirs whose source was removed). Only
# directories that already contain a SKILL.md are touched, so we don't
# accidentally rm user-created adjacent dirs.
if [ "$CHECK_MODE" = false ]; then
    for existing_dir in "$DEST_DIR"/*/; do
        [ -d "$existing_dir" ] || continue
        existing_name="$(basename "$existing_dir")"
        # Skip _src/, _partials/, and any leading-underscore dirs.
        case "$existing_name" in _*) continue ;; esac
        # Skip dirs that aren't skill folders.
        [ -f "$existing_dir/SKILL.md" ] || continue
        keep=false
        for n in "${source_names[@]}"; do
            if [ "$n" = "$existing_name" ]; then keep=true; break; fi
        done
        if [ "$keep" = false ]; then
            echo "Pruning stale skill: $existing_name/"
            rm -rf "$existing_dir"
        fi
    done
fi

if [ "$CHECK_MODE" = true ]; then
    echo ""
    echo "Checking generated skill folders against committed files..."

    has_diff=false
    for n in "${source_names[@]}"; do
        gen="$DEST_DIR/$n/SKILL.md"
        committed="$ROOT/skills/belmont/$n/SKILL.md"
        if [ ! -f "$gen" ]; then
            echo "MISSING from generation: $n/SKILL.md"
            has_diff=true
        elif [ ! -f "$committed" ]; then
            # Committed output is .gitignored under Phase 2 — absence is normal,
            # not a check failure.
            :
        elif ! diff -q "$gen" "$committed" >/dev/null 2>&1; then
            echo "STALE: $n/SKILL.md"
            has_diff=true
        fi
    done

    if [ "$has_diff" = true ]; then
        echo ""
        echo "Generated files are out of date. Run ./scripts/generate-skills.sh to update."
        exit 1
    else
        echo "All generated files are up to date."
    fi
else
    echo ""
    echo "Done."
fi
