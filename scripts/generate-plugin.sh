#!/bin/bash
set -e

# Generate Claude Code plugin structure from skills and agents.
#
# Usage:
#   ./scripts/generate-plugin.sh              Generate plugin/ directory (version: dev)
#   ./scripts/generate-plugin.sh 0.8.7        Generate with version 0.8.7
#   ./scripts/generate-plugin.sh --check      Verify plugin/ is up to date

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"

SKILLS_SRC="$ROOT/skills/belmont"
AGENTS_SRC="$ROOT/agents/belmont"
PLUGIN_DIR="$ROOT/plugin"

CHECK_MODE=false
VERSION="dev"

for arg in "$@"; do
    if [ "$arg" = "--check" ]; then
        CHECK_MODE=true
    else
        VERSION="$arg"
    fi
done

if [ "$CHECK_MODE" = true ]; then
    PLUGIN_DIR="$(mktemp -d)"
    trap 'rm -rf "$PLUGIN_DIR"' EXIT
fi

# Clean and create structure
rm -rf "$PLUGIN_DIR"
mkdir -p "$PLUGIN_DIR/.claude-plugin"
mkdir -p "$PLUGIN_DIR/skills"
mkdir -p "$PLUGIN_DIR/agents"

# Generate plugin.json
cat > "$PLUGIN_DIR/.claude-plugin/plugin.json" <<EOF
{
  "name": "belmont",
  "version": "$VERSION",
  "description": "Structured AI coding sessions with PRD-driven planning, implementation, and verification",
  "author": {
    "name": "Blake Simpson"
  },
  "repository": "https://github.com/blake-simpson/belmont",
  "license": "MIT",
  "keywords": ["ai", "coding", "prd", "planning", "implementation", "verification"]
}
EOF

# Phase 2: skills/belmont/ already produces folder layout with SKILL.md +
# per-skill references/. The plugin generator just copies each skill folder
# into plugin/skills/. Run generate-skills.sh first to ensure the source is
# fresh.
"$SCRIPT_DIR/generate-skills.sh" >/dev/null

for skill_dir in "$SKILLS_SRC"/*/; do
    [ -d "$skill_dir" ] || continue
    name="$(basename "$skill_dir")"
    case "$name" in _*) continue ;; esac
    [ -f "$skill_dir/SKILL.md" ] || continue

    dest="$PLUGIN_DIR/skills/$name"
    mkdir -p "$dest"
    cp "$skill_dir/SKILL.md" "$dest/SKILL.md"
    if [ -d "$skill_dir/references" ]; then
        mkdir -p "$dest/references"
        cp "$skill_dir/references"/*.md "$dest/references/" 2>/dev/null || true
    fi
    echo "  skill: $name"
done

# Copy agents with name and description added to frontmatter
for agent_file in "$AGENTS_SRC"/*.md; do
    [ -f "$agent_file" ] || continue
    filename="$(basename "$agent_file")"
    name="$(basename "$agent_file" .md)"

    # Extract first heading for description, and transform frontmatter
    awk '
    BEGIN { in_fm=0; fm_done=0; first_line=1; got_desc=0 }
    {
        if (first_line && $0 == "---") {
            in_fm=1
            first_line=0
            next
        }
        if (in_fm && $0 == "---") {
            in_fm=0
            fm_done=1
            # Write new frontmatter
            print "---"
            print "name: " AGENT_NAME
            for (i in fm_lines) print fm_lines[i]
            print "---"
            next
        }
        if (in_fm) {
            fm_lines[++fm_count] = $0
            next
        }
        if (fm_done) {
            print $0
        }
    }
    ' AGENT_NAME="$name" "$agent_file" > "$PLUGIN_DIR/agents/$filename"

    echo "  agent: $name"
done

echo ""

if [ "$CHECK_MODE" = true ]; then
    echo "Checking plugin files against committed plugin/..."

    has_diff=false
    committed="$ROOT/plugin"

    if [ ! -d "$committed" ]; then
        echo "MISSING: plugin/ directory does not exist"
        echo "Run ./scripts/generate-plugin.sh to generate it."
        exit 1
    fi

    # Compare all generated files against committed
    while IFS= read -r rel_path; do
        generated="$PLUGIN_DIR/$rel_path"
        existing="$committed/$rel_path"

        if [ ! -f "$existing" ]; then
            echo "MISSING: $rel_path"
            has_diff=true
        elif ! diff -q "$generated" "$existing" >/dev/null 2>&1; then
            echo "STALE: $rel_path"
            has_diff=true
        fi
    done < <(cd "$PLUGIN_DIR" && find . -type f | sed 's|^\./||' | sort)

    # Check for extra files in committed that shouldn't be there
    while IFS= read -r rel_path; do
        if [ ! -f "$PLUGIN_DIR/$rel_path" ]; then
            echo "EXTRA: $rel_path (should be removed)"
            has_diff=true
        fi
    done < <(cd "$committed" && find . -type f | sed 's|^\./||' | sort)

    if [ "$has_diff" = true ]; then
        echo ""
        echo "Plugin files are out of date. Run ./scripts/generate-plugin.sh to update."
        exit 1
    else
        echo "All plugin files are up to date."
    fi
else
    echo "Plugin generated at $PLUGIN_DIR (version: $VERSION)"
fi
