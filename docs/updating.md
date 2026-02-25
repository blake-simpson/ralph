# Updating Belmont

## Self-update (recommended)

```bash
belmont update
```

This downloads the latest release binary from GitHub and replaces the current one. If you're in a project directory (`.belmont/` exists), it automatically re-runs `belmont install` to update skills and agents.

```bash
belmont update --check    # Check for updates without installing
belmont update --force    # Force update even if same version
```

## Re-install skills in a project

To refresh skills and agents without updating the CLI:

```bash
cd ~/your-project
belmont install
```

The installer detects changes between the embedded (or source) files and your installed files:
- **New files** are copied
- **Changed files** are updated
- **Renamed/deleted files** are removed from the target (keeps installed tree exact)
- **Unchanged files** are skipped
- **Symlinks** are verified and updated if needed
- `.belmont/` state files (PRD, PROGRESS, TECH_PLAN) are always preserved

## Developer updates

If you cloned the repo and built from source:

```bash
cd /path/to/belmont
git pull
./scripts/build.sh
```
