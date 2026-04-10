# Directory Structure

## Belmont Repository

```
belmont/
├── cmd/
│   └── belmont/
│       ├── main.go              # Go CLI entrypoint
│       ├── embed.go             # go:embed directives (release builds)
│       └── embed_dev.go         # Empty embed vars (dev builds)
├── go.mod
├── skills/
│   └── belmont/
│       ├── _partials/           # Shared content blocks for templates
│       ├── _src/                # Skill templates with @include directives
│       ├── product-plan.md      # Planning skill (generated)
│       ├── tech-plan.md         # Tech plan skill (generated)
│       ├── implement.md         # Implementation skill (generated)
│       ├── next.md              # Next task skill (generated)
│       ├── verify.md            # Verification skill (generated)
│       ├── working-backwards.md  # Working backwards skill (generated)
│       ├── debug.md             # Debug router (generated)
│       ├── debug-auto.md       # Auto debug loop (generated)
│       ├── debug-manual.md     # Manual debug loop (generated)
│       ├── status.md            # Status skill
│       ├── review-plans.md      # Alignment review skill
│       ├── cleanup.md           # Archive completed features, reduce bloat
│       └── reset.md             # Reset state skill
├── agents/
│   └── belmont/
│       ├── codebase-agent.md    # Codebase scanning agent
│       ├── design-agent.md      # Figma/design analysis agent
│       ├── implementation-agent.md  # Implementation agent
│       ├── verification-agent.md    # Verification agent
│       └── code-review-agent.md     # Code review agent
├── scripts/
│   ├── build.sh                 # Build with embedded content + version injection
│   ├── release.sh               # Prepare release (changelog + tag)
│   └── generate-skills.sh      # Generate skills from templates + partials
├── .github/
│   └── workflows/
│       └── release.yml          # CI: cross-compile + publish on tag push
├── install.sh                   # Public installer (curl | sh)
├── bin/
│   ├── install.sh               # Dev installer (macOS/Linux)
│   └── install.ps1              # Dev installer (Windows)
├── docs/                        # Documentation
├── CHANGELOG.md
└── README.md
```

## After Installing in a Project

```
your-project/
├── .agents/                     # Shared (committed to git)
│   ├── belmont/                 # Agent instructions
│   │   ├── codebase-agent.md
│   │   ├── design-agent.md
│   │   ├── implementation-agent.md
│   │   ├── verification-agent.md
│   │   └── code-review-agent.md
│   └── skills/
│       └── belmont/             # Skills (canonical location)
│           ├── working-backwards.md
│           ├── product-plan.md
│           ├── tech-plan.md
│           ├── implement.md
│           ├── next.md
│           ├── verify.md
│           ├── debug.md
│           ├── debug-auto.md
│           ├── debug-manual.md
│           ├── status.md
│           ├── review-plans.md
│           ├── cleanup.md
│           └── reset.md
├── .belmont/                    # Planning & state (commit to share with team)
│   ├── PR_FAQ.md
│   ├── PRD.md                   # Living spec (no status markers — purely requirements)
│   ├── PROGRESS.md              # Single source of truth for all state (task checkboxes, milestones)
│   ├── TECH_PLAN.md
│   ├── features/                # Sub-feature directories (optional)
│   │   └── <feature-slug>/
│   │       ├── PRD.md
│   │       ├── TECH_PLAN.md
│   │       ├── PROGRESS.md
│   │       └── MILESTONE.md
│   ├── MILESTONE.md             # Active milestone context (created during implement)
│   └── MILESTONE-M1.done.md     # Archived milestone (after completion)
├── .claude/                     # Claude Code (if selected)
│   ├── agents/
│   │   └── belmont -> ../../.agents/belmont   (symlink)
│   └── commands/
│       └── belmont/              (copied from .agents/skills/belmont)
├── .codex/                      # Codex (if selected)
│   └── belmont/                  (copied from .agents/skills/belmont)
├── AGENTS.md                    # Includes Belmont Codex skill-routing section (if selected)
├── .cursor/                     # Cursor (if selected)
│   └── rules/
│       └── belmont/
│           ├── product-plan.mdc -> ../../../.agents/skills/belmont/product-plan.md
│           ├── tech-plan.mdc    -> ../../../.agents/skills/belmont/tech-plan.md
│           ├── next.mdc         -> ../../../.agents/skills/belmont/next.md
│           └── ...              (per-file symlinks, .mdc -> .md)
└── ...
```

## Key Separation

- `.agents/belmont/` -- Shared agent instructions. Committed to git. Referenced by all tools.
- `.agents/skills/belmont/` -- Canonical skill files. Single source of truth.
- `.belmont/` -- Planning state (PR/FAQ, PRD, PROGRESS, TECH_PLAN, MILESTONE). PRD.md is a status-free living spec; PROGRESS.md is the single source of truth for all task/milestone state. Commit to git so the whole team has shared context.
- `.claude/`, `.codex/`, `.cursor/`, etc. -- Tool-specific wiring. Some use symlinks, some use copied/synced files.

## Should I gitignore `.belmont/`?

Generally, no — commit it so planning docs (PR/FAQ, PRD, TECH_PLAN) are shared across the team. The only case to gitignore it is if you're a solo developer who wants to keep planning state purely local and ephemeral.
