# Ralph CLI Toolkit

A CLI toolkit for running autonomous coding sessions with Claude. Ralph manages a PRD (Product Requirements Document) and tracks progress across multiple implementation iterations, supporting both local and Docker sandbox execution.

Based on the original work by Matt Pocock: [https://www.aihero.dev/getting-started-with-ralph](https://www.aihero.dev/getting-started-with-ralph)

## Quick Start

```bash
# One-time setup
cd /Users/blake/code/sandbox/ralph
./bin/ralph-setup

# Configure Figma MCP (optional, once per sandbox)
ralph-configure-mcp

# Start a new project
cd ~/your-project
ralph-clear              # Reset PRD and progress
ralph-plan               # Create PRD interactively with Claude
ralph-once               # Test one task locally
ralph-afk 10             # Run 10 iterations in Docker sandbox
```

## Installation

Run the setup script:

```bash
cd /Users/blake/code/sandbox/ralph
./bin/ralph-setup
```

This will:
1. Create `~/.local/bin` directory (if needed)
2. Create symlinks for all ralph commands
3. Create `~/.ralph/` config directory
4. Optionally configure your Figma token

Make sure `~/.local/bin` is in your PATH:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Add this line to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.) for persistence.

## Commands

### `ralph-plan`
Interactive planning session with Claude. Runs **locally** (not in sandbox) for interactive PRD creation.

- Uses `--permission-mode plan` for safe exploration
- Guides you through defining tasks
- Outputs structured JSON tasks to PRD.md

```bash
cd ~/your-project
ralph-plan
```

### `ralph-once`
Run a single task implementation locally. Good for testing before going AFK.

- Uses `--permission-mode acceptEdits`
- Implements one task from PRD.md
- Updates progress.txt and marks task complete

```bash
ralph-once
```

### `ralph-afk <iterations>`
Run multiple iterations in Docker sandbox. The main AFK mode.

- Runs in Docker sandbox for isolation
- Mounts ralph directory for PRD/progress access
- Works in your current project directory
- Stops early if all tasks complete

```bash
ralph-afk 10    # Run up to 10 iterations
ralph-afk 50    # Run up to 50 iterations
```

### `ralph-clear`
Reset PRD.md and progress.txt to start fresh.

```bash
ralph-clear
```

### `ralph-status`
Show task completion summary without invoking Claude.

```bash
ralph-status
```

Example output:
```
Ralph Status
============

Total Tasks: 12
  Completed: 7
  Pending:   5
  Progress:  7/12 (58%)

By Category:
  functional: 5/8
  styling: 2/4

Pending Tasks:
  [functional] Implement user logout functionality...
  [styling] Add responsive breakpoints for mobile...
```

### `ralph-setup`
One-time setup script. Creates symlinks and config directory.

```bash
./bin/ralph-setup
```

### `ralph-configure-mcp`
Configure Figma MCP inside Docker sandbox. Run once after sandbox creation.

```bash
ralph-configure-mcp
```

## Configuration

Configuration is stored in `~/.ralph/config`:

```bash
# Ralph configuration
FIGMA_TOKEN=your-figma-personal-access-token
```

The Figma token is:
- Used by `ralph-configure-mcp` to set up the Figma MCP server
- Passed to Docker sandbox via environment variable

## Workflow

### Starting a New Project Cycle

1. **Navigate** to your project directory
2. **Clear** previous state: `ralph-clear`
3. **Plan** interactively: `ralph-plan`
4. **Test** locally: `ralph-once`
5. **Go AFK**: `ralph-afk 20`

### Typical Session

```bash
cd ~/projects/my-app

# Start fresh
ralph-clear

# Interactive planning - Claude asks questions, you provide requirements
ralph-plan
# ... describe your features, provide Figma URLs, etc.

# Check what was planned
ralph-status

# Test one iteration locally to verify setup
ralph-once

# Verify it worked
git log -1
ralph-status

# Go AFK - Claude works autonomously
ralph-afk 15
```

### Checking Progress

While AFK mode is running (or after):

```bash
ralph-status           # Quick summary
cat ~/.local/progress.txt    # Detailed log
git log --oneline -10  # See commits
```

## Directory Structure

```
/Users/blake/code/sandbox/ralph/
├── bin/
│   ├── ralph-plan
│   ├── ralph-once
│   ├── ralph-afk
│   ├── ralph-clear
│   ├── ralph-status
│   ├── ralph-setup
│   └── ralph-configure-mcp
└── README.md

~/.local/
├── PRD.md              # Task definitions (JSON)
├── progress.txt        # Implementation log
└── bin/
    ├── ralph-plan -> /Users/blake/code/sandbox/ralph/bin/ralph-plan
    ├── ralph-once -> ...
    └── ...

~/.ralph/
└── config              # FIGMA_TOKEN and other settings
```

## PRD Format

The PRD.md file contains a JSON array of tasks:

```json
[
  {
    "category": "functional",
    "description": "New chat button creates a fresh conversation",
    "steps": [
      "Click the 'New Chat' button",
      "Verify a new conversation is created",
      "Check that chat area shows welcome state"
    ],
    "passes": false
  },
  {
    "category": "styling",
    "description": "Apply brand colors to header",
    "steps": [
      "Update header background to #1a1a2e",
      "Run npm run lint:fix",
      "Verify no visual regressions"
    ],
    "passes": true
  }
]
```

- `category`: Groups related tasks (functional, styling, etc.)
- `description`: What the task accomplishes
- `steps`: Specific implementation and verification steps
- `passes`: Set to `true` when task is complete

## Troubleshooting

### Commands not found
Ensure `~/.local/bin` is in your PATH:
```bash
echo $PATH | grep -q ".local/bin" || echo "Add ~/.local/bin to PATH"
```

### Docker sandbox issues
- Verify Docker Desktop is running
- Check sandbox exists: `docker sandbox ls`
- Recreate if needed: `docker sandbox rm claude && docker sandbox create claude`
- Re-run `ralph-configure-mcp` after recreating

### Sandbox not logged in
If `ralph-configure-mcp` or `ralph-afk` fails with "Invalid API key" or "Please run /login":
1. Run an interactive session: `docker sandbox run claude`
2. Inside Claude, run `/login` and complete authentication
3. Exit with `/exit`
4. Retry your command

### Figma MCP not working
1. Verify token is set: `grep FIGMA_TOKEN ~/.ralph/config`
2. Ensure sandbox is logged in (see above)
3. Re-run configuration: `ralph-configure-mcp`
4. Verify MCP is configured: `docker sandbox exec claude claude mcp list`

### PRD.md parse errors
- Ensure valid JSON format
- Run `ralph-clear` to reset if corrupted
- Use `ralph-status` to validate

## Tips

- **Start small**: Use `ralph-once` to test before `ralph-afk`
- **Check progress**: Run `ralph-status` periodically
- **Incremental planning**: You can run `ralph-plan` multiple times to add tasks
- **Git safety**: PRD.md and progress.txt should be in .gitignore
- **Voice feedback**: AFK mode uses macOS `say` for audio notifications

## Requirements

- Claude CLI (`claude` command available)
- Docker Desktop with sandbox support
- Python 3 (for `ralph-status` JSON parsing)
- macOS (for `say` command in AFK mode, optional)
