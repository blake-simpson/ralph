#!/bin/bash
set -e

# Generate skill markdown files from _src/ templates and _partials/.
#
# Usage:
#   ./scripts/generate-skills.sh          Generate skills to skills/belmont/
#   ./scripts/generate-skills.sh --check  Verify generated files are up to date

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"

PARTIALS_DIR="$ROOT/skills/belmont/_partials"
SRC_DIR="$ROOT/skills/belmont/_src"
DEST_DIR="$ROOT/skills/belmont"

CHECK_MODE=false
if [ "$1" = "--check" ]; then
    CHECK_MODE=true
    DEST_DIR="$(mktemp -d)"
    trap 'rm -rf "$DEST_DIR"' EXIT
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

# Process each template
for src_file in "$SRC_DIR"/*.md; do
    [ -f "$src_file" ] || continue
    filename="$(basename "$src_file")"
    out_file="$DEST_DIR/$filename"

    echo "Generating $filename..."
    process_file "$src_file" "$out_file"
done

if [ "$CHECK_MODE" = true ]; then
    echo ""
    echo "Checking generated files against committed files..."

    has_diff=false
    for src_file in "$SRC_DIR"/*.md; do
        [ -f "$src_file" ] || continue
        filename="$(basename "$src_file")"
        generated="$DEST_DIR/$filename"
        committed="$ROOT/skills/belmont/$filename"

        if [ ! -f "$committed" ]; then
            echo "MISSING: $filename (not in skills/belmont/)"
            has_diff=true
        elif ! diff -q "$generated" "$committed" >/dev/null 2>&1; then
            echo "STALE: $filename"
            diff "$committed" "$generated" || true
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
