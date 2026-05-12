package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupMainAndWorktree creates a main repo with one commit and a worktree
// branched from that commit. Returns (mainRoot, wtPath, branch).
func setupMainAndWorktree(t *testing.T) (string, string, string) {
	t.Helper()
	mainRoot := t.TempDir()
	runGit(t, mainRoot, "init", "-q", "-b", "main")
	runGit(t, mainRoot, "config", "user.email", "test@test.com")
	runGit(t, mainRoot, "config", "user.name", "Test")
	runGit(t, mainRoot, "config", "commit.gpgsign", "false")

	mustWrite(t, filepath.Join(mainRoot, "app.txt"), "v1\n")
	runGit(t, mainRoot, "add", "-A")
	runGit(t, mainRoot, "commit", "-q", "-m", "initial")

	wtParent := t.TempDir()
	wtPath := filepath.Join(wtParent, "feature-a")
	branch := "belmont/auto/feature-a"
	runGit(t, mainRoot, "worktree", "add", "-b", branch, wtPath, "HEAD")
	// Worktrees inherit user.* from the parent repo via includeIf? No — they share
	// the same .git/config (one repo, multiple worktrees). user.name/email applies.

	return mainRoot, wtPath, branch
}

func TestRebaseWorktreeOnMain_NoOpWhenAtMain(t *testing.T) {
	mainRoot, wtPath, _ := setupMainAndWorktree(t)

	n, err := rebaseWorktreeOnMain(mainRoot, wtPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 new commits, got %d", n)
	}
}

func TestRebaseWorktreeOnMain_BringsInNewMainCommits(t *testing.T) {
	mainRoot, wtPath, _ := setupMainAndWorktree(t)

	// Advance main with two new commits on a separate file to avoid conflicts.
	mustWrite(t, filepath.Join(mainRoot, "other.txt"), "x\n")
	runGit(t, mainRoot, "add", "-A")
	runGit(t, mainRoot, "commit", "-q", "-m", "main commit 1")
	mustWrite(t, filepath.Join(mainRoot, "other2.txt"), "y\n")
	runGit(t, mainRoot, "add", "-A")
	runGit(t, mainRoot, "commit", "-q", "-m", "main commit 2")

	// Worktree has its own commit on a different file (so no conflict).
	mustWrite(t, filepath.Join(wtPath, "feature.txt"), "feat\n")
	runGit(t, wtPath, "add", "-A")
	runGit(t, wtPath, "commit", "-q", "-m", "feature commit")

	n, err := rebaseWorktreeOnMain(mainRoot, wtPath)
	if err != nil {
		t.Fatalf("rebase failed: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 new commits, got %d", n)
	}

	// Worktree should now have both main's new files AND its own feature file.
	for _, f := range []string{"app.txt", "other.txt", "other2.txt", "feature.txt"} {
		if _, err := os.Stat(filepath.Join(wtPath, f)); err != nil {
			t.Errorf("expected %s in worktree after rebase, got: %v", f, err)
		}
	}

	// Main repo's HEAD SHA should be in the worktree's history.
	mainSHA := runGit(t, mainRoot, "rev-parse", "HEAD")
	log := runGit(t, wtPath, "log", "--format=%H")
	if !strings.Contains(log, mainSHA) {
		t.Errorf("worktree history missing main HEAD %s after rebase, log:\n%s", mainSHA, log)
	}
}

func TestRebaseWorktreeOnMain_SkipsOnDirtyWorktree(t *testing.T) {
	mainRoot, wtPath, _ := setupMainAndWorktree(t)

	// Advance main so a rebase would otherwise be needed.
	mustWrite(t, filepath.Join(mainRoot, "other.txt"), "x\n")
	runGit(t, mainRoot, "add", "-A")
	runGit(t, mainRoot, "commit", "-q", "-m", "main commit")

	// Make the worktree dirty with an uncommitted modification.
	mustWrite(t, filepath.Join(wtPath, "app.txt"), "dirty\n")

	preSHA := runGit(t, wtPath, "rev-parse", "HEAD")

	n, err := rebaseWorktreeOnMain(mainRoot, wtPath)
	if !errors.Is(err, errWorktreeDirty) {
		t.Fatalf("expected errWorktreeDirty, got: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 new commits when skipped, got %d", n)
	}

	postSHA := runGit(t, wtPath, "rev-parse", "HEAD")
	if preSHA != postSHA {
		t.Errorf("worktree HEAD changed despite dirty skip: pre=%s post=%s", preSHA, postSHA)
	}
	// Dirty file content should still be on disk.
	data, _ := os.ReadFile(filepath.Join(wtPath, "app.txt"))
	if strings.TrimSpace(string(data)) != "dirty" {
		t.Errorf("dirty content lost: got %q", string(data))
	}
}

func TestRebaseWorktreeOnMain_AbortsOnConflict(t *testing.T) {
	mainRoot, wtPath, _ := setupMainAndWorktree(t)

	// Both branches modify app.txt differently → conflict on rebase.
	mustWrite(t, filepath.Join(mainRoot, "app.txt"), "main-version\n")
	runGit(t, mainRoot, "add", "-A")
	runGit(t, mainRoot, "commit", "-q", "-m", "main edit")

	mustWrite(t, filepath.Join(wtPath, "app.txt"), "worktree-version\n")
	runGit(t, wtPath, "add", "-A")
	runGit(t, wtPath, "commit", "-q", "-m", "worktree edit")

	preSHA := runGit(t, wtPath, "rev-parse", "HEAD")

	_, err := rebaseWorktreeOnMain(mainRoot, wtPath)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if errors.Is(err, errWorktreeDirty) {
		t.Errorf("conflict misclassified as dirty: %v", err)
	}

	// After abort, worktree HEAD must be unchanged.
	postSHA := runGit(t, wtPath, "rev-parse", "HEAD")
	if preSHA != postSHA {
		t.Errorf("worktree HEAD moved after aborted rebase: pre=%s post=%s", preSHA, postSHA)
	}

	// No in-progress rebase should remain.
	gitDir := filepath.Join(wtPath, ".git")
	// In a worktree, .git is a file containing 'gitdir: <path>'.
	if fi, _ := os.Stat(gitDir); fi != nil && !fi.IsDir() {
		data, _ := os.ReadFile(gitDir)
		line := strings.TrimSpace(string(data))
		if strings.HasPrefix(line, "gitdir: ") {
			gitDir = strings.TrimPrefix(line, "gitdir: ")
		}
	}
	if _, err := os.Stat(filepath.Join(gitDir, "rebase-merge")); err == nil {
		t.Errorf("expected rebase-merge state to be cleaned up after abort")
	}
	if _, err := os.Stat(filepath.Join(gitDir, "rebase-apply")); err == nil {
		t.Errorf("expected rebase-apply state to be cleaned up after abort")
	}
}

func TestRebaseWorktreeOnMain_PreservesSteeringMd(t *testing.T) {
	mainRoot, wtPath, _ := setupMainAndWorktree(t)

	// Plant a STEERING.md inside the worktree's .belmont/features/<slug>/.
	// This file is normally assume-unchanged so it's invisible to git status.
	steeringPath := filepath.Join(wtPath, ".belmont", "features", "feature-a", "STEERING.md")
	mustWrite(t, steeringPath, "pending: rerun verify\n")

	// Add the .belmont/ path to git excludes so it doesn't dirty the tree.
	gitDirFile := filepath.Join(wtPath, ".git")
	data, err := os.ReadFile(gitDirFile)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(data))
	gitDir := strings.TrimPrefix(line, "gitdir: ")
	excludePath := filepath.Join(gitDir, "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(excludePath, []byte(".belmont/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Advance main with a non-overlapping commit.
	mustWrite(t, filepath.Join(mainRoot, "other.txt"), "x\n")
	runGit(t, mainRoot, "add", "-A")
	runGit(t, mainRoot, "commit", "-q", "-m", "main commit")

	// Worktree has a feature commit on a different file.
	mustWrite(t, filepath.Join(wtPath, "feature.txt"), "feat\n")
	runGit(t, wtPath, "add", "-A")
	runGit(t, wtPath, "commit", "-q", "-m", "feature commit")

	n, err := rebaseWorktreeOnMain(mainRoot, wtPath)
	if err != nil {
		t.Fatalf("rebase failed: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 new commit, got %d", n)
	}

	// STEERING.md must still exist with its content intact.
	got, err := os.ReadFile(steeringPath)
	if err != nil {
		t.Fatalf("STEERING.md disappeared after rebase: %v", err)
	}
	if !strings.Contains(string(got), "pending: rerun verify") {
		t.Errorf("STEERING.md content changed after rebase: %q", string(got))
	}
}

func TestRebaseWorktreeOnMain_ReportsZeroWhenWorktreeAhead(t *testing.T) {
	mainRoot, wtPath, _ := setupMainAndWorktree(t)

	// Worktree advances; main does not.
	mustWrite(t, filepath.Join(wtPath, "feature.txt"), "feat\n")
	runGit(t, wtPath, "add", "-A")
	runGit(t, wtPath, "commit", "-q", "-m", "feature commit")

	preSHA := runGit(t, wtPath, "rev-parse", "HEAD")

	n, err := rebaseWorktreeOnMain(mainRoot, wtPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 new commits when worktree is ahead, got %d", n)
	}
	postSHA := runGit(t, wtPath, "rev-parse", "HEAD")
	if preSHA != postSHA {
		t.Errorf("worktree HEAD changed when it was already ahead of main: pre=%s post=%s", preSHA, postSHA)
	}
}
