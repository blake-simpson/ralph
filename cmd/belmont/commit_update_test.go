package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGit is a test helper that runs git and returns stdout, failing the test
// on non-zero exit.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimRight(string(out), "\n")
}

// setupRepo creates an empty git repo with an initial commit and the Belmont
// directory layout populated for the claude tool.
func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	for _, p := range []string{
		".agents/belmont/codebase-agent.md",
		".agents/skills/belmont/implement.md",
		".claude/commands/belmont/implement.md",
	} {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("v1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "initial")
	return dir
}

func TestCommitBelmontUpdate_HappyPath(t *testing.T) {
	dir := setupRepo(t)

	// Simulate update rewriting Belmont files and the user having unrelated
	// staged work in progress.
	mustWrite(t, filepath.Join(dir, ".agents/belmont/codebase-agent.md"), "v2\n")
	mustWrite(t, filepath.Join(dir, ".agents/skills/belmont/implement.md"), "v2\n")
	mustWrite(t, filepath.Join(dir, "src/app.py"), "user code\n")
	runGit(t, dir, "add", "src/app.py")

	if err := commitBelmontUpdate(dir, "v0.99.0"); err != nil {
		t.Fatalf("commitBelmontUpdate: %v", err)
	}

	// The commit should contain ONLY Belmont files.
	files := runGit(t, dir, "diff", "--name-only", "HEAD~1", "HEAD")
	got := strings.Split(files, "\n")
	want := map[string]bool{
		".agents/belmont/codebase-agent.md": true,
		".agents/skills/belmont/implement.md": true,
	}
	for _, f := range got {
		if !want[f] {
			t.Errorf("unexpected file in commit: %s", f)
		}
		delete(want, f)
	}
	for f := range want {
		t.Errorf("missing file in commit: %s", f)
	}

	// The unrelated staged file should remain staged but uncommitted.
	status := runGit(t, dir, "status", "--porcelain")
	if !strings.Contains(status, "A  src/app.py") {
		t.Errorf("expected src/app.py to remain staged, got status:\n%s", status)
	}

	// Commit message format.
	msg := runGit(t, dir, "log", "-1", "--format=%s")
	if msg != "Update Belmont to v0.99.0" {
		t.Errorf("commit message = %q, want %q", msg, "Update Belmont to v0.99.0")
	}
}

func TestCommitBelmontUpdate_NoOpWhenUnchanged(t *testing.T) {
	dir := setupRepo(t)

	if err := commitBelmontUpdate(dir, "v0.99.0"); err != nil {
		t.Fatalf("commitBelmontUpdate: %v", err)
	}

	// No new commit should have been created.
	count := runGit(t, dir, "rev-list", "--count", "HEAD")
	if count != "1" {
		t.Errorf("commit count = %s, want 1 (no new commit)", count)
	}
}

func TestCommitBelmontUpdate_SkipsNonGitDir(t *testing.T) {
	dir := t.TempDir()
	// No git repo here.
	if err := commitBelmontUpdate(dir, "v0.99.0"); err != nil {
		t.Errorf("commitBelmontUpdate in non-git dir should return nil, got %v", err)
	}
}

func TestCommitBelmontUpdate_PreservesUnstagedUserWork(t *testing.T) {
	dir := setupRepo(t)

	// Simulate Belmont edits + user's unstaged change.
	mustWrite(t, filepath.Join(dir, ".agents/belmont/codebase-agent.md"), "v2\n")
	mustWrite(t, filepath.Join(dir, "src/notes.txt"), "user notes\n")

	if err := commitBelmontUpdate(dir, "v0.99.0"); err != nil {
		t.Fatalf("commitBelmontUpdate: %v", err)
	}

	// User's untracked file should remain untracked. Porcelain may report the
	// directory ("?? src/") rather than the file ("?? src/notes.txt"); accept
	// either, but verify the file is still on disk and not in the new commit.
	status := runGit(t, dir, "status", "--porcelain")
	if !strings.Contains(status, "?? src") {
		t.Errorf("expected src/notes.txt to remain untracked, got status:\n%s", status)
	}
	if _, err := os.Stat(filepath.Join(dir, "src/notes.txt")); err != nil {
		t.Errorf("expected src/notes.txt to still exist on disk, got: %v", err)
	}
	files := runGit(t, dir, "diff", "--name-only", "HEAD~1", "HEAD")
	if strings.Contains(files, "src/notes.txt") {
		t.Errorf("did not expect src/notes.txt in commit, got:\n%s", files)
	}
}

func TestRequireCleanWorkingTree_BlocksOnDirty(t *testing.T) {
	dir := setupRepo(t)
	mustWrite(t, filepath.Join(dir, ".agents/belmont/codebase-agent.md"), "dirty\n")

	err := requireCleanWorkingTree(dir)
	if err == nil {
		t.Fatal("expected error for dirty tree, got nil")
	}
	if !strings.Contains(err.Error(), "working tree is not clean") {
		t.Errorf("error missing expected header: %v", err)
	}
	if !strings.Contains(err.Error(), "Looks like a recent `belmont update`") {
		t.Errorf("expected Belmont-update-aware hint when belmont path is dirty, got:\n%s", err.Error())
	}
}

func TestRequireCleanWorkingTree_PassesWhenClean(t *testing.T) {
	dir := setupRepo(t)
	if err := requireCleanWorkingTree(dir); err != nil {
		t.Errorf("expected nil for clean tree, got: %v", err)
	}
}

func TestRequireCleanWorkingTree_GenericHintForNonBelmontDirty(t *testing.T) {
	dir := setupRepo(t)
	mustWrite(t, filepath.Join(dir, "src/app.py"), "user\n")

	err := requireCleanWorkingTree(dir)
	if err == nil {
		t.Fatal("expected error for dirty tree, got nil")
	}
	if strings.Contains(err.Error(), "Looks like a recent `belmont update`") {
		t.Errorf("did not expect belmont-update hint for non-belmont dirty file, got:\n%s", err.Error())
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCommitBelmontUpdate_StagesDeletionOfLegacyPath(t *testing.T) {
	dir := setupRepo(t)

	// Add a legacy `.gemini/rules/belmont/` dir to the initial commit so it's
	// tracked, then delete it (mimicking what runLegacyCleanup would do during
	// a Phase 1 → Phase 2 upgrade).
	mustWrite(t, filepath.Join(dir, ".gemini/rules/belmont/foo.md"), "legacy\n")
	runGit(t, dir, "add", ".gemini/rules/belmont/foo.md")
	runGit(t, dir, "commit", "-q", "-m", "add legacy")
	if err := os.RemoveAll(filepath.Join(dir, ".gemini/rules/belmont")); err != nil {
		t.Fatal(err)
	}

	if err := commitBelmontUpdate(dir, "v0.99.0"); err != nil {
		t.Fatalf("commitBelmontUpdate: %v", err)
	}

	// The deletion of the legacy file should be in the new commit.
	files := runGit(t, dir, "diff", "--name-only", "--diff-filter=D", "HEAD~1", "HEAD")
	if !strings.Contains(files, ".gemini/rules/belmont/foo.md") {
		t.Errorf("expected legacy file deletion in commit, got:\n%s", files)
	}
}

func TestRunLegacyCleanup_RemovesLegacyDirsAndAgentsSection(t *testing.T) {
	dir := setupRepo(t)

	// Plant several legacy artifacts that older Belmont versions would have
	// created.
	mustWrite(t, filepath.Join(dir, ".codex/belmont/old.md"), "legacy\n")
	mustWrite(t, filepath.Join(dir, ".cursor/rules/belmont/old.mdc"), "legacy\n")
	mustWrite(t, filepath.Join(dir, ".windsurf/rules/belmont/old.md"), "legacy\n")
	mustWrite(t, filepath.Join(dir, ".gemini/rules/belmont/old.md"), "legacy\n")
	mustWrite(t, filepath.Join(dir, ".copilot/belmont/old.md"), "legacy\n")
	mustWrite(t, filepath.Join(dir, ".agents/skills/belmont/implement.md"), "stale flat skill\n")
	mustWrite(t, filepath.Join(dir, ".agents/skills/belmont/references/old-ref.md"), "stale ref\n")

	agentsContent := "# AGENTS\n\nUser stuff here.\n\n" +
		belmontAgentsSectionStart + "\n## Belmont section\nlegacy\n" + belmontAgentsSectionEnd + "\n\nMore user stuff.\n"
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), agentsContent)
	mustWrite(t, filepath.Join(dir, "GEMINI.md"),
		belmontGeminiSectionStart+"\n@.agents/skills/belmont/implement.md\n"+belmontGeminiSectionEnd+"\n")

	if err := runLegacyCleanup(dir); err != nil {
		t.Fatalf("runLegacyCleanup: %v", err)
	}

	for _, removed := range []string{
		".codex/belmont", ".cursor/rules/belmont", ".windsurf/rules/belmont",
		".gemini/rules/belmont", ".copilot/belmont",
	} {
		if _, err := os.Stat(filepath.Join(dir, removed)); err == nil {
			t.Errorf("expected %s to be removed", removed)
		}
	}
	// Stale flat skill file under .agents/skills/belmont/ should be gone.
	if _, err := os.Stat(filepath.Join(dir, ".agents/skills/belmont/implement.md")); err == nil {
		t.Errorf("expected stale flat skill to be removed")
	}
	// Top-level references/ dir should be gone.
	if _, err := os.Stat(filepath.Join(dir, ".agents/skills/belmont/references")); err == nil {
		t.Errorf("expected stale top-level references/ to be removed")
	}

	// AGENTS.md preserves user content but loses the Belmont section.
	updated, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(updated), belmontAgentsSectionStart) {
		t.Errorf("AGENTS.md still contains Belmont marker: %s", updated)
	}
	if !strings.Contains(string(updated), "User stuff here.") || !strings.Contains(string(updated), "More user stuff.") {
		t.Errorf("AGENTS.md user content lost: %s", updated)
	}

	// GEMINI.md held only Belmont content, so the file should be deleted.
	if _, err := os.Stat(filepath.Join(dir, "GEMINI.md")); err == nil {
		t.Errorf("expected GEMINI.md (Belmont-only) to be deleted")
	}
}

func TestRunLegacyCleanup_Idempotent(t *testing.T) {
	dir := setupRepo(t)

	// First run on a fresh repo with nothing legacy to clean — should be a no-op.
	if err := runLegacyCleanup(dir); err != nil {
		t.Fatalf("runLegacyCleanup #1: %v", err)
	}
	if err := runLegacyCleanup(dir); err != nil {
		t.Fatalf("runLegacyCleanup #2: %v", err)
	}
}
