---
description: Save learnings and discoveries to NOTES.md for persistence across sessions
alwaysApply: false
---

# Belmont: Note

You are saving learnings and discoveries to NOTES.md so they persist across sessions and context compactions. This skill captures non-obvious information — environment quirks, workarounds, debugging insights, credential locations — that would otherwise be lost.

## Storage Locations

- **Feature-level**: `{base}/NOTES.md` — learnings specific to the current feature
- **Global**: `.belmont/NOTES.md` — cross-cutting learnings (env setup, project-wide patterns)

Where `{base}` is the feature's base path (e.g., `.belmont/features/<slug>/`).

## Step 1: Determine Target

Decide where to save the note:

1. If the user specifies "global" or the note is about project-wide setup/environment, use `.belmont/NOTES.md`
2. If the user specifies a feature or you're clearly working in a feature context, use `{base}/NOTES.md`
3. If unclear, ask the user:

```
Where should this note be saved?
  [1] Feature: <feature-name> ({base}/NOTES.md)
  [2] Global (.belmont/NOTES.md)
```

## Step 2: Extract Learnings

Gather the content to save:

1. If the user provided specific text, use that
2. If the user says "save what we learned" or similar, extract non-obvious discoveries from the conversation:
   - Environment requirements or quirks
   - Workarounds for bugs or limitations
   - Debugging insights that took effort to discover
   - Credential or config file locations (NEVER save actual secret values)
   - Performance findings
   - Non-obvious patterns or conventions

**Do NOT save**:
- Routine task completion notes (that's what PROGRESS.md is for)
- Obvious information that's in the docs
- Actual secret values, tokens, passwords, or API keys — only save their locations

## Step 3: Confirm with User

Show the user what will be saved:

```
Saving to: {target file path}

### {Category}
- {learning 1}
- {learning 2}

Save this? [y/n]
```

Wait for confirmation before writing.

## Step 4: Write to NOTES.md

1. **If the file doesn't exist**: Create it with the header and today's entry:

```markdown
# Notes

## {YYYY-MM-DD}

### {Category}
- {learning}
```

2. **If the file exists**: Read it, then:
   - If today's date heading (`## {YYYY-MM-DD}`) already exists, add the new category/entries under it
   - If today's date heading doesn't exist, add a new date section **after the `# Notes` header** (newest first)

## Categories

Use these category headings (or create descriptive ones as needed):

- **Environment** — setup requirements, tool versions, env vars
- **Workaround** — bugs, limitations, and their fixes
- **Discovery** — non-obvious behavior, undocumented features
- **Credential** — where secrets/configs live (never the values themselves)
- **Pattern** — codebase conventions, architectural patterns
- **Debugging** — diagnostic techniques, common error causes
- **Performance** — optimization findings, bottlenecks

## Step 5: Confirm

Report what was saved:

```
Saved to {target file path}
```

## Rules

1. **Never save secret values** — only their locations (e.g., "API key is in `.env.local` under `STRIPE_KEY`")
2. **Only save non-obvious things** — this is not a task log
3. **Always confirm before writing** — show the user what will be saved
4. **Newest first** — most recent date sections go at the top (after the `# Notes` header)
5. **Freeform categories** — use the suggested categories or create descriptive ones that fit
