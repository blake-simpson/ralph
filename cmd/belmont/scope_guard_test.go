package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------------
// Layer 0.5 — belmont validate / detectViolations
// ----------------------------------------------------------------------------

func TestDetectViolations_PolishMilestoneNames(t *testing.T) {
	ms := []milestone{
		{ID: "M1", Name: "Route scaffold"},
		{ID: "M2", Name: "Upper sections — Hero, Why We Exist"},
		{ID: "M5", Name: "Polish / follow-ups from M1 implementation"},
		{ID: "M6", Name: "Verification Fixes"},
		{ID: "M7", Name: "Cleanup"},
	}
	got := detectViolations("about", ms)
	if len(got) != 3 {
		t.Fatalf("want 3 polish-name violations, got %d: %+v", len(got), got)
	}
	foundIDs := map[string]bool{}
	for _, v := range got {
		if v.Rule != "polish_milestone_name" {
			t.Errorf("expected polish_milestone_name rule, got %q for %s", v.Rule, v.Milestone)
		}
		foundIDs[v.Milestone] = true
	}
	for _, id := range []string{"M5", "M6", "M7"} {
		if !foundIDs[id] {
			t.Errorf("missing violation for %s", id)
		}
	}
}

func TestDetectViolations_CrossMilestoneTaskID(t *testing.T) {
	ms := []milestone{
		{ID: "M2", Name: "Upper sections", Tasks: []task{
			{ID: "P1-1", Status: taskDone, Name: "Hero"},
			{ID: "P1-2", Status: taskTodo, Name: "Why"},
		}},
		{ID: "M5", Name: "Polish", Tasks: []task{
			{ID: "P3-FWLUP-M2-1", Status: taskTodo, Name: "Replace placeholder"},
			{ID: "P3-FWLUP-3", Status: taskTodo, Name: "SectionReveal axe"},
			{ID: "P0-M1-FIX-1", Status: taskTodo, Name: "Hydration"},
		}},
	}
	got := detectViolations("about", ms)
	// Expect: 1 polish-name for M5, plus 2 cross-milestone task IDs under M5
	// (P3-FWLUP-M2-1 refs M2; P0-M1-FIX-1 refs M1). P3-FWLUP-3 does not
	// embed a milestone.
	var crossViolations []validationViolation
	for _, v := range got {
		if v.Rule == "cross_milestone_task_id" {
			crossViolations = append(crossViolations, v)
		}
	}
	if len(crossViolations) != 2 {
		t.Fatalf("want 2 cross-milestone violations, got %d: %+v", len(crossViolations), crossViolations)
	}
	wantTasks := map[string]bool{"P3-FWLUP-M2-1": true, "P0-M1-FIX-1": true}
	for _, v := range crossViolations {
		if !wantTasks[v.TaskID] {
			t.Errorf("unexpected cross-violation task %q", v.TaskID)
		}
	}
}

func TestDetectViolations_CleanMilestones(t *testing.T) {
	ms := []milestone{
		{ID: "M1", Name: "Route scaffold", Tasks: []task{{ID: "P0-1", Name: "route"}}},
		{ID: "M2", Name: "Upper sections", Tasks: []task{{ID: "P1-1", Name: "Hero"}}},
	}
	got := detectViolations("about", ms)
	if len(got) != 0 {
		t.Errorf("want zero violations, got %d: %+v", len(got), got)
	}
}

func TestDetectViolations_TaskIDInCorrectMilestone(t *testing.T) {
	// A task ID that embeds its own milestone (e.g., P3-FWLUP-M2-1 under M2)
	// is fine — only flag when it's under the WRONG milestone.
	ms := []milestone{
		{ID: "M2", Name: "Upper sections", Tasks: []task{
			{ID: "P3-FWLUP-M2-1", Name: "Replace placeholder"},
		}},
	}
	got := detectViolations("about", ms)
	if len(got) != 0 {
		t.Errorf("task ID under matching milestone should not be flagged, got %+v", got)
	}
}

// ----------------------------------------------------------------------------
// Layer 1 — post-phase scope guard
// ----------------------------------------------------------------------------

func TestParseProgressSnapshot_BasicStructure(t *testing.T) {
	content := `# Progress

## Milestones

### M1: Scaffold
- [v] P0-1: Route
- [v] P0-2: SEO

### M2: Upper sections (depends: M1)
- [x] P1-1: Hero
- [ ] P1-2: Why

## Decisions

Some notes.
`
	snap := parseProgressSnapshot("/tmp/PROGRESS.md", content)
	if len(snap.Blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(snap.Blocks))
	}
	if snap.Blocks[0].ID != "M1" || snap.Blocks[1].ID != "M2" {
		t.Errorf("block IDs wrong: %+v", snap.Blocks)
	}
	if snap.Blocks[0].TaskStates["P0-1"] != "v" || snap.Blocks[0].TaskStates["P0-2"] != "v" {
		t.Errorf("M1 task states wrong: %+v", snap.Blocks[0].TaskStates)
	}
	if snap.Blocks[1].TaskStates["P1-1"] != "x" || snap.Blocks[1].TaskStates["P1-2"] != " " {
		t.Errorf("M2 task states wrong: %+v", snap.Blocks[1].TaskStates)
	}
	// The `## Decisions` block must terminate M2 — so "Some notes." should
	// NOT be part of M2's raw lines.
	joined := strings.Join(snap.Blocks[1].RawLines, "\n")
	if strings.Contains(joined, "Some notes.") {
		t.Errorf("M2 block leaked past `## Decisions`: %q", joined)
	}
}

func TestDiffScopeViolations_DetectsOutOfScopeFlip(t *testing.T) {
	pre := &progressSnapshot{
		Blocks: []milestoneBlockText{
			{ID: "M2", Name: "Upper", TaskStates: map[string]string{"P1-1": "x", "P1-2": "x"}},
			{ID: "M3", Name: "Lower", TaskStates: map[string]string{"P1-5": " ", "P1-6": " "}},
			{ID: "M5", Name: "Polish from M1", TaskStates: map[string]string{"P3-FWLUP-3": " "}},
		},
		ByID: map[string]int{"M2": 0, "M3": 1, "M5": 2},
	}
	post := &progressSnapshot{
		Blocks: []milestoneBlockText{
			// M3 tasks wrongly marked [v] by an M5 phase
			{ID: "M3", Name: "Lower", TaskStates: map[string]string{"P1-5": "v", "P1-6": "v"}},
			{ID: "M5", Name: "Polish from M1", TaskStates: map[string]string{"P3-FWLUP-3": "x"}},
			{ID: "M2", Name: "Upper", TaskStates: map[string]string{"P1-1": "x", "P1-2": "x"}},
		},
		ByID: map[string]int{"M3": 0, "M5": 1, "M2": 2},
	}
	got := diffScopeViolations(pre, post, "M5")
	// Expect 2 out-of-scope flips (M3/P1-5, M3/P1-6). M5/P3-FWLUP-3 is in
	// scope and should not be flagged.
	if len(got) != 2 {
		t.Fatalf("want 2 violations, got %d: %+v", len(got), got)
	}
	for _, v := range got {
		if v.Kind != "out_of_scope_flip" {
			t.Errorf("unexpected kind %q", v.Kind)
		}
		if v.Milestone != "M3" {
			t.Errorf("wrong milestone %q", v.Milestone)
		}
	}
}

func TestDiffScopeViolations_DetectsNewMilestone(t *testing.T) {
	pre := &progressSnapshot{
		Blocks: []milestoneBlockText{
			{ID: "M1", Name: "Scaffold", TaskStates: map[string]string{"P0-1": "v"}},
		},
		ByID: map[string]int{"M1": 0},
	}
	post := &progressSnapshot{
		Blocks: []milestoneBlockText{
			{ID: "M1", Name: "Scaffold", TaskStates: map[string]string{"P0-1": "v"}},
			{ID: "M5", Name: "Polish / follow-ups from M1", TaskStates: map[string]string{"P3-FWLUP-1": " "}},
		},
		ByID: map[string]int{"M1": 0, "M5": 1},
	}
	got := diffScopeViolations(pre, post, "M1")
	if len(got) != 1 {
		t.Fatalf("want 1 violation, got %d: %+v", len(got), got)
	}
	if got[0].Kind != "new_milestone" || got[0].Milestone != "M5" {
		t.Errorf("unexpected violation: %+v", got[0])
	}
}

func TestDiffScopeViolations_EmptyTargetAllowsAllCheckboxes(t *testing.T) {
	// actionImplementNext sometimes runs with empty MilestoneID (batch FWLUP
	// sweep). In that mode, checkbox flips across milestones are permitted;
	// only new milestones are still forbidden.
	pre := &progressSnapshot{
		Blocks: []milestoneBlockText{
			{ID: "M2", TaskStates: map[string]string{"P1-1": "x"}},
			{ID: "M3", TaskStates: map[string]string{"P1-5": " "}},
		},
		ByID: map[string]int{"M2": 0, "M3": 1},
	}
	post := &progressSnapshot{
		Blocks: []milestoneBlockText{
			{ID: "M2", TaskStates: map[string]string{"P1-1": "v"}},
			{ID: "M3", TaskStates: map[string]string{"P1-5": "v"}},
		},
		ByID: map[string]int{"M2": 0, "M3": 1},
	}
	got := diffScopeViolations(pre, post, "")
	if len(got) != 0 {
		t.Errorf("empty target should permit any checkbox change, got %+v", got)
	}
}

func TestRebuildAfterScopeGuard_RestoresOutOfScope(t *testing.T) {
	preContent := `# Progress

## Milestones

### M2: Upper
- [x] P1-1: Hero
- [ ] P1-2: Why

### M3: Lower
- [ ] P1-5: Safety
- [ ] P1-6: Subjects
`
	postContent := `# Progress

## Milestones

### M2: Upper
- [x] P1-1: Hero
- [ ] P1-2: Why

### M3: Lower
- [v] P1-5: Safety
- [v] P1-6: Subjects

### M5: Polish / follow-ups from M2
- [ ] P3-FWLUP-1: Placeholder fix
`
	pre := parseProgressSnapshot("/tmp/PROGRESS.md", preContent)
	post := parseProgressSnapshot("/tmp/PROGRESS.md", postContent)

	rebuilt, err := rebuildAfterScopeGuard(pre, post, "M5")
	if err != nil {
		t.Fatal(err)
	}
	// M3's flips must be reverted.
	if !strings.Contains(rebuilt, "[ ] P1-5: Safety") {
		t.Errorf("M3/P1-5 should be reverted to [ ]; got:\n%s", rebuilt)
	}
	if strings.Contains(rebuilt, "[v] P1-5: Safety") {
		t.Errorf("M3/P1-5 still shows [v]; got:\n%s", rebuilt)
	}
	// M5 is new — it should be dropped entirely.
	if strings.Contains(rebuilt, "M5: Polish") {
		t.Errorf("M5 should have been dropped; got:\n%s", rebuilt)
	}
	// Non-milestone content preserved.
	if !strings.Contains(rebuilt, "# Progress") {
		t.Errorf("non-milestone preamble lost; got:\n%s", rebuilt)
	}
}

func TestRebuildAfterScopeGuard_KeepsInScopeChanges(t *testing.T) {
	preContent := `### M5: Polish
- [ ] P3-FWLUP-1: Fix font
- [ ] P3-FWLUP-2: Fix OG image
`
	postContent := `### M5: Polish
- [x] P3-FWLUP-1: Fix font
- [x] P3-FWLUP-2: Fix OG image
- [ ] P3-FWLUP-4: New follow-up
`
	pre := parseProgressSnapshot("/tmp/PROGRESS.md", preContent)
	post := parseProgressSnapshot("/tmp/PROGRESS.md", postContent)
	rebuilt, err := rebuildAfterScopeGuard(pre, post, "M5")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rebuilt, "[x] P3-FWLUP-1") || !strings.Contains(rebuilt, "[x] P3-FWLUP-2") {
		t.Errorf("in-scope flips should survive; got:\n%s", rebuilt)
	}
	if !strings.Contains(rebuilt, "P3-FWLUP-4") {
		t.Errorf("in-scope new tasks should survive; got:\n%s", rebuilt)
	}
}

// ----------------------------------------------------------------------------
// Layer 2 — verify evidence check
// ----------------------------------------------------------------------------

func TestFindEvidenceMissingFlips_NoGitRepo(t *testing.T) {
	// When the repo doesn't exist (or merge-base query fails), taskHasCommit
	// returns true (fail-open) so the guard doesn't incorrectly revert.
	dir := t.TempDir()
	pre := parseProgressSnapshot(filepath.Join(dir, "PROGRESS.md"), `### M2: X
- [x] P1-1: Task
`)
	post := parseProgressSnapshot(filepath.Join(dir, "PROGRESS.md"), `### M2: X
- [v] P1-1: Task
`)
	missing := findEvidenceMissingFlips(dir, pre, post, "M2")
	if len(missing) != 0 {
		t.Errorf("no git repo → should fail-open, got %+v", missing)
	}
}

func TestRevertEvidenceMissing_FlipsTaskLineBack(t *testing.T) {
	postContent := `### M3: Lower
- [v] P1-5: Safety
- [v] P1-6: Subjects
- [v] P1-7: Press
`
	preContent := `### M3: Lower
- [ ] P1-5: Safety
- [x] P1-6: Subjects
- [ ] P1-7: Press
`
	pre := parseProgressSnapshot("/tmp/PROGRESS.md", preContent)
	post := parseProgressSnapshot("/tmp/PROGRESS.md", postContent)
	missing := []evidenceMissing{
		{Milestone: "M3", TaskID: "P1-5", FromState: " "},
		{Milestone: "M3", TaskID: "P1-7", FromState: " "},
	}
	// P1-6 is not in missing → its [v] should survive
	rebuilt := revertEvidenceMissing(post, pre, missing)
	if !strings.Contains(rebuilt, "[ ] P1-5: Safety") {
		t.Errorf("P1-5 not reverted to [ ]:\n%s", rebuilt)
	}
	if !strings.Contains(rebuilt, "[v] P1-6: Subjects") {
		t.Errorf("P1-6 should remain [v]:\n%s", rebuilt)
	}
	if !strings.Contains(rebuilt, "[ ] P1-7: Press") {
		t.Errorf("P1-7 not reverted to [ ]:\n%s", rebuilt)
	}
}

// ----------------------------------------------------------------------------
// Integration smoke: snapshot → diff → rebuild round-trip over a real
// synthetic PROGRESS.md, mimicking the ea672675 failure pattern.
// ----------------------------------------------------------------------------

// ----------------------------------------------------------------------------
// Live-status overlay — buildStatus merges worktree state per milestone
// ----------------------------------------------------------------------------

func TestOverlayLiveMilestones_OverlaysOwningMilestoneOnly(t *testing.T) {
	dir := t.TempDir()
	wtM2 := filepath.Join(dir, "about-m2-wt")
	wtM3 := filepath.Join(dir, "about-m3-wt")
	if err := os.MkdirAll(wtM2, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wtM3, 0755); err != nil {
		t.Fatal(err)
	}
	// Master state (as baseline — all M2/M3 tasks pending)
	master := []milestone{
		{ID: "M1", Name: "Scaffold", Tasks: []task{{ID: "P0-1", Status: taskVerified}}},
		{ID: "M2", Name: "Upper", Tasks: []task{
			{ID: "P1-1", Status: taskTodo}, {ID: "P1-2", Status: taskTodo},
		}},
		{ID: "M3", Name: "Lower", Tasks: []task{
			{ID: "P1-5", Status: taskTodo}, {ID: "P1-6", Status: taskTodo},
		}},
	}
	// Worktree-local PROGRESS.md for M2 (in progress — agent flipped P1-1 to [x])
	// Note: worktree contains ALL milestones as context (copyBelmontStateToWorktree)
	// but we only overlay its own. We include realistic worktree content.
	m2Progress := `## Milestones

### M1: Scaffold
- [v] P0-1: route

### M2: Upper
- [x] P1-1: Hero
- [>] P1-2: Why we exist

### M3: Lower
- [ ] P1-5: Safety
- [ ] P1-6: Subjects
`
	m3Progress := `## Milestones

### M1: Scaffold
- [v] P0-1: route

### M2: Upper
- [ ] P1-1: Hero
- [ ] P1-2: Why we exist

### M3: Lower
- [x] P1-5: Safety
- [ ] P1-6: Subjects
`
	if err := os.WriteFile(filepath.Join(wtM2, "PROGRESS.md"), []byte(m2Progress), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtM3, "PROGRESS.md"), []byte(m3Progress), 0644); err != nil {
		t.Fatal(err)
	}
	perMS := map[string]string{"M2": wtM2, "M3": wtM3}
	merged := overlayLiveMilestones(master, perMS)

	// M1 should be untouched (no live worktree).
	if merged[0].ID != "M1" || merged[0].LiveFrom != "" {
		t.Errorf("M1 should be untouched, got %+v", merged[0])
	}
	if len(merged[0].Tasks) != 1 || merged[0].Tasks[0].Status != taskVerified {
		t.Errorf("M1 task state lost: %+v", merged[0].Tasks)
	}

	// M2 must be overlaid with the M2 worktree's view.
	if merged[1].ID != "M2" || merged[1].LiveFrom != wtM2 {
		t.Errorf("M2 not overlaid from worktree, got LiveFrom=%q", merged[1].LiveFrom)
	}
	if len(merged[1].Tasks) != 2 {
		t.Fatalf("M2 should have 2 tasks, got %d", len(merged[1].Tasks))
	}
	if merged[1].Tasks[0].Status != taskDone {
		t.Errorf("M2/P1-1 should be [x] (taskDone), got %v", merged[1].Tasks[0].Status)
	}
	if merged[1].Tasks[1].Status != taskInProgress {
		t.Errorf("M2/P1-2 should be [>] (taskInProgress), got %v", merged[1].Tasks[1].Status)
	}

	// M3 must be overlaid with the M3 worktree's view (NOT M2's M3 copy,
	// which may be stale).
	if merged[2].ID != "M3" || merged[2].LiveFrom != wtM3 {
		t.Errorf("M3 not overlaid from its own worktree, got LiveFrom=%q", merged[2].LiveFrom)
	}
	if merged[2].Tasks[0].Status != taskDone {
		t.Errorf("M3/P1-5 should be [x] (from M3 worktree), got %v", merged[2].Tasks[0].Status)
	}
}

func TestOverlayLiveMilestones_FallsBackWhenWorktreeMissing(t *testing.T) {
	master := []milestone{
		{ID: "M1", Tasks: []task{{ID: "P0-1", Status: taskVerified}}},
		{ID: "M2", Tasks: []task{{ID: "P1-1", Status: taskTodo}}},
	}
	// Point M2's worktree path at a non-existent dir — should keep master.
	merged := overlayLiveMilestones(master, map[string]string{"M2": "/nonexistent/path"})
	if merged[1].LiveFrom != "" {
		t.Errorf("missing worktree should not set LiveFrom, got %q", merged[1].LiveFrom)
	}
	if merged[1].Tasks[0].Status != taskTodo {
		t.Errorf("missing worktree should fall back to master state, got %v", merged[1].Tasks[0].Status)
	}
}

func TestScopeGuard_EA672675ReplayScenario(t *testing.T) {
	dir := t.TempDir()
	featureDir := filepath.Join(dir, ".belmont", "features", "about")
	if err := os.MkdirAll(featureDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(featureDir, "PROGRESS.md")

	pre := `# Progress: about

## Milestones

### M3: Lower sections
- [ ] P1-5: Safety
- [ ] P1-6: Subjects
- [ ] P1-7: Press & partners
- [ ] P1-8: Final CTA

### M5: Polish
- [ ] P3-FWLUP-3: SectionReveal axe
`
	post := `# Progress: about

## Milestones

### M3: Lower sections
- [v] P1-5: Safety
- [v] P1-6: Subjects
- [v] P1-7: Press & partners
- [v] P1-8: Final CTA

### M5: Polish
- [v] P3-FWLUP-3: SectionReveal axe
`
	if err := os.WriteFile(path, []byte(pre), 0644); err != nil {
		t.Fatal(err)
	}
	preSnap := snapshotProgress(dir, "about")
	if preSnap == nil {
		t.Fatal("pre snapshot nil")
	}
	// Agent "runs" and writes the post content.
	if err := os.WriteFile(path, []byte(post), 0644); err != nil {
		t.Fatal(err)
	}
	postSnap := parseProgressSnapshot(path, post)
	violations := diffScopeViolations(preSnap, postSnap, "M5")
	// M3's four flips are all out of scope.
	if len(violations) != 4 {
		t.Fatalf("want 4 violations (P1-5..P1-8), got %d: %+v", len(violations), violations)
	}
	// Rebuild and confirm M3 is restored, M5 kept.
	rebuilt, err := rebuildAfterScopeGuard(preSnap, postSnap, "M5")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"P1-5", "P1-6", "P1-7", "P1-8"} {
		if !strings.Contains(rebuilt, "[ ] "+id) {
			t.Errorf("%s not reverted to [ ]:\n%s", id, rebuilt)
		}
	}
	if !strings.Contains(rebuilt, "[v] P3-FWLUP-3") {
		t.Errorf("M5's own [v] flip should survive:\n%s", rebuilt)
	}
}

// ----------------------------------------------------------------------------
// Multi-feature scheduling — wave ordering + dep-paused gating
// ----------------------------------------------------------------------------

func TestComputeFeatureWaves_PreservesInputOrder(t *testing.T) {
	features := []featureSummary{
		{Slug: "c", Status: "Not Started"},
		{Slug: "a", Status: "Not Started"},
		{Slug: "b", Status: "Not Started"},
	}
	waves, err := computeFeatureWaves(features)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(waves) != 1 {
		t.Fatalf("want 1 wave, got %d", len(waves))
	}
	got := []string{}
	for _, f := range waves[0].Features {
		got = append(got, f.Slug)
	}
	want := []string{"c", "a", "b"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("wave 1 order = %v, want %v (input-order contract)", got, want)
	}
}

func TestComputeFeatureWaves_DependencyBeatsInputOrder(t *testing.T) {
	features := []featureSummary{
		{Slug: "b", Status: "Not Started", Deps: []string{"a"}},
		{Slug: "a", Status: "Not Started"},
	}
	waves, err := computeFeatureWaves(features)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(waves) != 2 {
		t.Fatalf("want 2 waves, got %d: %+v", len(waves), waves)
	}
	if waves[0].Features[0].Slug != "a" {
		t.Errorf("wave 1 = %q, want %q", waves[0].Features[0].Slug, "a")
	}
	if waves[1].Features[0].Slug != "b" {
		t.Errorf("wave 2 = %q, want %q", waves[1].Features[0].Slug, "b")
	}
}

func TestResolveFeatureSlugs_AllFlagAlphabetical(t *testing.T) {
	dir := t.TempDir()
	featuresDir := filepath.Join(dir, ".belmont", "features")
	for _, slug := range []string{"charlie", "alpha", "bravo"} {
		fdir := filepath.Join(featuresDir, slug)
		if err := os.MkdirAll(fdir, 0755); err != nil {
			t.Fatal(err)
		}
		// Minimal PRD + PROGRESS so listFeatures picks them up.
		if err := os.WriteFile(filepath.Join(fdir, "PRD.md"), []byte("# PRD: "+slug+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fdir, "PROGRESS.md"), []byte("### M1: stub\n- [ ] T1: t\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Master PROGRESS.md so syncMasterFeatureStatuses doesn't fail noisily.
	if err := os.WriteFile(filepath.Join(dir, ".belmont", "PROGRESS.md"), []byte("# Project Progress\n\n## Features\n"), 0644); err != nil {
		t.Fatal(err)
	}
	slugs, err := resolveFeatureSlugs(dir, "", true)
	if err != nil {
		t.Fatalf("resolveFeatureSlugs: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	if strings.Join(slugs, ",") != strings.Join(want, ",") {
		t.Errorf("--all slugs = %v, want %v (alphabetical contract)", slugs, want)
	}
}

func TestFilterWaveByBlocked_PausedDepCascades(t *testing.T) {
	wave := []featureSummary{
		{Slug: "B", Deps: []string{"A"}},
	}
	paused := map[string]bool{"A": true}
	failed := map[string]bool{}
	runnable, skipped := filterWaveByBlocked(wave, failed, paused)
	if len(runnable) != 0 {
		t.Errorf("want B skipped, got runnable=%+v", runnable)
	}
	if len(skipped) != 1 || skipped[0].Slug != "B" || skipped[0].Reason != "paused" || skipped[0].DepSlug != "A" {
		t.Errorf("want skipResult{B, paused, A}, got %+v", skipped)
	}
}

func TestFilterWaveByBlocked_FailedAndPausedDistinct(t *testing.T) {
	wave := []featureSummary{
		{Slug: "B", Deps: []string{"A"}},
		{Slug: "D", Deps: []string{"C"}},
	}
	paused := map[string]bool{"A": true}
	failed := map[string]bool{"C": true}
	_, skipped := filterWaveByBlocked(wave, failed, paused)
	if len(skipped) != 2 {
		t.Fatalf("want 2 skipped, got %d: %+v", len(skipped), skipped)
	}
	bySlug := map[string]skipResult{}
	for _, s := range skipped {
		bySlug[s.Slug] = s
	}
	if bySlug["B"].Reason != "paused" {
		t.Errorf("B reason = %q, want paused", bySlug["B"].Reason)
	}
	if bySlug["D"].Reason != "failed" {
		t.Errorf("D reason = %q, want failed", bySlug["D"].Reason)
	}
}

func TestFilterWaveByBlocked_TransitiveSkip(t *testing.T) {
	// Simulate three waves processed sequentially: A pauses (wave 1), then
	// wave 2 contains B (depends:A) — gets skipped; pausedSlugs gains B.
	// Wave 3 contains C (depends:B) — also skipped.
	paused := map[string]bool{"A": true}
	failed := map[string]bool{}

	wave2 := []featureSummary{{Slug: "B", Deps: []string{"A"}}}
	_, skipped2 := filterWaveByBlocked(wave2, failed, paused)
	if len(skipped2) != 1 || skipped2[0].Reason != "paused" {
		t.Fatalf("wave 2 skipped = %+v", skipped2)
	}
	// Caller propagates: skipped-due-to-paused → pausedSlugs.
	for _, s := range skipped2 {
		paused[s.Slug] = true
	}

	wave3 := []featureSummary{{Slug: "C", Deps: []string{"B"}}}
	_, skipped3 := filterWaveByBlocked(wave3, failed, paused)
	if len(skipped3) != 1 || skipped3[0].Slug != "C" || skipped3[0].Reason != "paused" || skipped3[0].DepSlug != "B" {
		t.Fatalf("wave 3 transitive skip failed: %+v", skipped3)
	}
	// Caller propagates wave 3's skip too — confirms the cascade chain.
	for _, s := range skipped3 {
		paused[s.Slug] = true
	}
	if !paused["B"] || !paused["C"] {
		t.Errorf("paused after cascade: B=%v C=%v (caller propagation)", paused["B"], paused["C"])
	}
}

func TestFilterWaveByBlocked_FailedWinsOverPaused(t *testing.T) {
	// When both deps block, the harder reason (failed) should be reported.
	wave := []featureSummary{
		{Slug: "X", Deps: []string{"paused-dep", "failed-dep"}},
	}
	failed := map[string]bool{"failed-dep": true}
	paused := map[string]bool{"paused-dep": true}
	_, skipped := filterWaveByBlocked(wave, failed, paused)
	if len(skipped) != 1 || skipped[0].Reason != "failed" || skipped[0].DepSlug != "failed-dep" {
		t.Errorf("want failed/failed-dep, got %+v", skipped)
	}
}

func TestScanReadiness_FlagsNonTerminalDeps(t *testing.T) {
	features := []featureSummary{
		{Slug: "foundation", Status: "In Progress", TasksBlocked: 1},
		{Slug: "student", Status: "Not Started", Deps: []string{"foundation"}},
		{Slug: "parent", Status: "Not Started", Deps: []string{"foundation"}},
	}
	warns := scanReadiness(features)
	if len(warns) != 2 {
		t.Fatalf("want 2 warnings, got %d: %+v", len(warns), warns)
	}
	for _, w := range warns {
		if w.DepSlug != "foundation" {
			t.Errorf("warning %s has wrong dep %q", w.Slug, w.DepSlug)
		}
		if w.DepStatus != "In Progress" {
			t.Errorf("warning %s has wrong dep status %q", w.Slug, w.DepStatus)
		}
		if w.Blocked != 1 {
			t.Errorf("warning %s lost blocked-task count: %d", w.Slug, w.Blocked)
		}
	}
}

func TestScanReadiness_TerminalDepsSilent(t *testing.T) {
	for _, status := range []string{"Complete", "Verified", "Archived"} {
		features := []featureSummary{
			{Slug: "foundation", Status: status},
			{Slug: "student", Status: "Not Started", Deps: []string{"foundation"}},
		}
		if warns := scanReadiness(features); len(warns) != 0 {
			t.Errorf("%s dep should be silent, got %+v", status, warns)
		}
	}
}
