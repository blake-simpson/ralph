package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsumePendingSteering(t *testing.T) {
	dir := t.TempDir()
	featureDir := filepath.Join(dir, ".belmont", "features", "myfeat")
	if err := os.MkdirAll(featureDir, 0755); err != nil {
		t.Fatal(err)
	}
	steeringPath := filepath.Join(featureDir, "STEERING.md")

	initial := `## 2026-04-21T10:00:00Z [M5] (pending)
Fix the axes.

## 2026-04-21T10:01:00Z (pending)
Broadcast note — any milestone.

## 2026-04-21T10:02:00Z [M3] (pending)
Only for M3.

## 2026-04-21T09:00:00Z [M5] (consumed 2026-04-21T09:10:00Z by implement)
Legacy consumed entry — should be dropped.
`
	if err := os.WriteFile(steeringPath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	block, count := consumePendingSteering(dir, "myfeat", "M5", "implement")
	if count != 2 {
		t.Fatalf("want 2 consumed entries for M5, got %d", count)
	}
	if !strings.Contains(block, "Fix the axes.") {
		t.Errorf("missing M5-tagged entry in block: %q", block)
	}
	if !strings.Contains(block, "Broadcast note") {
		t.Errorf("missing broadcast entry in block: %q", block)
	}
	if strings.Contains(block, "Only for M3") {
		t.Errorf("M3 entry leaked into M5 run: %q", block)
	}
	if !strings.Contains(block, "URGENT — User steering") {
		t.Errorf("steering header missing: %q", block)
	}

	// STEERING.md should now contain ONLY the M3 pending entry —
	// no consumed markers, no legacy content.
	data, err := os.ReadFile(steeringPath)
	if err != nil {
		t.Fatalf("STEERING.md should still exist (M3 pending remains): %v", err)
	}
	s := string(data)
	if strings.Count(s, "(pending)") != 1 {
		t.Errorf("want exactly one remaining (pending), got:\n%s", s)
	}
	if !strings.Contains(s, "[M3] (pending)") {
		t.Errorf("M3 pending entry lost: %s", s)
	}
	if strings.Contains(s, "consumed ") {
		t.Errorf("STEERING.md must not contain consumed markers, got:\n%s", s)
	}
	if strings.Contains(s, "Legacy consumed") {
		t.Errorf("legacy consumed entry leaked back into STEERING.md: %s", s)
	}

	// Second call on the same milestone should find nothing new.
	block2, count2 := consumePendingSteering(dir, "myfeat", "M5", "implement")
	if count2 != 0 || block2 != "" {
		t.Errorf("re-invocation should be a no-op, got count=%d block=%q", count2, block2)
	}

	// M3 call consumes the last pending entry. STEERING.md should be deleted.
	_, count3 := consumePendingSteering(dir, "myfeat", "M3", "verify")
	if count3 != 1 {
		t.Errorf("M3 should consume its one pending entry, got %d", count3)
	}
	if _, err := os.Stat(steeringPath); !os.IsNotExist(err) {
		t.Errorf("STEERING.md should be deleted once empty, got err=%v", err)
	}
}

func TestConsumePendingSteeringDropsLegacyConsumedOnly(t *testing.T) {
	// A STEERING.md left over from older runs — all entries already
	// consumed — should be cleared from disk so agents don't re-read it.
	dir := t.TempDir()
	featureDir := filepath.Join(dir, ".belmont", "features", "myfeat")
	if err := os.MkdirAll(featureDir, 0755); err != nil {
		t.Fatal(err)
	}
	steeringPath := filepath.Join(featureDir, "STEERING.md")
	initial := `## 2026-04-21T09:00:00Z [M5] (consumed 2026-04-21T09:10:00Z by implement)
Legacy entry.
`
	if err := os.WriteFile(steeringPath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}
	block, count := consumePendingSteering(dir, "myfeat", "M5", "implement")
	if count != 0 || block != "" {
		t.Errorf("legacy consumed-only file should not re-inject; got count=%d block=%q", count, block)
	}
	if _, err := os.Stat(steeringPath); !os.IsNotExist(err) {
		t.Errorf("STEERING.md should be deleted after cleanup, got err=%v", err)
	}
}

func TestConsumePendingSteeringMissingFile(t *testing.T) {
	dir := t.TempDir()
	block, count := consumePendingSteering(dir, "nosuch", "M1", "implement")
	if count != 0 || block != "" {
		t.Errorf("missing file should yield empty result; got count=%d block=%q", count, block)
	}
}

func TestCopyBelmontStateToWorktreePreservesSteering(t *testing.T) {
	// Master has a feature dir with PRD/PROGRESS but no STEERING.md.
	// Worktree has the same feature dir plus a STEERING.md the user injected.
	// After copyBelmontStateToWorktree runs (simulating an auto resume),
	// the master state should be overlaid and STEERING.md must survive.
	master := t.TempDir()
	worktree := t.TempDir()

	masterFeature := filepath.Join(master, ".belmont", "features", "demo")
	if err := os.MkdirAll(masterFeature, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(masterFeature, "PRD.md"), []byte("# master\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(masterFeature, "PROGRESS.md"), []byte("# progress\n"), 0644); err != nil {
		t.Fatal(err)
	}

	worktreeFeature := filepath.Join(worktree, ".belmont", "features", "demo")
	if err := os.MkdirAll(worktreeFeature, 0755); err != nil {
		t.Fatal(err)
	}
	steeringBody := "## 2026-04-21T10:00:00Z [M5] (pending)\nkeep me alive\n"
	if err := os.WriteFile(filepath.Join(worktreeFeature, "STEERING.md"), []byte(steeringBody), 0644); err != nil {
		t.Fatal(err)
	}

	// A fake .git file so untrackBelmontInWorktree etc. don't blow up if they poke at it.
	// copyBelmontStateToWorktree calls writeWorktreeGitExcludes which needs it; tolerate a no-op.
	_ = os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: /nonexistent\n"), 0644)

	if err := copyBelmontStateToWorktree(master, worktree, "demo"); err != nil {
		t.Fatalf("copy: %v", err)
	}

	// STEERING.md must survive.
	got, err := os.ReadFile(filepath.Join(worktreeFeature, "STEERING.md"))
	if err != nil {
		t.Fatalf("STEERING.md missing after copy: %v", err)
	}
	if string(got) != steeringBody {
		t.Errorf("STEERING.md content changed.\nwant: %q\ngot:  %q", steeringBody, string(got))
	}
	// Master state (PRD.md) must have been copied in.
	if _, err := os.Stat(filepath.Join(worktreeFeature, "PRD.md")); err != nil {
		t.Errorf("master PRD.md not overlaid: %v", err)
	}
}

func TestStripSteerComments(t *testing.T) {
	in := "# comment line\n\nreal instruction\n# another comment\nmore text\n"
	out := stripSteerComments(in)
	if strings.Contains(out, "comment") {
		t.Errorf("comments not stripped: %q", out)
	}
	if !strings.Contains(out, "real instruction") || !strings.Contains(out, "more text") {
		t.Errorf("real lines dropped: %q", out)
	}
}
