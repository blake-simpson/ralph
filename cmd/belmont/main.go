package main

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
)

var (
	Version   = "dev"
	CommitSHA = "unknown"
	BuildDate = "unknown"
)

// Model tier registry: maps (tool, tier) to the CLI --model identifier.
// Tiers (low/medium/high) are stable across releases; model IDs get bumped
// here as tools ship new versions. This is the single source of truth —
// skill bodies reference the same mapping via _partials/tier-registry.md.
var modelTiers = map[string]map[string]string{
	"claude": {
		"low":    "haiku",
		"medium": "sonnet",
		"high":   "opus",
	},
	"codex": {
		"low":    "gpt-5.4-mini",
		"medium": "gpt-5.3-codex",
		"high":   "gpt-5.4",
	},
	"gemini": {
		"low":    "gemini-2.5-flash-lite",
		"medium": "gemini-2.5-flash",
		"high":   "gemini-2.5-pro",
	},
	"cursor": {
		"low":    "sonnet-4",
		"medium": "sonnet-4-thinking",
		"high":   "gpt-5",
	},
	"copilot": {
		"low":    "haiku-4.5",
		"medium": "claude-sonnet-4.5",
		"high":   "gpt-5.4",
	},
}

// toolSupportsModel indicates whether the tool's CLI accepts --model at all.
var toolSupportsModel = map[string]bool{
	"claude":  true,
	"codex":   true,
	"gemini":  true,
	"cursor":  true,
	"copilot": true,
}

// planningTier is always used for product-plan and tech-plan invocations.
// Planning produces the spec downstream agents execute against, so it
// always runs at the highest-capability tier regardless of per-feature
// config. Editing this is a deliberate, global decision.
const planningTier = "high"

// reconciliationDefaultTier is used when no models.yaml is present.
const reconciliationDefaultTier = "high"

// resolveModelFlags returns the --model <id> flag pair for the given
// tool+tier, or nil if the tool doesn't support model selection or the
// tier is unknown/empty. For copilot with no tier, returns --model auto
// (copilot's explicit "pick a sensible model" token).
func resolveModelFlags(tool, tier string) []string {
	if !toolSupportsModel[tool] {
		return nil
	}
	if tier == "" {
		if tool == "copilot" {
			return []string{"--model", "auto"}
		}
		return nil
	}
	tiers, ok := modelTiers[tool]
	if !ok {
		return nil
	}
	model, ok := tiers[tier]
	if !ok {
		return nil
	}
	return []string{"--model", model}
}

// modelTierConfig holds the parsed contents of .belmont/features/<slug>/models.yaml.
// Empty value is safe to pass everywhere — callers get nil tier strings and fall
// back to agent-frontmatter defaults.
type modelTierConfig struct {
	Profile  string
	Planning string
	Tiers    map[string]string // agent name (e.g. "implementation") -> "low"|"medium"|"high"
}

// parseModelTiers reads .belmont/features/<slug>/models.yaml with a minimal
// line-based parser. Flat schema only: top-level scalar keys (profile, planning)
// and one nested map (tiers:). Unknown keys are ignored so users can add comments
// or extra fields without breaking parse. Stdlib only (no YAML dependency).
// Returns a zero-value struct if the file does not exist.
func parseModelTiers(path string) (modelTierConfig, error) {
	cfg := modelTierConfig{Tiers: map[string]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	inTiers := false
	for _, raw := range strings.Split(string(data), "\n") {
		// Strip # comments (naive — does not support # inside quoted values).
		line := raw
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		isIndented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
		content := strings.TrimSpace(line)
		if !isIndented {
			inTiers = false
			k, v, ok := splitYAMLKV(content)
			if !ok {
				continue
			}
			switch k {
			case "profile":
				cfg.Profile = v
			case "planning":
				cfg.Planning = v
			case "tiers":
				if v == "" {
					inTiers = true
				}
			}
			continue
		}
		if !inTiers {
			continue
		}
		k, v, ok := splitYAMLKV(content)
		if !ok || k == "" || v == "" {
			continue
		}
		cfg.Tiers[k] = v
	}
	return cfg, nil
}

// splitYAMLKV splits "key: value" into trimmed parts with quotes stripped.
func splitYAMLKV(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	k := strings.TrimSpace(line[:idx])
	v := strings.TrimSpace(line[idx+1:])
	v = strings.Trim(v, `"'`)
	return k, v, true
}

// actionAgent maps a loop action type to the Belmont agent name that runs
// the heaviest work for that action. Used for tier lookup. Empty string
// means "no agent mapping" (tier falls through to empty → default model).
func actionAgent(t loopActionType) string {
	switch t {
	case actionImplementMilestone, actionImplementNext, actionFixAll:
		return "implementation"
	case actionVerify:
		return "verification"
	case actionTriage:
		return "verification" // triage reads verification output; share its tier
	case actionReplan:
		return "" // planning uses planningTier, handled separately
	default:
		return ""
	}
}

// tierForAction returns the tier label ("low"|"medium"|"high") for the given
// action under the supplied tier config. Planning actions always return the
// global planningTier. Agent-mapped actions look up the agent's configured
// tier. If the agent has no configured tier, returns "" so callers fall back
// to the tool's default model (no --model flag).
func tierForAction(t loopActionType, cfg modelTierConfig) string {
	if t == actionReplan {
		return planningTier
	}
	agent := actionAgent(t)
	if agent == "" {
		return ""
	}
	if tier, ok := cfg.Tiers[agent]; ok && tier != "" {
		return tier
	}
	return ""
}

// reconciliationTier returns the tier for reconciliation work given a config,
// falling back to reconciliationDefaultTier when not specified.
func reconciliationTier(cfg modelTierConfig) string {
	if t, ok := cfg.Tiers["reconciliation"]; ok && t != "" {
		return t
	}
	return reconciliationDefaultTier
}

type taskStatus string

const (
	taskTodo       taskStatus = "todo"
	taskInProgress taskStatus = "in_progress"
	taskDone       taskStatus = "done"
	taskVerified   taskStatus = "verified"
	taskBlocked    taskStatus = "blocked"
)

type task struct {
	ID          string
	Name        string
	Status      taskStatus
	MilestoneID string // which milestone this task belongs to (from PROGRESS.md)
}

type milestone struct {
	ID    string
	Name  string
	Tasks []task   // tasks in this milestone (from PROGRESS.md)
	Deps  []string // e.g. ["M1", "M3"] — nil for no explicit deps
}

// Milestone computed state helpers

func milestoneAllDone(m milestone) bool {
	if len(m.Tasks) == 0 {
		return false
	}
	for _, t := range m.Tasks {
		if t.Status != taskDone && t.Status != taskVerified {
			return false
		}
	}
	return true
}

func milestoneAllVerified(m milestone) bool {
	if len(m.Tasks) == 0 {
		return false
	}
	for _, t := range m.Tasks {
		if t.Status != taskVerified {
			return false
		}
	}
	return true
}

func milestoneHasBlockers(m milestone) bool {
	for _, t := range m.Tasks {
		if t.Status == taskBlocked {
			return true
		}
	}
	return false
}

func milestoneNotStarted(m milestone) bool {
	for _, t := range m.Tasks {
		if t.Status != taskTodo {
			return false
		}
	}
	return true
}

type featureSummary struct {
	Slug             string      `json:"slug"`
	Name             string      `json:"name"`
	TasksDone        int         `json:"tasks_done"`
	TasksVerified    int         `json:"tasks_verified"`
	TasksInProgress  int         `json:"tasks_in_progress"`
	TasksBlocked     int         `json:"tasks_blocked"`
	TasksTotal       int         `json:"tasks_total"`
	MilestonesDone   int         `json:"milestones_done"`
	MilestonesTotal  int         `json:"milestones_total"`
	Milestones       []milestone `json:"milestones"`
	NextMilestone    *milestone  `json:"next_milestone,omitempty"`
	NextTask         *task       `json:"next_task,omitempty"`
	Status           string      `json:"status"`
	Deps             []string    `json:"deps,omitempty"`
	Priority         string      `json:"priority,omitempty"`
}

type statusReport struct {
	Feature         string
	TechPlanReady   bool
	PRFAQReady      bool
	OverallStatus   string
	TaskCounts      map[string]int
	Tasks           []task
	Milestones      []milestone
	NextMilestone   *milestone
	NextTask        *task
	LastCompleted   *task
	RecentDecisions []string
	Features        []featureSummary
}

type config struct {
	Source string `json:"source"`
}

// Loop types
type loopActionType string

const (
	actionImplementMilestone loopActionType = "IMPLEMENT_MILESTONE"
	actionImplementNext      loopActionType = "IMPLEMENT_NEXT"
	actionVerify             loopActionType = "VERIFY"
	actionPause              loopActionType = "PAUSE"
	actionComplete           loopActionType = "COMPLETE"
	actionError              loopActionType = "ERROR"
	actionReplan             loopActionType = "REPLAN"
	actionSkipMilestone      loopActionType = "SKIP_MILESTONE"
	actionDebug              loopActionType = "DEBUG"
	actionTriage             loopActionType = "TRIAGE"
	actionFixAll             loopActionType = "FIX_ALL"
)

var errFeaturePaused = fmt.Errorf("feature paused")

type loopAction struct {
	Type            loopActionType
	Reason          string
	MilestoneID     string
	TriageDecision  string // "fix_and_reverify", "fix_and_proceed", "defer_and_proceed" — set after triage
	ReverifyScope   string // "full" or "focused" — set by triage
}

type executionResult struct {
	Success    bool
	Output     string
	Error      string
	DurationMs int64
}

type workType string

const (
	workFrontend workType = "frontend" // .tsx, .jsx, .css, .scss, .html, .vue, .svelte
	workBackend  workType = "backend"  // .go, .py, .rs, .java, etc.
	workConfig   workType = "config"   // .yml, .yaml, .json, .toml, CI files
	workDocs     workType = "docs"     // .md, .txt
	workMixed    workType = "mixed"
	workMinimal  workType = "minimal"  // < 3 files changed
	workUnknown  workType = "unknown"
)

type historyEntry struct {
	Action       loopAction
	Result       *executionResult
	TasksDone    int
	TasksTotal   int
	MsDone       int
	MsTotal      int
	BlockerCount int
	HasFwlup     bool
	Iteration    int
	WorkType     workType
	FilesChanged int
	GitSHA       string
	PostGitSHA   string
}

type milestoneLoopState struct {
	ID              string
	Name            string
	Done            bool
	Implemented     bool
	Verified        bool
	VerifyFailed    int
	VerifySucceeded int // how many times verification passed for this milestone
	WorkType        workType
	FilesChanged    int
	FwlupFixRounds  int // how many triage+fix cycles have run for this milestone
}

type checkpointPolicy string

const (
	policyAutonomous  checkpointPolicy = "autonomous"
	policyMilestone   checkpointPolicy = "milestone"
	policyEveryAction checkpointPolicy = "every_action"
)

type loopConfig struct {
	Feature       string
	Root          string
	Tool          string
	From          string
	To            string
	Policy        checkpointPolicy
	MaxIterations int
	MaxFailures   int
	MaxParallel   int
	DryRun        bool
	Port          int                // assigned port for worktree isolation (0 = not in worktree)
	WorktreeEnv   map[string]string  // extra env vars from worktree.json
	Tracker       *worktreeTracker   // process group tracker for cleanup on interrupt
	TrackerID     string             // worktree ID for tracker operations
	ModelTiers    modelTierConfig    // per-feature model tiers (from .belmont/features/<slug>/models.yaml)
}

// worktreeHooks defines lifecycle hooks for worktree isolation.
type worktreeHooks struct {
	Setup    []string          `json:"setup"`
	Teardown []string          `json:"teardown"`
	Env      map[string]string `json:"env"`
}

// loadWorktreeHooks reads .belmont/worktree.json from the project root.
// Returns nil if the file does not exist.
func loadWorktreeHooks(root string) *worktreeHooks {
	data, err := os.ReadFile(filepath.Join(root, ".belmont", "worktree.json"))
	if err != nil {
		return nil
	}
	var hooks worktreeHooks
	if err := json.Unmarshal(data, &hooks); err != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ Failed to parse .belmont/worktree.json: %s\033[0m\n", err)
		return nil
	}
	return &hooks
}

// worktreeBasePath returns the directory for worktrees, stored in the user's home directory.
// This avoids nesting worktrees inside the project where tools like Turbopack detect
// multiple lockfiles and infer the wrong workspace root.
// For /home/user/code/myapp -> ~/.belmont/worktrees/myapp/
func worktreeBasePath(root string) string {
	absRoot, _ := filepath.Abs(root)
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback: use a sibling directory if home is unavailable
		parent := filepath.Dir(absRoot)
		name := filepath.Base(absRoot)
		return filepath.Join(parent, ".belmont-worktrees", name)
	}
	name := filepath.Base(absRoot)
	return filepath.Join(home, ".belmont", "worktrees", name)
}

// allocatePort asks the OS for a free TCP port by binding to :0.
func allocatePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// buildWorktreeEnv creates an environment slice with worktree-specific variables.
// It starts from the current process env and appends PORT, BELMONT_PORT, BELMONT_WORKTREE,
// and any user-defined env vars from worktree.json.
func buildWorktreeEnv(port int, extraEnv map[string]string) []string {
	env := os.Environ()
	if port != 0 {
		env = append(env,
			fmt.Sprintf("PORT=%d", port),
			fmt.Sprintf("BELMONT_PORT=%d", port),
			"BELMONT_WORKTREE=1",
		)
	}
	for k, v := range extraEnv {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

// runWorktreeHookCommands executes a list of shell commands in the worktree directory.
func runWorktreeHookCommands(commands []string, wtPath string, port int, extraEnv map[string]string) error {
	env := buildWorktreeEnv(port, extraEnv)
	for _, cmdStr := range commands {
		cmd := exec.Command("sh", "-c", cmdStr)
		cmd.Dir = wtPath
		cmd.Env = env
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("hook %q failed: %w", cmdStr, err)
		}
	}
	return nil
}

// detectAutoInstallCommands checks the project root for known lock files and
// returns the appropriate dependency install command(s). Returns nil if no
// recognized lock file is found. First match wins.
func detectAutoInstallCommands(root string) []string {
	type entry struct {
		File     string
		Commands []string
	}
	lockfiles := []entry{
		{"pnpm-lock.yaml", []string{"pnpm install --prefer-offline"}},
		{"bun.lockb", []string{"bun install"}},
		{"bun.lock", []string{"bun install"}},
		{"yarn.lock", []string{"yarn install --prefer-offline"}},
		{"package-lock.json", []string{"npm install --prefer-offline"}},
		{"Gemfile.lock", []string{"bundle install"}},
		{"requirements.txt", []string{"pip install -r requirements.txt"}},
		{"Cargo.lock", []string{"cargo build"}},
	}
	for _, lf := range lockfiles {
		if _, err := os.Stat(filepath.Join(root, lf.File)); err == nil {
			return lf.Commands
		}
	}
	return nil
}

// copyEnvFiles copies .env* files from the project root into the worktree.
// These are gitignored so they don't exist in fresh worktrees, but are needed
// by postinstall scripts (e.g., prisma generate) and dev servers.
func copyEnvFiles(projectRoot, wtPath string) {
	entries, err := os.ReadDir(projectRoot)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == ".env" || strings.HasPrefix(name, ".env.") {
			src := filepath.Join(projectRoot, name)
			dst := filepath.Join(wtPath, name)
			data, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			os.WriteFile(dst, data, 0644)
		}
	}
}

type aiDecision struct {
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	MilestoneID string `json:"milestone_id,omitempty"`
}

type reconciliationFile struct {
	Strategy          string `json:"strategy"`
	PostResolveCmd    string `json:"post_resolve_command"`
	File            string `json:"file"`
	Confidence      string `json:"confidence"`
	Reason          string `json:"reason"`
	ConflictSummary string `json:"conflict_summary"`
	ResolvedContent string `json:"resolved_content"`
}

type reconciliationReport struct {
	Files []reconciliationFile `json:"files"`
}

func main() {
	// Clean up old binary on Windows after self-update
	if runtime.GOOS == "windows" {
		if exe, err := os.Executable(); err == nil {
			old := exe + ".old"
			if _, err := os.Stat(old); err == nil {
				os.Remove(old)
			}
		}
	}

	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "status":
		must(runStatus(os.Args[2:]))
	case "auto", "loop":
		must(runAutoCmd(os.Args[2:]))
	case "install":
		must(runInstall(os.Args[2:]))
	case "update":
		must(runUpdate(os.Args[2:]))
	case "recover":
		must(runRecover(os.Args[2:]))
	case "steer":
		must(runSteerCmd(os.Args[2:]))
	case "reverify":
		must(runReverifyCmd(os.Args[2:]))
	case "sync":
		must(runSyncCmd(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Printf("belmont %s (%s, %s)\n", Version, CommitSHA, BuildDate)
	case "help", "-h", "--help":
		printUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(1)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Belmont Helper")
	fmt.Fprintln(w, "==============")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  belmont install [--source PATH] [--project PATH] [--tools all|none|claude,codex,...]")
	fmt.Fprintln(w, "  belmont update [--check] [--force]")
	fmt.Fprintln(w, "  belmont status [--root PATH] [--feature SLUG] [--format text|json]")
	fmt.Fprintln(w, "  belmont auto --feature SLUG [--from M1] [--to M5] [--tool claude|codex|gemini|copilot|cursor] [--policy autonomous|milestone|every_action] [--max-iterations N] [--max-parallel N] [--root PATH]")
	fmt.Fprintln(w, "    (alias: belmont loop)")
	fmt.Fprintln(w, "  belmont reverify [--feature SLUG] [--from M1] [--to M5] [--root PATH] [--format text|json]")
	fmt.Fprintln(w, "  belmont sync [--root PATH]")
	fmt.Fprintln(w, "  belmont recover [--list] [--merge SLUG] [--clean SLUG] [--clean-all] [--tool claude|codex|gemini|copilot|cursor] [--root PATH] [--format text|json]")
	fmt.Fprintln(w, "  belmont steer [--feature SLUG] [--milestone M5] [--message \"text\" | --file PATH | -] [--root PATH]")
	fmt.Fprintln(w, "  belmont version")
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func runStatus(args []string) error {
	fsFlags := flag.NewFlagSet("status", flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard)
	var root string
	var format string
	var maxName int
	var feature string
	fsFlags.StringVar(&root, "root", ".", "project root")
	fsFlags.StringVar(&format, "format", "text", "text or json")
	fsFlags.IntVar(&maxName, "max-task-name", 55, "max task name length")
	fsFlags.StringVar(&feature, "feature", "", "feature slug")
	if err := fsFlags.Parse(args); err != nil {
		return fmt.Errorf("status: %w", err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	report, err := buildStatus(absRoot, maxName, feature)
	if err != nil {
		return err
	}

	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case "text":
		fmt.Print(renderStatus(report, isTerminal(os.Stdout)))
		return nil
	default:
		return fmt.Errorf("status: unknown format %q", format)
	}
}

func buildStatus(root string, maxName int, feature string) (statusReport, error) {
	var report statusReport
	report.TaskCounts = map[string]int{
		"todo":        0,
		"in_progress": 0,
		"done":        0,
		"verified":    0,
		"blocked":     0,
		"total":       0,
	}

	// Check for PR_FAQ
	prfaqPath := filepath.Join(root, ".belmont", "PR_FAQ.md")
	report.PRFAQReady = fileHasRealContent(prfaqPath)

	// Determine base path based on feature mode
	featuresDir := filepath.Join(root, ".belmont", "features")

	// Load worktree overrides so we can read live state from active worktrees
	worktreeOverrides := loadAutoWorktrees(root)

	if feature != "" {
		// Specific feature requested
		featurePath := filepath.Join(featuresDir, feature)
		// If there's an active worktree for this feature, read state from there
		if override, ok := worktreeOverrides[feature]; ok {
			featurePath = override
		}
		if !dirExists(featurePath) {
			return report, fmt.Errorf("status: feature %q not found in %s", feature, featuresDir)
		}

		prdPath := filepath.Join(featurePath, "PRD.md")
		progressPath := filepath.Join(featurePath, "PROGRESS.md")
		techPlanPath := filepath.Join(featurePath, "TECH_PLAN.md")

		prdContent, err := os.ReadFile(prdPath)
		if err != nil {
			return report, fmt.Errorf("status: missing %s", prdPath)
		}

		progressContent, err := os.ReadFile(progressPath)
		if err != nil {
			return report, fmt.Errorf("status: missing %s", progressPath)
		}

		report.Feature = extractFeatureName(string(prdContent))
		report.Milestones = parseMilestones(string(progressContent))
		report.Tasks = flattenTasks(report.Milestones, maxName)

		report.TaskCounts["total"] = len(report.Tasks)
		for _, t := range report.Tasks {
			switch t.Status {
			case taskDone:
				report.TaskCounts["done"]++
			case taskVerified:
				report.TaskCounts["verified"]++
			case taskBlocked:
				report.TaskCounts["blocked"]++
			case taskInProgress:
				report.TaskCounts["in_progress"]++
			case taskTodo:
				report.TaskCounts["todo"]++
			}
		}

		report.LastCompleted = lastCompletedTask(report.Tasks)
		report.RecentDecisions = parseDecisions(string(progressContent), 3)
		report.NextMilestone = nextMilestone(report.Milestones)
		report.NextTask = nextTask(report.Tasks)
		report.TechPlanReady = techPlanReady(techPlanPath)
		report.OverallStatus = computeOverallStatus(report.Tasks)

		return report, nil
	}

	// Feature listing mode (default)
	features := listFeaturesWithOverrides(featuresDir, maxName, worktreeOverrides)
	if features == nil {
		features = []featureSummary{}
	}
	populateFeatureDeps(features, root)
	report.Features = features
	report.Feature = extractProductName(filepath.Join(root, ".belmont", "PRD.md"))
	report.TechPlanReady = techPlanReady(filepath.Join(root, ".belmont", "TECH_PLAN.md"))

	if len(features) > 0 {
		report.OverallStatus = computeFeatureListStatus(features)
	} else {
		report.OverallStatus = "Not Started"
	}

	return report, nil
}

func extractProductName(prdPath string) string {
	content, err := os.ReadFile(prdPath)
	if err != nil {
		return "Unnamed Product"
	}
	re := regexp.MustCompile(`(?m)^#\s*Product:\s*(.+)$`)
	match := re.FindStringSubmatch(string(content))
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return extractFeatureName(string(content))
}

// loadAutoWorktrees reads .belmont/auto.json and returns a map of feature slug → worktree feature path.
// If auto.json doesn't exist or isn't active, returns nil.
func loadAutoWorktrees(root string) map[string]string {
	autoPath := filepath.Join(root, ".belmont", "auto.json")
	data, err := os.ReadFile(autoPath)
	if err != nil {
		return nil
	}
	var aj autoJSON
	if err := json.Unmarshal(data, &aj); err != nil || !aj.Active {
		return nil
	}
	result := make(map[string]string)
	for slug, entry := range aj.Worktrees {
		// Verify the worktree still exists before using it
		wtFeaturePath := filepath.Join(entry.Path, ".belmont", "features", slug)
		if dirExists(wtFeaturePath) {
			result[slug] = wtFeaturePath
		}
	}
	return result
}

func listFeatures(featuresDir string, maxName int) []featureSummary {
	return listFeaturesWithOverrides(featuresDir, maxName, nil)
}

// listFeaturesWithOverrides is like listFeatures but allows worktree path overrides.
// The overrides map slug → worktree feature path for features with active worktrees.
func listFeaturesWithOverrides(featuresDir string, maxName int, worktreeOverrides map[string]string) []featureSummary {
	entries, err := os.ReadDir(featuresDir)
	if err != nil {
		return nil
	}
	var features []featureSummary
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		slug := entry.Name()
		featurePath := filepath.Join(featuresDir, slug)
		// If there's an active worktree for this feature, read state from there instead
		if override, ok := worktreeOverrides[slug]; ok {
			featurePath = override
		}
		prdPath := filepath.Join(featurePath, "PRD.md")

		name := slug
		if prdContent, err := os.ReadFile(prdPath); err == nil {
			extracted := extractFeatureName(string(prdContent))
			if extracted != "Unknown" {
				name = extracted
			}
		}

		// Read all state from PROGRESS.md
		var milestones []milestone
		progressPath := filepath.Join(featurePath, "PROGRESS.md")
		if progressContent, err := os.ReadFile(progressPath); err == nil {
			milestones = parseMilestones(string(progressContent))
		}

		tasks := flattenTasks(milestones, maxName)
		tasksTotal := len(tasks)
		tasksDone := 0
		tasksVerified := 0
		tasksInProgress := 0
		tasksBlocked := 0
		for _, t := range tasks {
			switch t.Status {
			case taskDone:
				tasksDone++
			case taskVerified:
				tasksVerified++
			case taskInProgress:
				tasksInProgress++
			case taskBlocked:
				tasksBlocked++
			}
		}

		milestonesDone := 0
		for _, m := range milestones {
			if milestoneAllDone(m) {
				milestonesDone++
			}
		}

		featureNextMilestone := nextMilestone(milestones)
		featureNextTask := nextTask(tasks)

		status := computeOverallStatus(tasks)

		features = append(features, featureSummary{
			Slug:            slug,
			Name:            name,
			TasksDone:       tasksDone + tasksVerified,
			TasksVerified:   tasksVerified,
			TasksInProgress: tasksInProgress,
			TasksBlocked:    tasksBlocked,
			TasksTotal:      tasksTotal,
			MilestonesDone:  milestonesDone,
			MilestonesTotal: len(milestones),
			Milestones:      milestones,
			NextMilestone:   featureNextMilestone,
			NextTask:        featureNextTask,
			Status:          status,
		})
	}
	return features
}

// parseMasterDeps reads the master PROGRESS.md and extracts feature slug → dependency slugs mapping
// from the ## Features table. Handles "None", empty, and comma-separated slugs.
// New table format: | Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |
func parseMasterDeps(root string) (deps map[string][]string, priorities map[string]string) {
	deps = make(map[string][]string)
	priorities = make(map[string]string)

	progressPath := filepath.Join(root, ".belmont", "PROGRESS.md")
	content, err := os.ReadFile(progressPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	colIdx := parseMasterTableColumns(lines)
	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## Features") {
			inTable = true
			continue
		}

		if inTable && strings.HasPrefix(trimmed, "## ") {
			break
		}

		if !inTable || !strings.HasPrefix(trimmed, "|") {
			continue
		}

		cells := splitTableCells(trimmed)
		slugCol := colIdx["Slug"]
		prioCol := colIdx["Priority"]
		depCol := colIdx["Dependencies"]

		if slugCol < 0 || len(cells) <= slugCol {
			continue
		}

		slug := strings.TrimSpace(cells[slugCol])
		if slug == "Slug" || strings.HasPrefix(slug, "-") || strings.HasPrefix(slug, ":") {
			continue
		}

		if prioCol >= 0 && prioCol < len(cells) {
			priorities[slug] = strings.TrimSpace(cells[prioCol])
		}

		if depCol < 0 || depCol >= len(cells) {
			continue
		}
		depStr := strings.TrimSpace(cells[depCol])
		if depStr == "" || strings.EqualFold(depStr, "None") || depStr == "-" {
			continue
		}

		var depSlugs []string
		for _, d := range strings.Split(depStr, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				depSlugs = append(depSlugs, d)
			}
		}
		if len(depSlugs) > 0 {
			deps[slug] = depSlugs
		}
	}
	return
}

// parseMasterFeatureStatuses reads the ## Features table in the master .belmont/PROGRESS.md
// and returns a map of slug → status string (e.g. "Complete", "In Progress").
// New table format: | Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |
func parseMasterFeatureStatuses(root string) map[string]string {
	statuses := make(map[string]string)

	progressPath := filepath.Join(root, ".belmont", "PROGRESS.md")
	content, err := os.ReadFile(progressPath)
	if err != nil {
		return statuses
	}

	lines := strings.Split(string(content), "\n")
	inTable := false
	colIdx := parseMasterTableColumns(lines)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## Features") {
			inTable = true
			continue
		}

		if inTable && strings.HasPrefix(trimmed, "## ") {
			break
		}

		if !inTable || !strings.HasPrefix(trimmed, "|") {
			continue
		}

		cells := splitTableCells(trimmed)
		slugCol := colIdx["Slug"]
		statusCol := colIdx["Status"]
		if slugCol < 0 || statusCol < 0 || len(cells) <= slugCol || len(cells) <= statusCol {
			continue
		}

		slug := strings.TrimSpace(cells[slugCol])
		if slug == "Slug" || strings.HasPrefix(slug, "-") || strings.HasPrefix(slug, ":") {
			continue
		}

		status := strings.TrimSpace(cells[statusCol])
		statuses[slug] = status
	}
	return statuses
}

// parseMasterTableColumns finds column indices by header name in the master PROGRESS.md features table.
func parseMasterTableColumns(lines []string) map[string]int {
	result := map[string]int{
		"Feature": -1, "Slug": -1, "Priority": -1, "Dependencies": -1,
		"Status": -1, "Milestones": -1, "Tasks": -1,
	}
	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## Features") {
			inTable = true
			continue
		}
		if inTable && strings.HasPrefix(trimmed, "|") {
			cells := splitTableCells(trimmed)
			for i, c := range cells {
				c = strings.TrimSpace(c)
				if _, ok := result[c]; ok {
					result[c] = i
				}
			}
			return result
		}
	}
	// Fallback: old 6-column format or new 7-column format by position
	return result
}

// splitTableCells splits a markdown table row into cells (stripping leading/trailing pipes).
func splitTableCells(line string) []string {
	cols := strings.Split(line, "|")
	var cells []string
	for _, c := range cols {
		c = strings.TrimSpace(c)
		if c != "" {
			cells = append(cells, c)
		}
	}
	return cells
}

// syncMasterFeatureStatuses updates the ## Features table in master .belmont/PROGRESS.md
// to match computed feature-level statuses. This prevents stale master data from causing
// auto mode to skip features that still have pending work.
// New table format: | Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |
func syncMasterFeatureStatuses(root string, features []featureSummary) {
	progressPath := filepath.Join(root, ".belmont", "PROGRESS.md")
	content, err := os.ReadFile(progressPath)
	if err != nil {
		return
	}

	// Build lookup from computed features
	type computed struct {
		Status     string
		MsDone     int
		MsTotal    int
		TasksDone  int
		TasksTotal int
	}
	lookup := make(map[string]computed)
	for _, f := range features {
		lookup[f.Slug] = computed{
			Status:     f.Status,
			MsDone:     f.MilestonesDone,
			MsTotal:    f.MilestonesTotal,
			TasksDone:  f.TasksDone,
			TasksTotal: f.TasksTotal,
		}
	}

	lines := strings.Split(string(content), "\n")
	colIdx := parseMasterTableColumns(lines)
	inTable := false
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## Features") {
			inTable = true
			continue
		}
		if inTable && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if !inTable || !strings.HasPrefix(trimmed, "|") {
			continue
		}

		cells := splitTableCells(trimmed)
		slugCol := colIdx["Slug"]
		statusCol := colIdx["Status"]
		msCol := colIdx["Milestones"]
		tasksCol := colIdx["Tasks"]

		if slugCol < 0 || len(cells) <= slugCol {
			continue
		}

		slug := strings.TrimSpace(cells[slugCol])
		if slug == "Slug" || strings.HasPrefix(slug, "-") || strings.HasPrefix(slug, ":") {
			continue
		}

		c, ok := lookup[slug]
		if !ok {
			continue
		}

		newStatus := c.Status
		newMs := fmt.Sprintf("%d/%d", c.MsDone, c.MsTotal)
		newTasks := fmt.Sprintf("%d/%d", c.TasksDone, c.TasksTotal)

		cellsChanged := false
		if statusCol >= 0 && statusCol < len(cells) && cells[statusCol] != newStatus {
			cells[statusCol] = newStatus
			cellsChanged = true
		}
		if msCol >= 0 && msCol < len(cells) && cells[msCol] != newMs {
			cells[msCol] = newMs
			cellsChanged = true
		}
		if tasksCol >= 0 && tasksCol < len(cells) && cells[tasksCol] != newTasks {
			cells[tasksCol] = newTasks
			cellsChanged = true
		}

		if cellsChanged {
			var parts []string
			for _, c := range cells {
				parts = append(parts, " "+c+" ")
			}
			lines[i] = "|" + strings.Join(parts, "|") + "|"
			changed = true
		}
	}

	if changed {
		os.WriteFile(progressPath, []byte(strings.Join(lines, "\n")), 0644)
	}
}

// populateFeatureDeps enriches feature summaries with dependency and priority info from master PROGRESS.md.
func populateFeatureDeps(features []featureSummary, root string) {
	deps, priorities := parseMasterDeps(root)
	for i := range features {
		if d, ok := deps[features[i].Slug]; ok {
			features[i].Deps = d
		}
		if p, ok := priorities[features[i].Slug]; ok {
			features[i].Priority = p
		}
	}
}

func computeFeatureListStatus(features []featureSummary) string {
	allVerified := true
	allComplete := true
	anyProgress := false
	for _, f := range features {
		if f.Status != "Verified" {
			allVerified = false
		}
		if f.Status != "Complete" && f.Status != "Verified" {
			allComplete = false
		}
		if f.TasksDone > 0 || f.TasksInProgress > 0 {
			anyProgress = true
		}
	}
	if allVerified && len(features) > 0 {
		return "Verified"
	}
	if allComplete && len(features) > 0 {
		return "Complete"
	}
	if anyProgress {
		return "In Progress"
	}
	return "Not Started"
}

func extractFeatureName(prd string) string {
	re := regexp.MustCompile(`(?m)^#\s*PRD:\s*(.+)$`)
	match := re.FindStringSubmatch(prd)
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return "Unknown"
}

// flattenTasks extracts all tasks from parsed milestones, sorted by task ID.
func flattenTasks(milestones []milestone, maxName int) []task {
	var tasks []task
	for _, m := range milestones {
		for _, t := range m.Tasks {
			name := t.Name
			if maxName > 0 && len([]rune(name)) > maxName {
				name = string([]rune(name)[:maxName-1]) + "…"
			}
			tasks = append(tasks, task{ID: t.ID, Name: name, Status: t.Status, MilestoneID: t.MilestoneID})
		}
	}

	sort.Slice(tasks, func(i, j int) bool {
		pi, ni := parseTaskOrder(tasks[i].ID)
		pj, nj := parseTaskOrder(tasks[j].ID)
		if pi != pj {
			return pi < pj
		}
		return ni < nj
	})

	return tasks
}


func parseTaskOrder(id string) (int, int) {
	re := regexp.MustCompile(`^P(\d+)-(\d+)`)
	match := re.FindStringSubmatch(id)
	if len(match) != 3 {
		return 99, 99
	}
	return atoiDefault(match[1], 99), atoiDefault(match[2], 99)
}

func lastCompletedTask(tasks []task) *task {
	var last *task
	for i := range tasks {
		if tasks[i].Status == taskDone || tasks[i].Status == taskVerified {
			t := tasks[i]
			last = &t
		}
	}
	return last
}

func parseMilestones(progress string) []milestone {
	// Match milestone headers: ### M1: Name or ### ✅ M1: Name (legacy) or ### ⬜ M1: Name (legacy)
	msRe := regexp.MustCompile(`(?m)^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):\s*(.+)$`)
	depsRe := regexp.MustCompile(`\(depends:\s*(M[\d]+(?:\s*,\s*M[\d]+)*)\)\s*$`)
	// Match task checkboxes: - [ ] P0-1: Task Name, - [x] ..., - [>] ..., - [v] ..., - [!] ...
	taskRe := regexp.MustCompile(`(?m)^\s*-\s+\[(.)\]\s+(.+)$`)

	lines := strings.Split(progress, "\n")
	var milestones []milestone
	var currentMS *milestone

	for _, line := range lines {
		// Check for milestone header
		if msMatch := msRe.FindStringSubmatch(line); len(msMatch) >= 3 {
			// Save previous milestone
			if currentMS != nil {
				milestones = append(milestones, *currentMS)
			}

			id := "M" + strings.TrimSpace(msMatch[1])
			name := strings.TrimSpace(msMatch[2])

			// Extract dependency annotations from name
			var deps []string
			if depsMatch := depsRe.FindStringSubmatch(name); len(depsMatch) >= 2 {
				name = strings.TrimSpace(depsRe.ReplaceAllString(name, ""))
				for _, d := range strings.Split(depsMatch[1], ",") {
					deps = append(deps, strings.TrimSpace(d))
				}
			}

			currentMS = &milestone{ID: id, Name: name, Deps: deps}
			continue
		}

		// Check for next section (## header) — stops current milestone
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			if currentMS != nil {
				milestones = append(milestones, *currentMS)
				currentMS = nil
			}
			continue
		}

		// Parse task checkboxes under current milestone
		if currentMS != nil {
			if taskMatch := taskRe.FindStringSubmatch(line); len(taskMatch) >= 3 {
				marker := taskMatch[1]
				taskText := strings.TrimSpace(taskMatch[2])

				var status taskStatus
				switch marker {
				case " ":
					status = taskTodo
				case ">":
					status = taskInProgress
				case "x":
					status = taskDone
				case "v":
					status = taskVerified
				case "!":
					status = taskBlocked
				default:
					status = taskTodo
				}

				// Extract task ID if present (e.g., "P0-1: Task Name")
				taskID := ""
				taskName := taskText
				idRe := regexp.MustCompile(`^(P\d+-[\w][\w-]*):\s*(.+)$`)
				if idMatch := idRe.FindStringSubmatch(taskText); len(idMatch) >= 3 {
					taskID = idMatch[1]
					taskName = strings.TrimSpace(idMatch[2])
				}

				currentMS.Tasks = append(currentMS.Tasks, task{
					ID:          taskID,
					Name:        taskName,
					Status:      status,
					MilestoneID: currentMS.ID,
				})
			}
		}
	}

	// Don't forget the last milestone
	if currentMS != nil {
		milestones = append(milestones, *currentMS)
	}

	return milestones
}

func nextMilestone(milestones []milestone) *milestone {
	for _, m := range milestones {
		if !milestoneAllDone(m) {
			mm := m
			return &mm
		}
	}
	return nil
}

func nextTask(tasks []task) *task {
	for _, t := range tasks {
		if t.Status == taskInProgress || t.Status == taskTodo {
			tt := t
			return &tt
		}
	}
	return nil
}

// blockedTaskCount returns the number of tasks with [!] status across all milestones.
func blockedTaskCount(milestones []milestone) int {
	count := 0
	for _, m := range milestones {
		for _, t := range m.Tasks {
			if t.Status == taskBlocked {
				count++
			}
		}
	}
	return count
}

// blockedTaskNames returns descriptions of blocked tasks for display.
func blockedTaskNames(milestones []milestone) []string {
	var names []string
	for _, m := range milestones {
		for _, t := range m.Tasks {
			if t.Status == taskBlocked {
				label := t.Name
				if t.ID != "" {
					label = t.ID + ": " + t.Name
				}
				names = append(names, label)
			}
		}
	}
	return names
}

func parseDecisions(progress string, limit int) []string {
	lines := parseSectionLines(progress, "## Decisions Log")
	if len(lines) <= limit {
		return lines
	}
	return lines[len(lines)-limit:]
}

func parseSectionLines(doc, header string) []string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(header) + `\s*$`)
	loc := re.FindStringIndex(doc)
	if loc == nil {
		return nil
	}
	rest := doc[loc[1]:]
	lines := strings.Split(rest, "\n")
	var results []string
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		if trimmed == "" {
			continue
		}
		if strings.Contains(strings.ToLower(trimmed), "none") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "-")
		trimmed = strings.TrimPrefix(trimmed, "*")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed != "" {
			results = append(results, trimmed)
		}
	}
	return results
}

func techPlanReady(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(content)) != ""
}

// fileHasRealContent checks if a file exists and has content beyond template/placeholder text.
func fileHasRealContent(path string) bool {
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return false
	}
	// Check for known template/placeholder texts
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "run /belmont:") || strings.HasPrefix(lower, "run the /belmont:") {
		return false
	}
	return true
}

func computeOverallStatus(tasks []task) string {
	if len(tasks) == 0 {
		return "Not Started"
	}

	allVerified := true
	allDone := true
	anyProgress := false
	allBlocked := true

	for _, t := range tasks {
		if t.Status != taskVerified {
			allVerified = false
		}
		if t.Status != taskDone && t.Status != taskVerified {
			allDone = false
		}
		if t.Status == taskDone || t.Status == taskVerified || t.Status == taskInProgress {
			anyProgress = true
		}
		if t.Status != taskBlocked {
			allBlocked = false
		}
	}

	if allVerified {
		return "Verified"
	}
	if allDone {
		return "Complete"
	}
	if allBlocked {
		return "BLOCKED"
	}
	if anyProgress {
		return "In Progress"
	}
	return "Not Started"
}


func renderStatus(report statusReport, color bool) string {
	// Feature listing mode (default when no --feature specified)
	if report.Features != nil {
		return renderFeatureListing(report, color)
	}

	techPlan := "Not written (run /belmont:tech-plan to create)"
	if report.TechPlanReady {
		techPlan = "Ready"
	}

	taskLine := fmt.Sprintf("Tasks: %d verified, %d done, %d in progress, %d blocked, %d todo (of %d total)",
		report.TaskCounts["verified"],
		report.TaskCounts["done"],
		report.TaskCounts["in_progress"],
		report.TaskCounts["blocked"],
		report.TaskCounts["todo"],
		report.TaskCounts["total"],
	)

	bold := func(s string) string {
		if color {
			return ansiBold + s + ansiReset
		}
		return s
	}

	var sb strings.Builder
	sb.WriteString(bold("Belmont Status") + "\n")
	sb.WriteString("==============\n\n")
	sb.WriteString(fmt.Sprintf("Feature: %s\n\n", report.Feature))
	sb.WriteString(fmt.Sprintf("Tech Plan: %s\n\n", techPlan))
	sb.WriteString(fmt.Sprintf("Status: %s\n\n", colorStatus(report.OverallStatus, color)))
	sb.WriteString(taskLine)
	sb.WriteString("\n\n")

	if len(report.Tasks) > 0 {
		for _, t := range report.Tasks {
			sb.WriteString(fmt.Sprintf("  %s %s: %s\n", taskStatusIcon(t.Status, color), t.ID, t.Name))
		}
	}
	sb.WriteString("\n")

	sb.WriteString("Milestones:\n")
	if len(report.Milestones) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, m := range report.Milestones {
			icon := milestoneStatusIcon(m, color)
			sb.WriteString(fmt.Sprintf("  %s %s: %s\n", icon, m.ID, m.Name))
		}
	}
	sb.WriteString("\n")

	blocked := blockedTaskNames(report.Milestones)
	if len(blocked) > 0 {
		sb.WriteString("Blocked Tasks:\n")
		for _, b := range blocked {
			sb.WriteString(fmt.Sprintf("  - %s\n", b))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Next Milestone:\n")
	if report.NextMilestone == nil {
		sb.WriteString("  - None\n")
	} else {
		sb.WriteString(fmt.Sprintf("  - %s - %s\n", report.NextMilestone.ID, report.NextMilestone.Name))
	}
	sb.WriteString("Next Individual Task:\n")
	if report.NextTask == nil {
		sb.WriteString("  - None\n")
	} else {
		sb.WriteString(fmt.Sprintf("  - %s - %s\n", report.NextTask.ID, report.NextTask.Name))
	}
	sb.WriteString("\n")

	sb.WriteString("Recent Activity:\n")
	sb.WriteString("---\n")
	if report.LastCompleted == nil {
		sb.WriteString("Last completed: None\n")
	} else {
		sb.WriteString(fmt.Sprintf("Last completed: %s - %s\n", report.LastCompleted.ID, report.LastCompleted.Name))
	}
	sb.WriteString("Recent decisions:\n")
	if len(report.RecentDecisions) == 0 {
		sb.WriteString("  - None\n")
	} else {
		for _, d := range report.RecentDecisions {
			sb.WriteString(fmt.Sprintf("  - %s\n", d))
		}
	}
	sb.WriteString(statusLegend(color))
	return sb.String()
}

func milestoneStatusIcon(m milestone, color bool) string {
	if milestoneAllVerified(m) {
		if color {
			return ansiGreen + "[v]" + ansiReset
		}
		return "[v]"
	} else if milestoneAllDone(m) {
		if color {
			return ansiCyan + "[x]" + ansiReset
		}
		return "[x]"
	} else if milestoneHasBlockers(m) {
		if color {
			return ansiRed + "[!]" + ansiReset
		}
		return "[!]"
	} else if !milestoneNotStarted(m) {
		if color {
			return ansiYellow + "[>]" + ansiReset
		}
		return "[>]"
	}
	if color {
		return ansiDim + "[ ]" + ansiReset
	}
	return "[ ]"
}

func colorStatus(status string, color bool) string {
	if !color {
		return status
	}
	switch status {
	case "Verified":
		return ansiGreen + status + ansiReset
	case "Complete":
		return ansiCyan + status + ansiReset
	case "In Progress":
		return ansiYellow + status + ansiReset
	case "BLOCKED":
		return ansiRed + status + ansiReset
	case "Not Started":
		return ansiDim + status + ansiReset
	default:
		return status
	}
}

func statusLegend(color bool) string {
	if !color {
		return "\nLegend: [v] verified  [x] done  [>] in progress  [!] blocked  [ ] todo\n"
	}
	return fmt.Sprintf("\nLegend: %s[v]%s verified  %s[x]%s done  %s[>]%s in progress  %s[!]%s blocked  %s[ ]%s todo\n",
		ansiGreen, ansiReset, ansiCyan, ansiReset, ansiYellow, ansiReset, ansiRed, ansiReset, ansiDim, ansiReset)
}

func featureStatusIcon(status string, color bool) string {
	bracket := "[ ]"
	switch status {
	case "Verified":
		bracket = "[v]"
	case "Complete":
		bracket = "[x]"
	case "In Progress":
		bracket = "[>]"
	}
	if !color {
		return bracket
	}
	switch status {
	case "Verified":
		return ansiGreen + bracket + ansiReset
	case "Complete":
		return ansiCyan + bracket + ansiReset
	case "In Progress":
		return ansiYellow + bracket + ansiReset
	default:
		return ansiDim + bracket + ansiReset
	}
}

func renderFeatureListing(report statusReport, color bool) string {
	prfaq := "Not written (run /belmont:working-backwards)"
	if report.PRFAQReady {
		prfaq = "Written"
	}
	techPlan := "Not written"
	if report.TechPlanReady {
		techPlan = "Ready"
	}

	bold := func(s string) string {
		if color {
			return ansiBold + s + ansiReset
		}
		return s
	}

	var sb strings.Builder
	sb.WriteString(bold("Belmont Status") + "\n")
	sb.WriteString("==============\n\n")
	sb.WriteString(fmt.Sprintf("Product: %s\n\n", report.Feature))
	sb.WriteString(fmt.Sprintf("PR/FAQ: %s\n", prfaq))
	sb.WriteString(fmt.Sprintf("Master Tech Plan: %s\n\n", techPlan))
	sb.WriteString(fmt.Sprintf("Status: %s\n\n", colorStatus(report.OverallStatus, color)))

	if len(report.Features) == 0 {
		sb.WriteString("Features:\n")
		sb.WriteString("  (none — run /belmont:product-plan to create your first feature)\n")
	} else {
		for _, f := range report.Features {
			icon := featureStatusIcon(f.Status, color)
			sb.WriteString(fmt.Sprintf("%s %s (%s)\n", icon, f.Name, f.Slug))
			sb.WriteString(fmt.Sprintf("  Tasks: %d/%d done", f.TasksDone, f.TasksTotal))
			if f.TasksVerified > 0 {
				sb.WriteString(fmt.Sprintf(" (%d verified)", f.TasksVerified))
			}
			if f.MilestonesTotal > 0 {
				sb.WriteString(fmt.Sprintf("  |  Milestones: %d/%d done", f.MilestonesDone, f.MilestonesTotal))
			}
			sb.WriteString("\n")

			// Show milestone listing
			if len(f.Milestones) > 0 {
				for _, m := range f.Milestones {
					isNext := f.NextMilestone != nil && m.ID == f.NextMilestone.ID
					mIcon := milestoneStatusIcon(m, color)
					if milestoneNotStarted(m) && isNext {
						if color {
							mIcon = ansiYellow + "[>]" + ansiReset
						} else {
							mIcon = "[>]"
						}
					}
					sb.WriteString(fmt.Sprintf("    %s %s: %s\n", mIcon, m.ID, m.Name))
				}
			}

			// Show next task if feature is in progress
			if f.NextTask != nil && f.Status == "In Progress" {
				sb.WriteString(fmt.Sprintf("  Next: %s — %s\n", f.NextTask.ID, f.NextTask.Name))
			}

			// Show blocked tasks if any
			if f.TasksBlocked > 0 {
				blockedNames := blockedTaskNames(f.Milestones)
				sb.WriteString("  Blocked:\n")
				for _, b := range blockedNames {
					sb.WriteString(fmt.Sprintf("    - %s\n", b))
				}
			}

			sb.WriteString("\n")
		}
	}

	sb.WriteString("Use --feature <slug> for detailed task-level status.\n")
	sb.WriteString(statusLegend(color))
	return sb.String()
}

// ANSI color codes for terminal output
const (
	ansiReset   = "\033[0m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiRed     = "\033[31m"
	ansiCyan    = "\033[36m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
)

func taskStatusIcon(status taskStatus, color bool) string {
	switch status {
	case taskVerified:
		if color {
			return ansiGreen + "[v]" + ansiReset
		}
		return "[v]"
	case taskDone:
		if color {
			return ansiCyan + "[x]" + ansiReset
		}
		return "[x]"
	case taskInProgress:
		if color {
			return ansiYellow + "[>]" + ansiReset
		}
		return "[>]"
	case taskBlocked:
		if color {
			return ansiRed + "[!]" + ansiReset
		}
		return "[!]"
	default:
		if color {
			return ansiDim + "[ ]" + ansiReset
		}
		return "[ ]"
	}
}

type toolConfig struct {
	Name  string
	Label string
}

var toolConfigs = []toolConfig{
	{Name: "claude", Label: "Claude Code (.claude/)"},
	{Name: "codex", Label: "Codex (.codex/)"},
	{Name: "cursor", Label: "Cursor (.cursor/)"},
	{Name: "windsurf", Label: "Windsurf (.windsurf/)"},
	{Name: "gemini", Label: "Gemini (.gemini/)"},
	{Name: "copilot", Label: "GitHub Copilot (.copilot/)"},
}

func runInstall(args []string) error {
	fsFlags := flag.NewFlagSet("install", flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard)
	var source string
	var project string
	var toolsFlag string
	var noPrompt bool
	fsFlags.StringVar(&source, "source", "", "belmont source directory")
	fsFlags.StringVar(&project, "project", ".", "project directory")
	fsFlags.StringVar(&toolsFlag, "tools", "", "all|none|comma list")
	fsFlags.BoolVar(&noPrompt, "no-prompt", false, "disable interactive prompts")
	if err := fsFlags.Parse(args); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	projectRoot, err := filepath.Abs(project)
	if err != nil {
		return err
	}

	// Determine mode: embedded (release binary) vs source (developer)
	useEmbedded := (source == "" && os.Getenv("BELMONT_SOURCE") == "") && hasEmbeddedFiles

	fmt.Println("Belmont Project Setup")
	fmt.Println("=====================")
	fmt.Println("")
	fmt.Printf("Project: %s\n", projectRoot)
	fmt.Println("")

	selectedTools, err := resolveTools(projectRoot, toolsFlag, noPrompt)
	if err != nil {
		return err
	}

	if useEmbedded {
		fmt.Println("Installing agents to .agents/belmont/...")
		if err := syncEmbeddedDir(embeddedAgents, "agents/belmont", filepath.Join(projectRoot, ".agents", "belmont")); err != nil {
			return err
		}
		fmt.Println("")

		fmt.Println("Installing skills to .agents/skills/belmont/...")
		if err := syncEmbeddedDir(embeddedSkills, "skills/belmont", filepath.Join(projectRoot, ".agents", "skills", "belmont")); err != nil {
			return err
		}
		fmt.Println("")
	} else {
		sourceRoot, err := resolveSourceRoot(source)
		if err != nil {
			return err
		}

		agentsSource := filepath.Join(sourceRoot, "agents", "belmont")
		skillsSource := filepath.Join(sourceRoot, "skills", "belmont")

		if !dirExists(agentsSource) || !dirExists(skillsSource) {
			return fmt.Errorf("install: source missing agents/ or skills/ in %s", sourceRoot)
		}

		fmt.Println("Installing agents to .agents/belmont/...")
		if err := syncMarkdownDir(agentsSource, filepath.Join(projectRoot, ".agents", "belmont")); err != nil {
			return err
		}
		fmt.Println("")

		fmt.Println("Installing skills to .agents/skills/belmont/...")
		if err := syncMarkdownDir(skillsSource, filepath.Join(projectRoot, ".agents", "skills", "belmont")); err != nil {
			return err
		}
		fmt.Println("")
	}

	if containsTool(selectedTools, "codex") {
		fmt.Println("Updating AGENTS.md for Codex skill routing...")
		if changed, err := ensureCodexAgentsGuidance(projectRoot); err != nil {
			return err
		} else if changed {
			fmt.Println("  + AGENTS.md Belmont Codex skill routing section")
		} else {
			fmt.Println("  = AGENTS.md Belmont Codex skill routing section (unchanged)")
		}
		if removed, err := removeLegacyCodexSkillsIndex(projectRoot); err != nil {
			return err
		} else if removed {
			fmt.Println("  - SKILLS.md (removed legacy Belmont Codex index)")
		}
		fmt.Println("")
	}

	for _, tool := range selectedTools {
		if err := setupTool(projectRoot, tool); err != nil {
			return err
		}
		fmt.Println("")
	}

	if len(selectedTools) == 0 {
		fmt.Println("Skipped tool linking.")
		fmt.Println("Skills are in .agents/skills/belmont/ -- reference them from your tool.")
		fmt.Println("")
	}

	if err := ensureStateFiles(projectRoot); err != nil {
		return err
	}

	fmt.Println("")
	fmt.Println("Belmont installed!")
	fmt.Println("")
	fmt.Println("Agents:  .agents/belmont/")
	fmt.Println("Skills:  .agents/skills/belmont/")
	fmt.Println("State:   .belmont/")
	if fileExists(filepath.Join(projectRoot, ".belmont", "bin", "belmont")) || fileExists(filepath.Join(projectRoot, ".belmont", "bin", "belmont.exe")) {
		fmt.Println("Helper:  .belmont/bin/belmont")
	}

	if len(selectedTools) > 0 {
		fmt.Println("")
		fmt.Println("Tool integrations:")
		for _, tool := range selectedTools {
			switch tool {
			case "claude":
				fmt.Println("  Claude Code  .claude/agents/belmont -> ../../.agents/belmont")
				fmt.Println("              .claude/commands/belmont (copied from .agents/skills/belmont)")
				fmt.Println("    Use: /belmont:working-backwards, /belmont:product-plan, /belmont:tech-plan, /belmont:implement, /belmont:next, /belmont:verify, /belmont:debug, /belmont:debug-auto, /belmont:debug-manual, /belmont:status")
			case "codex":
				fmt.Println("  Codex        .codex/belmont (copied from .agents/skills/belmont)")
				fmt.Println("    Use: AGENTS.md includes Belmont skill routing for belmont:<skill> prompts")
			case "cursor":
				fmt.Println("  Cursor       .cursor/rules/belmont/*.mdc -> .agents/skills/belmont/*.md")
				fmt.Println("    Use: Reference belmont rules in Composer/Agent, or toggle in Settings > Rules")
			case "windsurf":
				fmt.Println("  Windsurf     .windsurf/rules/belmont -> .agents/skills/belmont")
				fmt.Println("    Use: Reference belmont rules in Cascade")
			case "gemini":
				fmt.Println("  Gemini       .gemini/rules/belmont -> .agents/skills/belmont")
				fmt.Println("    Use: Reference belmont rules in Gemini")
			case "copilot":
				fmt.Println("  Copilot      .copilot/belmont -> .agents/skills/belmont")
				fmt.Println("    Use: Reference belmont files in Copilot Chat")
			}
		}
	}

	fmt.Println("")
	fmt.Println("Workflow:")
	fmt.Println("  0. PR/FAQ     - Define product vision (Working Backwards)")
	fmt.Println("  1. Plan       - Create PRD interactively")
	fmt.Println("  2. Tech Plan  - Create technical implementation plan")
	fmt.Println("  3. Implement  - Implement next milestone (full pipeline)")
	fmt.Println("  4. Next       - Implement next single task (lightweight)")
	fmt.Println("  5. Verify     - Run verification and code review")
	fmt.Println("  6. Status     - View progress")
	fmt.Println("  7. Reset      - Reset state and start fresh")
	fmt.Println("  8. Cleanup    - Archive completed features, reduce token bloat")
	fmt.Println("")

	// Hint about worktree auto-install if project has a lockfile but no worktree config
	worktreeJSON := filepath.Join(projectRoot, ".belmont", "worktree.json")
	if _, err := os.Stat(worktreeJSON); os.IsNotExist(err) {
		lockfiles := []string{"pnpm-lock.yaml", "bun.lockb", "bun.lock", "yarn.lock", "package-lock.json", "Gemfile.lock", "requirements.txt", "Cargo.lock"}
		for _, lf := range lockfiles {
			if _, err := os.Stat(filepath.Join(projectRoot, lf)); err == nil {
				fmt.Printf("Note: Worktree dependencies will be auto-installed (%s detected).\n", lf)
				fmt.Println("      Create .belmont/worktree.json to customize setup hooks, teardown, or env vars.")
				fmt.Println("")
				break
			}
		}
	}

	return nil
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func resolveSourceRoot(source string) (string, error) {
	if source != "" {
		abs, err := filepath.Abs(source)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	if env := strings.TrimSpace(os.Getenv("BELMONT_SOURCE")); env != "" {
		abs, err := filepath.Abs(env)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	if cfgSource, ok := loadConfigSource(); ok {
		abs, err := filepath.Abs(cfgSource)
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exePath)
	for i := 0; i < 6; i++ {
		skills := filepath.Join(dir, "skills", "belmont")
		agents := filepath.Join(dir, "agents", "belmont")
		if dirExists(skills) && dirExists(agents) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", errors.New("install: unable to locate belmont source; pass --source PATH")
}

func loadConfigSource() (string, bool) {
	paths := configPaths()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg config
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}
		if strings.TrimSpace(cfg.Source) != "" {
			return cfg.Source, true
		}
	}
	return "", false
}

func configPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	paths := []string{
		filepath.Join(home, ".config", "belmont", "config.json"),
		filepath.Join(home, ".belmont", "config.json"),
	}
	return paths
}

func resolveTools(projectRoot, toolsFlag string, noPrompt bool) ([]string, error) {
	detected := detectTools(projectRoot)

	if toolsFlag != "" {
		switch strings.ToLower(toolsFlag) {
		case "all":
			if len(detected) > 0 {
				return detected, nil
			}
			return allToolNames(), nil
		case "none":
			return nil, nil
		default:
			parts := strings.Split(toolsFlag, ",")
			var selected []string
			for _, p := range parts {
				name := strings.TrimSpace(strings.ToLower(p))
				if name == "" {
					continue
				}
				if !isKnownTool(name) {
					return nil, fmt.Errorf("install: unknown tool %q", name)
				}
				selected = append(selected, name)
			}
			return selected, nil
		}
	}

	if noPrompt {
		if len(detected) > 0 {
			return detected, nil
		}
		return nil, nil
	}

	reader := bufio.NewReader(os.Stdin)

	if len(detected) > 0 {
		fmt.Println("Detected AI tools:")
		for i, tool := range detected {
			fmt.Printf("  [%d] %s\n", i+1, toolLabel(tool))
		}
		fmt.Println("")
		fmt.Println("Install skills for:")
		fmt.Println("  [a] All detected tools")
		for i, tool := range detected {
			fmt.Printf("  [%d] %s only\n", i+1, toolLabel(tool))
		}
		fmt.Println("  [s] Skip (install agents only)")
		fmt.Println("")
		fmt.Print("Choice [a]: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		fmt.Println("")

		if choice == "" || strings.EqualFold(choice, "a") {
			return detected, nil
		}
		if strings.EqualFold(choice, "s") {
			return nil, nil
		}
		if idx, err := strconv.Atoi(choice); err == nil {
			if idx >= 1 && idx <= len(detected) {
				return []string{detected[idx-1]}, nil
			}
		}
		return detected, nil
	}

	fmt.Println("No AI tool directories detected.")
	fmt.Println("")
	fmt.Println("Which tool are you using?")
	for i, tool := range toolConfigs {
		fmt.Printf("  [%d] %s\n", i+1, tool.Label)
	}
	fmt.Println("  [s] Skip (install agents only - reference files manually)")
	fmt.Println("")
	fmt.Print("Choice: ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)
	fmt.Println("")

	if strings.EqualFold(choice, "s") {
		return nil, nil
	}
	if idx, err := strconv.Atoi(choice); err == nil {
		if idx >= 1 && idx <= len(toolConfigs) {
			return []string{toolConfigs[idx-1].Name}, nil
		}
	}

	fmt.Println("Invalid choice. Installing agents only.")
	return nil, nil
}

func detectTools(projectRoot string) []string {
	var detected []string
	if dirExists(filepath.Join(projectRoot, ".claude")) {
		detected = append(detected, "claude")
	}
	if dirExists(filepath.Join(projectRoot, ".codex")) {
		detected = append(detected, "codex")
	}
	if dirExists(filepath.Join(projectRoot, ".cursor")) {
		detected = append(detected, "cursor")
	}
	if dirExists(filepath.Join(projectRoot, ".windsurf")) {
		detected = append(detected, "windsurf")
	}
	if dirExists(filepath.Join(projectRoot, ".gemini")) {
		detected = append(detected, "gemini")
	}
	if dirExists(filepath.Join(projectRoot, ".copilot")) {
		detected = append(detected, "copilot")
	}
	return detected
}

func allToolNames() []string {
	names := make([]string, 0, len(toolConfigs))
	for _, tool := range toolConfigs {
		names = append(names, tool.Name)
	}
	return names
}

func isKnownTool(name string) bool {
	for _, tool := range toolConfigs {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func toolLabel(name string) string {
	for _, tool := range toolConfigs {
		if tool.Name == name {
			return tool.Label
		}
	}
	return name
}

func syncMarkdownDir(sourceDir, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == "SKILL.md" {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		src := filepath.Join(sourceDir, entry.Name())
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			same, err := filesEqual(src, dest)
			if err != nil {
				return err
			}
			if same {
				fmt.Printf("  = %s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ %s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + %s\n", entry.Name())
		}
		if err := copyFile(src, dest); err != nil {
			return err
		}
	}

	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - %s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	// Sync the references/ subdir if present (progressive-disclosure detail
	// loaded on demand by skills via relative paths).
	refsSrc := filepath.Join(sourceDir, "references")
	refsDest := filepath.Join(targetDir, "references")
	if dirExists(refsSrc) {
		if err := syncReferencesDir(refsSrc, refsDest); err != nil {
			return err
		}
	} else if dirExists(refsDest) {
		fmt.Println("  - references/ (removed, no longer in source)")
		if err := os.RemoveAll(refsDest); err != nil {
			return err
		}
	}

	return nil
}

// syncReferencesDir mirrors .md files in a references/ subdirectory.
func syncReferencesDir(sourceDir, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		src := filepath.Join(sourceDir, entry.Name())
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			same, err := filesEqual(src, dest)
			if err != nil {
				return err
			}
			if same {
				fmt.Printf("  = references/%s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ references/%s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + references/%s\n", entry.Name())
		}
		if err := copyFile(src, dest); err != nil {
			return err
		}
	}

	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - references/%s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

// removeLegacySyncHook removes the belmont sync PostToolUse hook from Claude Code settings
// if it was installed by a previous version of belmont. The sync hook is no longer needed —
// with copy-based worktree isolation, belmont status reads live state from active worktrees
// via auto.json.
func removeLegacySyncHook(projectRoot string) {
	settingsPath := filepath.Join(projectRoot, ".claude", "settings.json")

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		return
	}

	PostToolUse, ok := hooks["PostToolUse"].([]interface{})
	if !ok {
		return
	}

	// Filter out any entry containing "belmont sync"
	var filtered []interface{}
	removed := false
	for _, entry := range PostToolUse {
		keep := true
		if m, ok := entry.(map[string]interface{}); ok {
			if hooksArr, ok := m["hooks"].([]interface{}); ok {
				for _, h := range hooksArr {
					if hm, ok := h.(map[string]interface{}); ok {
						if cmd, _ := hm["command"].(string); strings.Contains(cmd, "belmont sync") {
							keep = false
							removed = true
						}
					}
				}
			}
			if cmd, _ := m["command"].(string); strings.Contains(cmd, "belmont sync") {
				keep = false
				removed = true
			}
		}
		if keep {
			filtered = append(filtered, entry)
		}
	}

	if !removed {
		return
	}

	if len(filtered) == 0 {
		delete(hooks, "PostToolUse")
		if len(hooks) == 0 {
			delete(settings, "hooks")
		}
	} else {
		hooks["PostToolUse"] = filtered
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(settings); err != nil {
		return
	}
	os.WriteFile(settingsPath, buf.Bytes(), 0644)
	fmt.Println("  - .claude/settings.json (removed legacy belmont sync hook)")
}

func setupTool(projectRoot, tool string) error {
	switch tool {
	case "claude":
		fmt.Println("Linking Claude Code...")
		skillsTarget := filepath.Join(projectRoot, ".agents", "skills", "belmont")
		agentsTarget := filepath.Join(projectRoot, ".agents", "belmont")
		linkAgents := filepath.Join(projectRoot, ".claude", "agents", "belmont")
		if err := ensureSymlink(linkAgents, agentsTarget, true); err != nil {
			return err
		}
		claudeCommandsDir := filepath.Join(projectRoot, ".claude", "commands", "belmont")
		if err := syncMarkdownDir(skillsTarget, claudeCommandsDir); err != nil {
			return err
		}
		claudeSkillsDir := filepath.Join(projectRoot, ".claude", "skills", "belmont")
		if dirExists(claudeSkillsDir) {
			fmt.Println("  - .claude/skills/belmont (deprecated, removing)")
			if err := os.RemoveAll(claudeSkillsDir); err != nil {
				return err
			}
		}
		// Remove legacy belmont sync hook if present (no longer needed)
		removeLegacySyncHook(projectRoot)
	case "codex":
		fmt.Println("Linking Codex...")
		skillsTarget := filepath.Join(projectRoot, ".agents", "skills", "belmont")
		codexDir := filepath.Join(projectRoot, ".codex", "belmont")
		if err := syncMarkdownDir(skillsTarget, codexDir); err != nil {
			return err
		}
	case "windsurf":
		fmt.Println("Linking Windsurf...")
		target := filepath.Join(projectRoot, ".agents", "skills", "belmont")
		link := filepath.Join(projectRoot, ".windsurf", "rules", "belmont")
		if err := ensureSymlink(link, target, true); err != nil {
			return err
		}
	case "gemini":
		fmt.Println("Linking Gemini...")
		target := filepath.Join(projectRoot, ".agents", "skills", "belmont")
		link := filepath.Join(projectRoot, ".gemini", "rules", "belmont")
		if err := ensureSymlink(link, target, true); err != nil {
			return err
		}
	case "copilot":
		fmt.Println("Linking GitHub Copilot...")
		target := filepath.Join(projectRoot, ".agents", "skills", "belmont")
		link := filepath.Join(projectRoot, ".copilot", "belmont")
		if err := ensureSymlink(link, target, true); err != nil {
			return err
		}
	case "cursor":
		fmt.Println("Linking Cursor...")
		skillsSource := filepath.Join(projectRoot, ".agents", "skills", "belmont")
		cursorDir := filepath.Join(projectRoot, ".cursor", "rules", "belmont")
		if err := linkPerFileDir(skillsSource, cursorDir, ".md", ".mdc"); err != nil {
			return err
		}
		// Skills reference progressive-disclosure files via relative
		// `references/*.md` paths, so link the references dir alongside.
		refsSource := filepath.Join(skillsSource, "references")
		refsLink := filepath.Join(cursorDir, "references")
		if dirExists(refsSource) {
			if err := ensureSymlink(refsLink, refsSource, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureSymlink(linkPath, target string, isDir bool) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return err
	}

	// Compute a relative target from the link's directory to the target.
	// Relative symlinks resolve identically in main and in git worktrees, so
	// the symlink content is byte-identical across trees — prevents merge
	// conflicts when the same install is re-run from different worktree roots.
	// If relative computation fails (e.g. different volumes on Windows), fall
	// back to the absolute target.
	symlinkTarget := target
	if rel, err := filepath.Rel(filepath.Dir(linkPath), target); err == nil {
		symlinkTarget = rel
	}

	if existing, err := os.Lstat(linkPath); err == nil {
		if existing.Mode()&os.ModeSymlink != 0 {
			current, err := os.Readlink(linkPath)
			if err == nil && current == symlinkTarget {
				fmt.Printf("  = %s (symlink ok)\n", linkPath)
				return nil
			}
		}
		if existing.IsDir() {
			fmt.Printf("  ~ %s (replacing old directory with symlink)\n", linkPath)
			if err := os.RemoveAll(linkPath); err != nil {
				return err
			}
		} else {
			fmt.Printf("  ~ %s (replacing existing file with symlink)\n", linkPath)
			if err := os.Remove(linkPath); err != nil {
				return err
			}
		}
	}

	if err := os.Symlink(symlinkTarget, linkPath); err != nil {
		fmt.Printf("  ! symlink failed for %s (copying instead)\n", linkPath)
		if isDir {
			return copyDir(target, linkPath)
		}
		return copyFile(target, linkPath)
	}
	fmt.Printf("  + %s -> %s\n", linkPath, symlinkTarget)
	return nil
}

func linkPerFileDir(sourceDir, targetDir, sourceExt, targetExt string) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	sourceEntries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	sourceNames := make(map[string]struct{})
	for _, entry := range sourceEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), sourceExt) {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), sourceExt)
		sourceNames[name] = struct{}{}
		target := filepath.Join(sourceDir, entry.Name())
		link := filepath.Join(targetDir, name+targetExt)
		if err := ensureSymlink(link, target, false); err != nil {
			return err
		}
	}

	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), targetExt) {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), targetExt)
		if _, ok := sourceNames[name]; !ok {
			fmt.Printf("  - %s%s (removed, no longer in source)\n", name, targetExt)
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

func ensureStateFiles(projectRoot string) error {
	stateDir := filepath.Join(projectRoot, ".belmont")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}

	// Ensure auto-mode artifacts are gitignored
	ensureGitignoreEntry(projectRoot, ".belmont/auto.json")
	ensureGitignoreEntry(projectRoot, ".belmont/worktrees/")

	// Create features directory
	featuresDir := filepath.Join(stateDir, "features")
	if !dirExists(featuresDir) {
		if err := os.MkdirAll(featuresDir, 0o755); err != nil {
			return err
		}
		fmt.Println("  + .belmont/features/")
	} else {
		fmt.Println("  Exists: .belmont/features/ (keeping)")
	}

	// Create PR_FAQ.md template
	prfaqPath := filepath.Join(stateDir, "PR_FAQ.md")
	if !fileExists(prfaqPath) {
		if err := os.WriteFile(prfaqPath, []byte("Run /belmont:working-backwards to create your PR/FAQ document.\n"), 0o644); err != nil {
			return err
		}
		fmt.Println("  + .belmont/PR_FAQ.md")
	} else {
		fmt.Println("  Exists: .belmont/PR_FAQ.md (keeping)")
	}

	prdPath := filepath.Join(stateDir, "PRD.md")
	if !fileExists(prdPath) {
		if err := os.WriteFile(prdPath, []byte("Run the /belmont:product-plan skill to create a plan for your feature.\n"), 0o644); err != nil {
			return err
		}
		fmt.Println("  + .belmont/PRD.md")
	} else {
		fmt.Println("  Exists: .belmont/PRD.md (keeping)")
	}

	return nil
}

const codexAgentsGuidanceStart = "<!-- belmont:codex-skill-routing:start -->"
const codexAgentsGuidanceEnd = "<!-- belmont:codex-skill-routing:end -->"

func ensureCodexAgentsGuidance(projectRoot string) (bool, error) {
	path := filepath.Join(projectRoot, "AGENTS.md")
	current := ""
	if fileExists(path) {
		data, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		current = string(data)
	}

	section := codexAgentsGuidanceSection()
	updated, changed := upsertMarkedSection(current, codexAgentsGuidanceStart, codexAgentsGuidanceEnd, section)
	if !changed {
		return false, nil
	}

	if strings.TrimSpace(updated) == "" {
		updated = "# AGENTS\n\n" + strings.TrimSpace(section) + "\n"
	}
	return true, os.WriteFile(path, []byte(updated), 0o644)
}

func codexAgentsGuidanceSection() string {
	lines := []string{
		codexAgentsGuidanceStart,
		"## Belmont Skill Routing (Codex)",
		"",
		"- Belmont skills are local markdown files in `.agents/skills/belmont/` (and mirrored in `.codex/belmont/`).",
		"- If the user says `belmont:<skill>` or \"Use the belmont:<skill> skill\", treat it as a skill reference, not a shell command.",
		"- Load `.agents/skills/belmont/<skill>.md` first (fallback to `.codex/belmont/<skill>.md`) and follow that workflow.",
		"- Known Belmont skills: `working-backwards`, `product-plan`, `tech-plan`, `implement`, `next`, `verify`, `debug`, `debug-auto`, `debug-manual`, `status`, `reset`, `note`, `review-plans`, `cleanup`.",
		"- If a requested skill file is missing, list available files in those directories and continue with the closest matching Belmont skill.",
		codexAgentsGuidanceEnd,
	}
	return strings.Join(lines, "\n")
}

func upsertMarkedSection(content, startMarker, endMarker, section string) (string, bool) {
	newSection := strings.TrimSpace(section)
	if strings.TrimSpace(content) == "" {
		return newSection + "\n", true
	}

	start := strings.Index(content, startMarker)
	end := strings.Index(content, endMarker)
	if start >= 0 && end > start {
		end += len(endMarker)
		currentSection := strings.TrimSpace(content[start:end])
		if currentSection == newSection {
			return content, false
		}
		replacement := newSection
		if !strings.HasSuffix(replacement, "\n") {
			replacement += "\n"
		}
		updated := content[:start] + replacement + content[end:]
		return updated, updated != content
	}

	trimmed := strings.TrimRight(content, "\n")
	return trimmed + "\n\n" + newSection + "\n", true
}

func removeLegacyCodexSkillsIndex(projectRoot string) (bool, error) {
	path := filepath.Join(projectRoot, "SKILLS.md")
	if !fileExists(path) {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	content := string(data)
	if !strings.Contains(content, "name: belmont-skills-index") {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}


func filesEqual(a, b string) (bool, error) {
	ab, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	bb, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return string(ab) == string(bb), nil
}

func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(dest); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(dest); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

func ensureDir(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
	}
	return os.MkdirAll(path, 0o755)
}

func copyDir(src, dest string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func containsTool(tools []string, name string) bool {
	for _, tool := range tools {
		if tool == name {
			return true
		}
	}
	return false
}

// syncEmbeddedDir mirrors syncMarkdownDir but reads from an embed.FS.
func syncEmbeddedDir(embedFS embed.FS, root string, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := fs.ReadDir(embedFS, root)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	hasReferences := false
	for _, entry := range entries {
		if entry.IsDir() {
			if entry.Name() == "references" {
				hasReferences = true
			}
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == "SKILL.md" {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		data, err := fs.ReadFile(embedFS, root+"/"+entry.Name())
		if err != nil {
			return err
		}
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			existing, err := os.ReadFile(dest)
			if err != nil {
				return err
			}
			if string(existing) == string(data) {
				fmt.Printf("  = %s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ %s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + %s\n", entry.Name())
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
	}

	// Clean stale files
	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - %s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	// Sync references/ subdir from the embed FS.
	refsDest := filepath.Join(targetDir, "references")
	if hasReferences {
		if err := syncEmbeddedReferences(embedFS, root+"/references", refsDest); err != nil {
			return err
		}
	} else if dirExists(refsDest) {
		fmt.Println("  - references/ (removed, no longer in source)")
		if err := os.RemoveAll(refsDest); err != nil {
			return err
		}
	}

	return nil
}

// syncEmbeddedReferences mirrors the references/ subdir from an embed.FS.
func syncEmbeddedReferences(embedFS embed.FS, root string, targetDir string) error {
	if err := ensureDir(targetDir); err != nil {
		return err
	}

	entries, err := fs.ReadDir(embedFS, root)
	if err != nil {
		return err
	}

	sourceNames := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		sourceNames[entry.Name()] = struct{}{}
		data, err := fs.ReadFile(embedFS, root+"/"+entry.Name())
		if err != nil {
			return err
		}
		dest := filepath.Join(targetDir, entry.Name())
		if fileExists(dest) {
			existing, err := os.ReadFile(dest)
			if err != nil {
				return err
			}
			if string(existing) == string(data) {
				fmt.Printf("  = references/%s (unchanged)\n", entry.Name())
				continue
			}
			fmt.Printf("  ~ references/%s (updated)\n", entry.Name())
		} else {
			fmt.Printf("  + references/%s\n", entry.Name())
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
	}

	targetEntries, err := os.ReadDir(targetDir)
	if err != nil {
		return err
	}
	for _, entry := range targetEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if _, ok := sourceNames[entry.Name()]; !ok {
			fmt.Printf("  - references/%s (removed, no longer in source)\n", entry.Name())
			if err := os.Remove(filepath.Join(targetDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

// --- Update command ---

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Body    string        `json:"body"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func runUpdate(args []string) error {
	fsFlags := flag.NewFlagSet("update", flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard)
	var check bool
	var force bool
	fsFlags.BoolVar(&check, "check", false, "check for updates without installing")
	fsFlags.BoolVar(&force, "force", false, "force update even if same version")
	if err := fsFlags.Parse(args); err != nil {
		return fmt.Errorf("update: %w", err)
	}

	if Version == "dev" {
		return errors.New("update: development build detected — use git pull && scripts/build.sh to update")
	}

	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	if !force && !isNewer(release.TagName, "v"+Version) {
		fmt.Printf("Already up to date (v%s)\n", Version)
		return nil
	}

	if check {
		fmt.Printf("Update available: v%s → %s\n", Version, release.TagName)
		if release.Body != "" {
			fmt.Println("\nRelease notes:")
			fmt.Println(release.Body)
		}
		return nil
	}

	assetName := fmt.Sprintf("belmont-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("update: no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}

	if err := checkWriteAccess(filepath.Dir(exePath)); err != nil {
		return fmt.Errorf("update: cannot write to %s — try running with sudo or reinstall to ~/.local/bin", filepath.Dir(exePath))
	}

	fmt.Printf("Downloading %s...\n", assetName)
	tmpPath := exePath + ".tmp"
	if err := downloadFile(downloadURL, tmpPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("update: download failed: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// Replace self — on Windows, rename current to .old first
	if runtime.GOOS == "windows" {
		oldPath := exePath + ".old"
		os.Remove(oldPath)
		if err := os.Rename(exePath, oldPath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("update: %w", err)
		}
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("update: %w", err)
	}

	fmt.Printf("\nUpdated: v%s → %s\n", Version, release.TagName)
	if release.Body != "" {
		fmt.Println("\nRelease notes:")
		fmt.Println(release.Body)
	}

	// Migrate old state tracking format if needed
	if dirExists(filepath.Join(".", ".belmont")) {
		migrateToUnifiedTracking(".")
	}

	// Auto-install if .belmont/ exists in cwd
	if dirExists(filepath.Join(".", ".belmont")) {
		fmt.Println("\nRe-installing skills and agents...")
		cmd := exec.Command(exePath, "install", "--no-prompt", "--tools", "all")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Auto-install failed: %v\nRun 'belmont install' manually.\n", err)
		}
	} else {
		fmt.Println("\nTo update skills in a project: cd ~/your-project && belmont install")
	}

	return nil
}

func fetchLatestRelease() (*githubRelease, error) {
	url := "https://api.github.com/repos/blake-simpson/belmont/releases/latest"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach GitHub (are you offline?): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		return nil, fmt.Errorf("GitHub API rate limited — set GITHUB_TOKEN env var to authenticate")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

func downloadFile(url, dest string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func checkWriteAccess(dir string) error {
	tmp := filepath.Join(dir, ".belmont-update-check")
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	f.Close()
	os.Remove(tmp)
	return nil
}

func parseSemver(v string) (int, int, int, bool) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

func isNewer(remote, local string) bool {
	rMaj, rMin, rPat, ok1 := parseSemver(remote)
	lMaj, lMin, lPat, ok2 := parseSemver(local)
	if !ok1 || !ok2 {
		return true
	}
	if rMaj != lMaj {
		return rMaj > lMaj
	}
	if rMin != lMin {
		return rMin > lMin
	}
	return rPat > lPat
}

// ── Loop command ──

func runAutoCmd(args []string) error {
	fs := flag.NewFlagSet("auto", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfg loopConfig
	var policyStr string
	var featuresFlag string
	var allFlag bool
	fs.StringVar(&cfg.Feature, "feature", "", "feature slug (required)")
	fs.StringVar(&featuresFlag, "features", "", "comma-separated feature slugs for parallel execution")
	fs.BoolVar(&allFlag, "all", false, "run all pending features in parallel")
	fs.StringVar(&cfg.From, "from", "", "start milestone (e.g. M1)")
	fs.StringVar(&cfg.To, "to", "", "end milestone (e.g. M5)")
	fs.StringVar(&cfg.Tool, "tool", "", "CLI tool (claude|codex|gemini|copilot|cursor)")
	fs.StringVar(&policyStr, "policy", "autonomous", "checkpoint policy (autonomous|milestone|every_action)")
	fs.IntVar(&cfg.MaxIterations, "max-iterations", 50, "maximum loop iterations")
	fs.IntVar(&cfg.MaxFailures, "max-failures", 3, "consecutive failures before stopping")
	fs.IntVar(&cfg.MaxParallel, "max-parallel", 5, "max concurrent goroutines for parallel execution")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "show execution plan without running")
	fs.StringVar(&cfg.Root, "root", ".", "project root")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	// Validate mutual exclusivity
	multiFeature := featuresFlag != "" || allFlag
	if multiFeature && cfg.Feature != "" {
		return fmt.Errorf("auto: --feature cannot be combined with --features or --all")
	}
	if featuresFlag != "" && allFlag {
		return fmt.Errorf("auto: --features and --all are mutually exclusive")
	}
	if multiFeature && (cfg.From != "" || cfg.To != "") {
		return fmt.Errorf("auto: --from/--to cannot be used with --features or --all")
	}
	if !multiFeature && cfg.Feature == "" {
		return fmt.Errorf("auto: --feature is required (or use --features/--all for multi-feature mode)")
	}

	switch checkpointPolicy(policyStr) {
	case policyAutonomous, policyMilestone, policyEveryAction:
		cfg.Policy = checkpointPolicy(policyStr)
	default:
		return fmt.Errorf("auto: invalid --policy %q (use autonomous, milestone, or every_action)", policyStr)
	}

	absRoot, err := filepath.Abs(cfg.Root)
	if err != nil {
		return err
	}
	cfg.Root = absRoot

	// Auto-detect tool if not specified
	if cfg.Tool == "" {
		detected := detectTool()
		if detected == "" {
			return fmt.Errorf("auto: no supported AI tool CLI found on PATH\n\nSupported tools: claude, codex, gemini, copilot, cursor\nInstall one or use --tool to specify")
		}
		cfg.Tool = detected
	} else {
		// Validate tool name
		switch cfg.Tool {
		case "claude", "codex", "gemini", "copilot", "cursor":
			// ok
		default:
			return fmt.Errorf("auto: unsupported tool %q (use claude, codex, gemini, copilot, or cursor)", cfg.Tool)
		}
	}

	// Multi-feature mode: --features or --all
	if multiFeature {
		slugs, err := resolveFeatureSlugs(absRoot, featuresFlag, allFlag)
		if err != nil {
			return err
		}
		return runAutoMultiFeature(cfg, slugs)
	}

	// Single-feature mode
	// Verify feature directory exists
	featureDir := filepath.Join(absRoot, ".belmont", "features", cfg.Feature)
	if !dirExists(featureDir) {
		return fmt.Errorf("auto: feature %q not found at %s", cfg.Feature, featureDir)
	}

	// Load per-feature model tiers (if models.yaml exists)
	tiers, tierErr := parseModelTiers(filepath.Join(featureDir, "models.yaml"))
	if tierErr != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ failed to parse models.yaml: %s — falling back to defaults\033[0m\n", tierErr)
	}
	cfg.ModelTiers = tiers

	// Read milestones and check for dependency syntax
	progressPath := filepath.Join(absRoot, ".belmont", "features", cfg.Feature, "PROGRESS.md")
	progressContent, err := os.ReadFile(progressPath)
	if err != nil {
		return fmt.Errorf("auto: failed to read PROGRESS.md: %w", err)
	}
	milestones := parseMilestones(string(progressContent))
	inRange := milestonesInRange(milestones, cfg.From, cfg.To)

	// Interactive milestone selection when stdin is a terminal and no --from/--to
	if cfg.From == "" && cfg.To == "" && isTerminal(os.Stdin) {
		selectedFrom, selectedTo, err := interactiveMilestoneSelect(inRange)
		if err != nil {
			return err
		}
		cfg.From = selectedFrom
		cfg.To = selectedTo
	}

	if cfg.DryRun {
		fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (single-feature) — %s\033[0m\n", cfg.Feature)
		fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Policy: %s\033[0m\n", cfg.Tool, cfg.Policy)
		if cfg.From != "" || cfg.To != "" {
			fmt.Fprintf(os.Stderr, "\033[2mRange: %s → %s\033[0m\n", cfg.From, cfg.To)
		}
		fmt.Fprintf(os.Stderr, "\n\033[1mMilestones:\033[0m\n")
		for _, m := range inRange {
			status := "pending"
			if milestoneAllVerified(m) {
				status = "verified"
			} else if milestoneAllDone(m) {
				status = "done"
			} else if !milestoneNotStarted(m) {
				status = "in progress"
			}
			fmt.Fprintf(os.Stderr, "  • %s — %s [%s]\n", m.ID, m.Name, status)
		}
		fmt.Fprintln(os.Stderr)
		return nil
	}

	// Check if any milestones have explicit dependencies
	hasExplicitDeps := false
	for _, m := range inRange {
		if len(m.Deps) > 0 {
			hasExplicitDeps = true
			break
		}
	}

	if hasExplicitDeps {
		return runAutoParallel(cfg, inRange)
	}

	return runLoop(cfg)
}

// resolveFeatureSlugs resolves the list of feature slugs for multi-feature mode.
func resolveFeatureSlugs(root, featuresFlag string, allFlag bool) ([]string, error) {
	featuresDir := filepath.Join(root, ".belmont", "features")

	if allFlag {
		// Get all features, filter to pending ones
		features := listFeatures(featuresDir, 50)
		if len(features) == 0 {
			return nil, fmt.Errorf("auto: no features found in %s", featuresDir)
		}
		// Sync computed feature statuses back to master PROGRESS.md to fix drift
		syncMasterFeatureStatuses(root, features)
		var slugs []string
		for _, f := range features {
			// Skip features that are complete (computed from feature-level PRD/PROGRESS files)
			if f.Status == "Complete" {
				continue
			}
			slugs = append(slugs, f.Slug)
		}
		if len(slugs) == 0 {
			return nil, fmt.Errorf("auto: all features are already complete")
		}
		return slugs, nil
	}

	// Parse comma-separated slugs
	parts := strings.Split(featuresFlag, ",")
	var slugs []string
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s == "" {
			continue
		}
		// Verify feature directory exists
		featureDir := filepath.Join(featuresDir, s)
		if !dirExists(featureDir) {
			return nil, fmt.Errorf("auto: feature %q not found at %s", s, featureDir)
		}
		slugs = append(slugs, s)
	}
	if len(slugs) == 0 {
		return nil, fmt.Errorf("auto: no valid feature slugs provided")
	}
	return slugs, nil
}

// featureWave represents a group of features that can execute in parallel.
type featureWave struct {
	Index    int
	Features []featureSummary
}

// computeFeatureWaves groups features into waves using Kahn's algorithm for topological sort.
// Features in the same wave have all deps satisfied by prior waves.
// Already-complete features satisfy deps but don't execute.
func computeFeatureWaves(features []featureSummary) ([]featureWave, error) {
	if len(features) == 0 {
		return nil, nil
	}

	// Build slug -> feature map
	bySlug := make(map[string]featureSummary)
	for _, f := range features {
		bySlug[f.Slug] = f
	}

	// Compute in-degree for each non-complete feature
	inDegree := make(map[string]int)
	for _, f := range features {
		if f.Status == "Complete" {
			continue
		}
		count := 0
		for _, dep := range f.Deps {
			if df, ok := bySlug[dep]; ok && df.Status != "Complete" {
				count++
			}
		}
		inDegree[f.Slug] = count
	}

	var waves []featureWave
	remaining := len(inDegree)
	waveIdx := 0

	for remaining > 0 {
		// Find all features with zero in-degree
		var ready []featureSummary
		for slug, deg := range inDegree {
			if deg == 0 {
				ready = append(ready, bySlug[slug])
			}
		}

		if len(ready) == 0 {
			var cycleIDs []string
			for slug := range inDegree {
				cycleIDs = append(cycleIDs, slug)
			}
			sort.Strings(cycleIDs)
			return nil, fmt.Errorf("dependency cycle detected among features: %s", strings.Join(cycleIDs, ", "))
		}

		// Sort ready features by slug for deterministic ordering
		sort.Slice(ready, func(i, j int) bool {
			return ready[i].Slug < ready[j].Slug
		})

		waves = append(waves, featureWave{Index: waveIdx, Features: ready})
		waveIdx++

		// Remove completed features and update in-degrees
		for _, f := range ready {
			delete(inDegree, f.Slug)
			remaining--
		}
		for slug, deg := range inDegree {
			f := bySlug[slug]
			newDeg := deg
			for _, dep := range f.Deps {
				for _, completed := range ready {
					if dep == completed.Slug {
						newDeg--
					}
				}
			}
			inDegree[slug] = newDeg
		}
	}

	return waves, nil
}

// validateFeatureDeps checks for dangling dependency references and cycles.
func validateFeatureDeps(features []featureSummary, allKnown []featureSummary) error {
	// Build slug set from ALL known features so completed deps are recognized
	slugSet := make(map[string]bool)
	for _, f := range allKnown {
		slugSet[f.Slug] = true
	}

	// Check for dangling references — collect all errors
	var errs []string
	for _, f := range features {
		for _, dep := range f.Deps {
			if !slugSet[dep] {
				errs = append(errs, fmt.Sprintf("feature %q depends on %q which does not exist", f.Slug, dep))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}

	// Check for cycles by attempting wave computation
	_, err := computeFeatureWaves(features)
	return err
}

// runAutoMultiFeature orchestrates wave-based execution of multiple features.
// Features with dependencies execute after their dependencies complete.
func runAutoMultiFeature(cfg loopConfig, slugs []string) error {
	startTime := time.Now()

	// Pre-flight: ensure repo is in a clean state before starting
	if err := validateRepoState(cfg.Root); err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	// Record original branch and restore on exit if changed
	origBranch := getCurrentBranch(cfg.Root)
	defer func() {
		if origBranch != "" && origBranch != "HEAD" {
			if cur := getCurrentBranch(cfg.Root); cur != origBranch {
				fmt.Fprintf(os.Stderr, "\033[33m⚠ Branch changed from %s to %s — restoring...\033[0m\n", origBranch, cur)
				restoreCmd := exec.Command("git", "checkout", origBranch)
				restoreCmd.Dir = cfg.Root
				restoreCmd.Run()
			}
		}
	}()

	// Ensure the sibling worktree base directory exists
	os.MkdirAll(worktreeBasePath(cfg.Root), 0755)

	// Build feature summaries with dependency info
	featuresDir := filepath.Join(cfg.Root, ".belmont", "features")
	allFeatures := listFeatures(featuresDir, 50)
	populateFeatureDeps(allFeatures, cfg.Root)

	// Filter to requested slugs
	slugSet := make(map[string]bool)
	for _, s := range slugs {
		slugSet[s] = true
	}
	var features []featureSummary
	for _, f := range allFeatures {
		if slugSet[f.Slug] {
			features = append(features, f)
		}
	}

	// Validate dependencies
	if err := validateFeatureDeps(features, allFeatures); err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	// Check if any features have deps — if not, use flat parallel (original behavior)
	hasAnyDeps := false
	for _, f := range features {
		if len(f.Deps) > 0 {
			hasAnyDeps = true
			break
		}
	}

	// Compute waves
	waves, err := computeFeatureWaves(features)
	if err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	if !hasAnyDeps {
		// No dependencies — single wave with all features (original behavior)
		fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (multi-feature) — %d features\033[0m\n", len(slugs))
	} else {
		fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (multi-feature) — %d features in %d waves\033[0m\n", len(slugs), len(waves))
	}
	fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Max parallel: %d\033[0m\n", cfg.Tool, cfg.MaxParallel)

	// Print wave execution plan
	fmt.Fprintf(os.Stderr, "\n\033[1mExecution plan:\033[0m\n")
	for _, w := range waves {
		var names []string
		for _, f := range w.Features {
			names = append(names, f.Slug)
		}
		if len(waves) == 1 {
			for _, n := range names {
				fmt.Fprintf(os.Stderr, "  • %s\n", n)
			}
		} else {
			fmt.Fprintf(os.Stderr, "  Wave %d: [%s]\n", w.Index+1, strings.Join(names, ", "))
		}
	}
	fmt.Fprintln(os.Stderr)

	if cfg.DryRun {
		return nil
	}

	// Set up worktree tracker and signal handler
	activeWorktrees := &worktreeTracker{root: cfg.Root, entries: make(map[string]worktreeEntry), hooks: loadWorktreeHooks(cfg.Root)}
	sigCh := make(chan os.Signal, 1)
	notifySignals(sigCh)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ Interrupted — preserving worktrees for resume...\033[0m\n")
		activeWorktrees.gracefulShutdown(cfg.Root)
		os.Exit(1)
	}()

	type featureResult struct {
		Slug         string
		Branch       string
		WorktreePath string
		Err          error
	}

	var allFailures []featureResult
	failedSlugs := make(map[string]bool)
	totalMerged := 0

	// Execute wave by wave
	for _, w := range waves {
		// Filter out features whose deps include a failed slug
		var waveFeatures []featureSummary
		var skippedFeatures []string
		for _, f := range w.Features {
			skip := false
			for _, dep := range f.Deps {
				if failedSlugs[dep] {
					skip = true
					break
				}
			}
			if skip {
				skippedFeatures = append(skippedFeatures, f.Slug)
				failedSlugs[f.Slug] = true
			} else {
				waveFeatures = append(waveFeatures, f)
			}
		}

		for _, slug := range skippedFeatures {
			fmt.Fprintf(os.Stderr, "\033[33m⊘ %s skipped\033[0m — dependency failed\n", slug)
			allFailures = append(allFailures, featureResult{Slug: slug, Err: fmt.Errorf("dependency failed")})
		}

		if len(waveFeatures) == 0 {
			continue
		}

		if len(waves) > 1 {
			fmt.Fprintf(os.Stderr, "\n\033[1m── Wave %d ──\033[0m\n", w.Index+1)
		}

		// Resolve stale worktrees sequentially (before parallel launch) to avoid stdin races
		preResolved := make(map[string]bool) // slug -> resumed
		for _, f := range waveFeatures {
			branch := fmt.Sprintf("belmont/auto/%s", f.Slug)
			wtPath := filepath.Join(worktreeBasePath(cfg.Root), f.Slug)
			resumed, err := handleStaleWorktree(cfg.Root, f.Slug, branch, wtPath)
			if err != nil {
				return err
			}
			preResolved[f.Slug] = resumed
		}

		// Run this wave's features in parallel
		semaphore := make(chan struct{}, cfg.MaxParallel)
		var wg sync.WaitGroup
		results := make(chan featureResult, len(waveFeatures))

		for _, f := range waveFeatures {
			wg.Add(1)
			go func(slug string, resumed bool) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				branch := fmt.Sprintf("belmont/auto/%s", slug)
				wtPath := filepath.Join(worktreeBasePath(cfg.Root), slug)

				activeWorktrees.add(slug, wtPath, branch)

				fmt.Fprintf(os.Stderr, "\033[36m▶ %s\033[0m — starting in worktree\n", slug)

				err := runFeatureInWorktree(cfg, slug, branch, wtPath, activeWorktrees, resumed)
				results <- featureResult{
					Slug:         slug,
					Branch:       branch,
					WorktreePath: wtPath,
					Err:          err,
				}
			}(f.Slug, preResolved[f.Slug])
		}

		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect wave results
		var waveSuccesses []featureResult
		for r := range results {
			if r.Err != nil {
				if errors.Is(r.Err, errFeaturePaused) {
					fmt.Fprintf(os.Stderr, "\033[33m⏸ %s paused\033[0m — has unresolved blockers\n", r.Slug)
					// Don't add to failedSlugs (downstream deps may still be satisfiable)
					// Don't add to waveSuccesses (nothing to merge)
				} else {
					fmt.Fprintf(os.Stderr, "\033[31m✗ %s failed: %s\033[0m\n", r.Slug, r.Err)
					allFailures = append(allFailures, r)
					failedSlugs[r.Slug] = true
				}
			} else {
				fmt.Fprintf(os.Stderr, "\033[32m✓ %s complete\033[0m — merging...\n", r.Slug)
				waveSuccesses = append(waveSuccesses, r)
			}
		}

		// Merge this wave's successes before proceeding to next wave
		// State is NOT committed before merge — only after successful merge.
		// This prevents "phantom completion" where state says complete but code never merged.
		for i, s := range waveSuccesses {
			// Ensure repo is in a clean merge state before each merge
			if err := ensureCleanMergeState(cfg.Root); err != nil {
				fmt.Fprintf(os.Stderr, "\033[33m⚠ %s — skipping remaining %d merge(s)\033[0m\n", err, len(waveSuccesses)-i)
				for _, remaining := range waveSuccesses[i:] {
					allFailures = append(allFailures, featureResult{Slug: remaining.Slug, Err: fmt.Errorf("skipped: unclean merge state")})
					failedSlugs[remaining.Slug] = true
				}
				break
			}
			if err := mergeFeatureBranch(cfg, s.Slug, s.Branch, s.WorktreePath, activeWorktrees); err != nil {
				fmt.Fprintf(os.Stderr, "\033[31m✗ merge failed for %s: %s\033[0m\n", s.Slug, err)
				fmt.Fprintf(os.Stderr, "  Worktree preserved at: %s\n", s.WorktreePath)
				fmt.Fprintf(os.Stderr, "  Branch: %s\n", s.Branch)
				allFailures = append(allFailures, featureResult{Slug: s.Slug, Err: err})
				failedSlugs[s.Slug] = true
			} else {
				totalMerged++
			}
		}
	}

	// Sync master PROGRESS.md with actual feature states after all merges
	featuresDir = filepath.Join(cfg.Root, ".belmont", "features")
	syncMasterFeatureStatuses(cfg.Root, listFeatures(featuresDir, 50))

	// Commit any remaining .belmont/ state changes after all merges
	if err := commitBelmontState(cfg.Root); err != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ Failed to commit final .belmont/ state: %s\033[0m\n", err)
	}

	// Report
	if len(allFailures) > 0 {
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ %d feature(s) failed:\033[0m\n", len(allFailures))
		for _, f := range allFailures {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", f.Slug, f.Err)
			if f.WorktreePath != "" {
				fmt.Fprintf(os.Stderr, "    Worktree: %s\n", f.WorktreePath)
			}
		}
	}

	// Clean up auto.json now that all features are processed
	activeWorktrees.removeAutoJSON()

	fmt.Fprintf(os.Stderr, "\n\033[32m✓ %d/%d features complete\033[0m (%.1fs total)\n", totalMerged, len(slugs), time.Since(startTime).Seconds())

	if len(allFailures) > 0 {
		return fmt.Errorf("auto: %d feature(s) failed", len(allFailures))
	}
	return nil
}

// handleStaleWorktree checks for a stale branch/worktree from a previous interrupted run.
// Returns resumed=true if the existing worktree should be reused (skip creation).
// Returns resumed=false if stale state was cleaned up (proceed with fresh creation).
func handleStaleWorktree(root, id, branch, wtPath string) (resumed bool, err error) {
	// Check if branch already exists
	checkCmd := exec.Command("git", "branch", "--list", branch)
	checkCmd.Dir = root
	out, err := checkCmd.Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return false, nil // no stale branch, proceed normally
	}

	// Stale branch exists — determine what to do
	_, wtDirErr := os.Stat(wtPath)
	wtExists := wtDirErr == nil

	if isTerminal(os.Stdin) {
		// Interactive: prompt the user
		status := "branch exists"
		if wtExists {
			status = "branch + worktree exist"
		}
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ Branch '%s' exists from a previous run (%s).\033[0m\n", branch, status)
		fmt.Fprintf(os.Stderr, "  [r] Resume from where it left off\n")
		fmt.Fprintf(os.Stderr, "  [s] Start fresh (delete branch and restart)\n")
		fmt.Fprintf(os.Stderr, "  [q] Quit\n")
		fmt.Fprintf(os.Stderr, "> ")

		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		choice := strings.TrimSpace(strings.ToLower(line))

		switch choice {
		case "r", "resume":
			if wtExists {
				// Worktree still exists — reuse it directly
				fmt.Fprintf(os.Stderr, "  Resuming with existing worktree at %s\n", wtPath)
			} else {
				// Branch exists but worktree is gone — reattach
				fmt.Fprintf(os.Stderr, "  Reattaching worktree to existing branch %s\n", branch)
				wtDir := filepath.Dir(wtPath)
				if err := os.MkdirAll(wtDir, 0755); err != nil {
					return false, fmt.Errorf("create worktree dir: %w", err)
				}
				addCmd := exec.Command("git", "worktree", "add", wtPath, branch)
				addCmd.Dir = root
				if out, err := addCmd.CombinedOutput(); err != nil {
					return false, fmt.Errorf("git worktree add (resume): %w (%s)", err, strings.TrimSpace(string(out)))
				}
			}
			// Leave .belmont/ as-is in the worktree — it has committed state from the
			// previous run. Don't overwrite with stale copy from main repo.
			// If it's an old symlink from a previous version, replace with a fresh copy.
			dstBelmont := filepath.Join(wtPath, ".belmont")
			if fi, err := os.Lstat(dstBelmont); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				// Old-style symlink — replace with copy-based approach
				os.RemoveAll(dstBelmont)
				copyBelmontStateToWorktree(root, wtPath, id)
				commitWorktreeFeatureState(wtPath, id)
			} else if err != nil {
				// .belmont/ missing entirely — copy it in
				copyBelmontStateToWorktree(root, wtPath, id)
				commitWorktreeFeatureState(wtPath, id)
			}
			return true, nil

		case "q", "quit":
			return false, fmt.Errorf("user chose to quit")

		default: // "s", "start", or anything else → start fresh
			fmt.Fprintf(os.Stderr, "  Cleaning up stale state for %s...\n", id)
		}
	} else {
		// Non-interactive: auto-restart
		fmt.Fprintf(os.Stderr, "  Cleaning up stale branch '%s' from previous run...\n", branch)
	}

	// Clean up stale state (restart path)
	if wtExists {
		removeWorktree(root, wtPath, id)
	}
	// Prune any orphaned worktree references
	pruneCmd := exec.Command("git", "worktree", "prune")
	pruneCmd.Dir = root
	pruneCmd.Run()
	// Delete the stale branch
	delCmd := exec.Command("git", "branch", "-D", branch)
	delCmd.Dir = root
	delCmd.Run()

	return false, nil
}

// runFeatureInWorktree creates a worktree for a feature, installs belmont, and runs the full loop.
func runFeatureInWorktree(cfg loopConfig, slug, branch, wtPath string, tracker *worktreeTracker, resumed bool) error {
	if !resumed {
		// Create worktree directory
		wtDir := filepath.Dir(wtPath)
		if err := os.MkdirAll(wtDir, 0755); err != nil {
			return fmt.Errorf("create worktree dir: %w", err)
		}

		// Create git worktree
		cmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath, "HEAD")
		cmd.Dir = cfg.Root
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git worktree add: %w (%s)", err, strings.TrimSpace(string(out)))
		}

		// Copy .belmont state into worktree (isolated copy, not symlink)
		if err := copyBelmontStateToWorktree(cfg.Root, wtPath, slug); err != nil {
			return fmt.Errorf("copy .belmont state to worktree: %w", err)
		}

		// Commit the initial feature state so the AI agent starts from a clean git state
		commitWorktreeFeatureState(wtPath, slug)
	}

	// Copy .env files (gitignored, so not present in fresh worktrees)
	copyEnvFiles(cfg.Root, wtPath)

	// Run belmont install in the worktree
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}
	installCmd := exec.Command(exePath, "install", "--project", wtPath, "--no-prompt")
	installCmd.Dir = wtPath
	if out, err := installCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33mInstall warning for %s: %s\033[0m\n", slug, strings.TrimSpace(string(out)))
	}

	// Allocate a port for this worktree
	port, err := allocatePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Failed to allocate port for %s: %s\033[0m\n", slug, err)
	} else {
		fmt.Fprintf(os.Stderr, "  Port %d assigned to %s\n", port, slug)
	}
	if tracker != nil {
		tracker.setPort(slug, port)
	}

	// Run worktree setup hooks
	hooks := loadWorktreeHooks(cfg.Root)
	if hooks != nil && len(hooks.Setup) > 0 {
		fmt.Fprintf(os.Stderr, "  Running worktree setup hooks for %s...\n", slug)
		if err := runWorktreeHookCommands(hooks.Setup, wtPath, port, hooks.Env); err != nil {
			return fmt.Errorf("worktree setup for %s: %w", slug, err)
		}
	} else if hooks == nil {
		// No worktree.json — auto-detect dependency install from lock files
		if cmds := detectAutoInstallCommands(cfg.Root); len(cmds) > 0 {
			fmt.Fprintf(os.Stderr, "  Auto-installing dependencies for %s (%s)...\n", slug, strings.Join(cmds, ", "))
			if err := runWorktreeHookCommands(cmds, wtPath, port, nil); err != nil {
				fmt.Fprintf(os.Stderr, "  \033[33m⚠ Auto-install failed for %s: %s (continuing)\033[0m\n", slug, err)
			}
		}
	}

	// Run loop for this feature (all milestones)
	mCfg := cfg
	mCfg.Root = wtPath
	mCfg.Feature = slug
	mCfg.Port = port
	if hooks != nil {
		mCfg.WorktreeEnv = hooks.Env
	}

	// Load per-feature model tiers from the worktree's copy of models.yaml.
	if t, err := parseModelTiers(filepath.Join(wtPath, ".belmont", "features", slug, "models.yaml")); err == nil {
		mCfg.ModelTiers = t
	}

	return runLoop(mCfg)
}

// mergeFeatureBranch merges a feature branch back to main and cleans up.
func mergeFeatureBranch(cfg loopConfig, slug, branch, wtPath string, tracker *worktreeTracker) error {
	// Commit any uncommitted CODE changes in the worktree before merging.
	// .belmont/ is assume-unchanged so it won't be included in this commit.
	if err := commitWorktreeChanges(wtPath, slug); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Failed to commit worktree changes for %s: %s\033[0m\n", slug, err)
	}

	commitMsg := fmt.Sprintf("belmont: merge feature %s", slug)

	if err := attemptMerge(cfg, commitMsg, branch, slug); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[31m✗ Merge failed for feature %s\033[0m\n", slug)
		fmt.Fprintf(os.Stderr, "    Worktree preserved at: %s\n", wtPath)
		fmt.Fprintf(os.Stderr, "    Branch: %s\n", branch)
		fmt.Fprintf(os.Stderr, "    Resolve manually: git merge --no-ff %s\n", branch)
		fmt.Fprintf(os.Stderr, "    Or use: belmont recover --merge %s\n", slug)
		return err
	}

	// Copy the feature's updated state from the worktree back to the main repo.
	// .belmont/ was excluded from the merge (assume-unchanged), so we sync it
	// separately. This preserves other features' state on the main branch.
	syncFeatureStateAfterMerge(cfg.Root, wtPath, slug)

	// Clean up reconciliation report if it exists
	os.Remove(filepath.Join(cfg.Root, ".belmont", "reconciliation-report.json"))

	// Run teardown hooks and clean up worktree
	tracker.teardownEntry(slug)
	removeWorktree(cfg.Root, wtPath, slug)
	tracker.remove(slug)

	// Delete the branch
	delCmd := exec.Command("git", "branch", "-d", branch)
	delCmd.Dir = cfg.Root
	delCmd.Run() // best-effort

	fmt.Fprintf(os.Stderr, "  \033[32m✓ Feature %s merged successfully\033[0m\n", slug)
	return nil
}

func detectTool() string {
	for _, tool := range []string{"claude", "codex", "gemini", "copilot", "cursor"} {
		if _, err := exec.LookPath(tool); err == nil {
			return tool
		}
	}
	return ""
}

func runLoop(cfg loopConfig) error {
	startTime := time.Now()
	var history []historyEntry
	var lastOutput string

	// Write auto.json for status visibility when running standalone (not from parallel mode).
	// In parallel mode, the worktreeTracker manages auto.json separately.
	var autoCleanup func()
	if cfg.Port == 0 {
		autoPath := filepath.Join(cfg.Root, ".belmont", "auto.json")
		writeLoopAutoJSON(autoPath, cfg)
		autoCleanup = func() { os.Remove(autoPath) }
	}
	if autoCleanup != nil {
		defer autoCleanup()
	}

	fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto — %s\033[0m\n", cfg.Feature)
	fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Policy: %s | Max iterations: %d\033[0m\n", cfg.Tool, cfg.Policy, cfg.MaxIterations)
	if cfg.From != "" || cfg.To != "" {
		fromStr := cfg.From
		if fromStr == "" {
			fromStr = "start"
		}
		toStr := cfg.To
		if toStr == "" {
			toStr = "end"
		}
		fmt.Fprintf(os.Stderr, "\033[2mRange: %s → %s\033[0m\n", fromStr, toStr)
	}
	fmt.Fprintln(os.Stderr)

	for i := 1; i <= cfg.MaxIterations; i++ {
		// 1. Read current state
		report, err := buildStatus(cfg.Root, 55, cfg.Feature)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[31mFailed to read state: %s\033[0m\n", err)
			return fmt.Errorf("auto: state read failed: %w", err)
		}

		// 2. Derive extra signals
		hasFwlup := detectFwlupTasks(cfg.Root, cfg.Feature, report)
		currentMsID := lastMilestoneID(history)
		hasMsFwlup := currentMsID != "" && detectFwlupTasksForMilestone(cfg.Root, cfg.Feature, report, currentMsID)
		msStates := buildMilestoneLoopStates(history, report.Milestones)

		// Range-scoped signals: only consider tasks/FWLUPs under milestones within --from/--to
		pendingInRange := pendingTasksInRange(cfg.Root, cfg.Feature, cfg.From, cfg.To)
		fwlupInRange := fwlupTasksInRange(cfg.Root, cfg.Feature, report, cfg.From, cfg.To)

		// Print state summary
		printLoopState(report, hasFwlup)

		// 3. Check hard guardrails first
		action := checkHardGuardrails(report, history, cfg)

		// 4. If no guardrail triggered, try smart rules first
		if action == nil {
			action = decideLoopActionSmart(report, history, cfg, hasFwlup, hasMsFwlup, pendingInRange, fwlupInRange, msStates)
		}

		// 4b. If smart rules returned nil, check stuck detection before AI
		if action == nil && isLoopStuck(history) {
			action = &loopAction{Type: actionPause, Reason: fmt.Sprintf("Loop appears stuck — no state change after 2 iterations (last action: %s)", history[len(history)-1].Action.Type)}
		}

		// 5. If smart rules returned nil, use AI decisions (with rules fallback)
		if action == nil {
			aiAction, err := decideLoopActionAI(report, history, cfg, hasFwlup, lastOutput, msStates)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\033[33m  AI decision failed: %s — falling back to rules\033[0m\n", err)
				decided := decideLoopAction(report, history, cfg, fwlupInRange, pendingInRange)
				action = &decided
			} else {
				action = aiAction
			}
		}

		label := describeMilestone(action, report)
		actionLabel := shortActionLabel(action.Type)
		if label != "" {
			fmt.Fprintf(os.Stderr, "\n\033[1m━━ [%d] %s ━━ %s › %s ━━\033[0m\n", i, actionLabel, cfg.Feature, label)
		} else {
			fmt.Fprintf(os.Stderr, "\n\033[1m━━ [%d] %s ━━ %s ━━\033[0m\n", i, actionLabel, cfg.Feature)
		}
		fmt.Fprintf(os.Stderr, "\033[2m  %s\033[0m\n\n", action.Reason)

		// 5. Terminal actions
		if action.Type == actionComplete {
			fmt.Fprintf(os.Stderr, "\n\033[32m✓ Complete\033[0m — %s (%.1fs total)\n", action.Reason, time.Since(startTime).Seconds())
			return nil
		}
		if action.Type == actionError {
			fmt.Fprintf(os.Stderr, "\n\033[31m✗ Error\033[0m — %s\n", action.Reason)
			return fmt.Errorf("auto: %s", action.Reason)
		}
		if action.Type == actionPause {
			fmt.Fprintf(os.Stderr, "\n\033[33m⏸ Paused\033[0m — %s\n", action.Reason)
			fmt.Fprintf(os.Stderr, "Resume with: belmont auto --feature %s", cfg.Feature)
			if cfg.From != "" {
				fmt.Fprintf(os.Stderr, " --from %s", cfg.From)
			}
			if cfg.To != "" {
				fmt.Fprintf(os.Stderr, " --to %s", cfg.To)
			}
			fmt.Fprintln(os.Stderr)
			return errFeaturePaused
		}

		// 6. Handle SKIP_MILESTONE (state mutation, no tool call)
		if action.Type == actionSkipMilestone {
			skipErr := skipMilestoneInProgress(cfg.Root, cfg.Feature, action.MilestoneID)
			if skipErr != nil {
				fmt.Fprintf(os.Stderr, "\033[31m  ✗ Failed to skip milestone: %s\033[0m\n\n", skipErr)
			} else {
				fmt.Fprintf(os.Stderr, "\033[32m  ✓ Skipped milestone %s\033[0m\n\n", action.MilestoneID)
			}
			entry := historyEntry{
				Action:       *action,
				Result:       &executionResult{Success: skipErr == nil, DurationMs: 0},
				TasksDone:    report.TaskCounts["done"] + report.TaskCounts["verified"],
				TasksTotal:   report.TaskCounts["total"],
				MsDone:       countDoneMilestones(report.Milestones),
				MsTotal:      len(report.Milestones),
				BlockerCount: blockedTaskCount(report.Milestones),
				HasFwlup:     hasFwlup,
				Iteration:    i,
			}
			history = append(history, entry)
			continue
		}

		// 7. Checkpoint policy check
		if shouldLoopCheckpoint(*action, cfg.Policy, lastActionType(history)) {
			fmt.Fprintf(os.Stderr, "\n\033[33m⏸ Checkpoint\033[0m — %s\n", action.Reason)
			fmt.Fprintf(os.Stderr, "Resume with: belmont auto --feature %s", cfg.Feature)
			if cfg.From != "" {
				fmt.Fprintf(os.Stderr, " --from %s", cfg.From)
			}
			if cfg.To != "" {
				fmt.Fprintf(os.Stderr, " --to %s", cfg.To)
			}
			fmt.Fprintln(os.Stderr)
			return nil
		}

		// 8. Capture pre-action SHA
		preSHA := captureGitSHA(cfg.Root)

		// 9. Execute action
		result := executeLoopAction(*action, cfg)
		lastOutput = truncateTail(result.Output, 1500)

		// 9b. Parse triage decision from output
		if action.Type == actionTriage && result.Success {
			if td := parseTriageDecision(result.Output); td != nil {
				action.TriageDecision = td.Decision
				action.ReverifyScope = td.ReverifyScope
				fmt.Fprintf(os.Stderr, "\033[2m  Triage: %s (%s)\033[0m\n", td.Decision, td.Reason)
			} else {
				fmt.Fprintf(os.Stderr, "\033[33m  Warning: could not parse triage decision from output — deferring\033[0m\n")
				// Default to defer_and_proceed if parsing fails — avoids expensive fix-all + re-verify loops
				action.TriageDecision = "defer_and_proceed"
				action.ReverifyScope = ""
			}
		}

		// 9c. Propagate triage decision to FIX_ALL action for downstream rules
		if action.Type == actionFixAll && action.TriageDecision == "" {
			// Look back in history for the most recent triage decision
			for i := len(history) - 1; i >= 0; i-- {
				if history[i].Action.Type == actionTriage {
					action.TriageDecision = history[i].Action.TriageDecision
					action.ReverifyScope = history[i].Action.ReverifyScope
					break
				}
			}
		}

		// 10. Post-action classification
		postSHA := captureGitSHA(cfg.Root)
		wt, fc := classifyChanges(cfg.Root, preSHA)

		// 11. Record in history
		entry := historyEntry{
			Action:       *action,
			Result:       &result,
			TasksDone:    report.TaskCounts["done"] + report.TaskCounts["verified"],
			TasksTotal:   report.TaskCounts["total"],
			MsDone:       countDoneMilestones(report.Milestones),
			MsTotal:      len(report.Milestones),
			BlockerCount: blockedTaskCount(report.Milestones),
			HasFwlup:     hasFwlup,
			Iteration:    i,
			WorkType:     wt,
			FilesChanged: fc,
			GitSHA:       preSHA,
			PostGitSHA:   postSHA,
		}
		history = append(history, entry)

		// 12. Print result
		if result.Success {
			fmt.Fprintf(os.Stderr, "\n\033[32m  ✓ %.1fs\033[0m\n", float64(result.DurationMs)/1000)
		} else {
			fmt.Fprintf(os.Stderr, "\n\033[31m  ✗ %s (%.1fs)\033[0m\n", result.Error, float64(result.DurationMs)/1000)
		}
	}

	fmt.Fprintf(os.Stderr, "\n\033[33m⏸ Max iterations reached (%d)\033[0m\n", cfg.MaxIterations)
	return nil
}

func printLoopState(report statusReport, hasFwlup bool) {
	done := report.TaskCounts["done"] + report.TaskCounts["verified"]
	total := report.TaskCounts["total"]
	msDone := countDoneMilestones(report.Milestones)
	msTotal := len(report.Milestones)

	// Progress bar
	barWidth := 20
	filled := 0
	if total > 0 {
		filled = (done * barWidth) / total
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	fmt.Fprintf(os.Stderr, "  [%s] %d/%d tasks, %d/%d milestones", bar, done, total, msDone, msTotal)

	if hasFwlup {
		fmt.Fprintf(os.Stderr, " \033[33m(FWLUP)\033[0m")
	}
	blockedCount := blockedTaskCount(report.Milestones)
	if blockedCount > 0 {
		fmt.Fprintf(os.Stderr, " \033[31m(%d blocked)\033[0m", blockedCount)
	}
	fmt.Fprintln(os.Stderr)
}

func decideLoopAction(report statusReport, history []historyEntry, cfg loopConfig, hasFwlup bool, pendingTasks bool) loopAction {
	last := lastActionType(history)

	// Rule 1: Blocked tasks → PAUSE
	blocked := blockedTaskNames(report.Milestones)
	if len(blocked) > 0 {
		return loopAction{Type: actionPause, Reason: fmt.Sprintf("Blocked tasks: %s", strings.Join(blocked, ", "))}
	}

	// Rule 2: Consecutive failures >= maxFailures → ERROR
	if consecutiveFailures(history) >= cfg.MaxFailures {
		return loopAction{Type: actionError, Reason: fmt.Sprintf("%d consecutive failures", cfg.MaxFailures)}
	}

	// Rule 3: Stuck detection
	if isLoopStuck(history) {
		return loopAction{Type: actionPause, Reason: "Loop appears stuck — no state change after 2 iterations"}
	}

	// Rule 4: FWLUP tasks after VERIFY → TRIAGE
	if hasFwlup && (last == actionVerify || last == actionFixAll) {
		return loopAction{Type: actionTriage, Reason: "Triaging follow-up tasks"}
	}

	// Rule 5: After IMPLEMENT_NEXT or FIX_ALL → VERIFY
	if last == actionImplementNext || last == actionFixAll {
		return loopAction{Type: actionVerify, Reason: "Re-verifying after follow-up fix"}
	}

	// Rule 6: After IMPLEMENT_MILESTONE → VERIFY
	if last == actionImplementMilestone {
		return loopAction{Type: actionVerify, Reason: "Verifying completed milestone"}
	}

	// Rule 7-10: Check milestones in range
	inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
	allDone := true
	for _, m := range inRange {
		if !milestoneAllDone(m) {
			allDone = false
			break
		}
	}

	// Rule 7: All done + no FWLUP → COMPLETE (but check pending tasks first)
	if allDone && !hasFwlup && !pendingTasks {
		return loopAction{Type: actionComplete, Reason: "All milestones in range completed"}
	}
	if allDone && !hasFwlup && pendingTasks {
		return loopAction{Type: actionImplementNext, Reason: "All milestones marked done but tasks still pending"}
	}

	// Rule 8: All done but FWLUP remaining → TRIAGE
	if allDone && hasFwlup {
		return loopAction{Type: actionTriage, Reason: "Triaging remaining follow-up tasks"}
	}

	// Rule 10: Next milestone in range → IMPLEMENT_MILESTONE
	for _, m := range inRange {
		if !milestoneAllDone(m) {
			return loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Implementing milestone %s", m.ID), MilestoneID: m.ID}
		}
	}

	// Fallback
	if pendingTasks {
		return loopAction{Type: actionImplementNext, Reason: "Tasks still pending — implementing next"}
	}
	return loopAction{Type: actionComplete, Reason: "No actionable milestones found"}
}

func milestonesInRange(milestones []milestone, from, to string) []milestone {
	if from == "" && to == "" {
		return milestones
	}

	fromNum := parseMilestoneNum(from)
	toNum := parseMilestoneNum(to)

	var result []milestone
	for _, m := range milestones {
		num := parseMilestoneNum(m.ID)
		if num < 0 {
			continue
		}
		if fromNum >= 0 && num < fromNum {
			continue
		}
		if toNum >= 0 && num > toNum {
			continue
		}
		result = append(result, m)
	}
	return result
}

func parseMilestoneNum(id string) int {
	if id == "" {
		return -1
	}
	re := regexp.MustCompile(`(?i)M(\d+)`)
	match := re.FindStringSubmatch(id)
	if len(match) < 2 {
		return -1
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return -1
	}
	return n
}

// tailWriter writes all data to an underlying writer and keeps a rolling
// buffer of the last `size` bytes for later retrieval.
type tailWriter struct {
	out     io.Writer
	buf     []byte
	size    int
	prefix  string
	lineBuf []byte // partial line accumulator (used when prefix is set)
}

func newTailWriter(out io.Writer, size int, prefix string) *tailWriter {
	return &tailWriter{out: out, buf: make([]byte, 0, size), size: size, prefix: prefix}
}

func (tw *tailWriter) Write(p []byte) (int, error) {
	// Always store raw bytes in buf for error tail reporting
	tw.buf = append(tw.buf, p...)
	if len(tw.buf) > tw.size {
		tw.buf = tw.buf[len(tw.buf)-tw.size:]
	}

	// When no prefix, pass through directly
	if tw.prefix == "" {
		_, err := tw.out.Write(p)
		return len(p), err
	}

	// Line-buffer and prepend prefix to each complete line
	tw.lineBuf = append(tw.lineBuf, p...)
	for {
		idx := bytes.IndexByte(tw.lineBuf, '\n')
		if idx < 0 {
			break
		}
		line := tw.lineBuf[:idx]
		tw.lineBuf = tw.lineBuf[idx+1:]
		tw.out.Write([]byte(tw.prefix + string(line) + "\n"))
	}
	return len(p), nil
}

func (tw *tailWriter) String() string {
	return string(tw.buf)
}

// claudeStreamWriter wraps a tailWriter and parses Claude stream-json NDJSON,
// extracting only human-readable content (assistant text + tool use indicators).
type claudeStreamWriter struct {
	tw      *tailWriter
	partial []byte
	prefix  string // e.g. "\033[36m[slug]\033[0m: "
}

type streamLine struct {
	Type    string        `json:"type"`
	Message streamMessage `json:"message"`
}

type streamMessage struct {
	Content []streamContent `json:"content"`
}

type streamContent struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

func (c *claudeStreamWriter) Write(p []byte) (int, error) {
	c.partial = append(c.partial, p...)
	for {
		idx := bytes.IndexByte(c.partial, '\n')
		if idx < 0 {
			break
		}
		line := c.partial[:idx]
		c.partial = c.partial[idx+1:]
		if len(line) == 0 {
			continue
		}
		var sl streamLine
		if err := json.Unmarshal(line, &sl); err != nil {
			continue
		}
		if sl.Type != "assistant" {
			continue
		}
		for _, item := range sl.Message.Content {
			switch item.Type {
			case "text":
				if item.Text != "" {
					if c.prefix != "" {
						c.tw.Write([]byte(c.prefix + item.Text + "\n"))
					} else {
						c.tw.Write([]byte("  " + item.Text + "\n"))
					}
				}
			case "tool_use":
				if item.Name != "" {
					if c.prefix != "" {
						c.tw.Write([]byte(c.prefix + toolSummary(item.Name, item.Input) + "\n"))
					} else {
						c.tw.Write([]byte("  → " + toolSummary(item.Name, item.Input) + "\n"))
					}
				}
			}
		}
	}
	return len(p), nil
}

func toolSummary(name string, input map[string]interface{}) string {
	switch name {
	case "Read", "Write", "Edit":
		if fp, ok := input["file_path"].(string); ok {
			return name + " " + filepath.Base(fp)
		}
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 120 {
				cmd = cmd[:120] + "…"
			}
			return name + " " + cmd
		}
	case "Grep":
		if pat, ok := input["pattern"].(string); ok {
			return name + " " + strconv.Quote(pat)
		}
	case "Glob":
		if pat, ok := input["pattern"].(string); ok {
			return name + " " + pat
		}
	case "Agent":
		if desc, ok := input["description"].(string); ok {
			if len(desc) > 120 {
				desc = desc[:120] + "…"
			}
			return name + " " + desc
		}
	case "Skill":
		if sk, ok := input["skill"].(string); ok {
			return name + " " + sk
		}
	}
	return name
}

func shortActionLabel(t loopActionType) string {
	switch t {
	case actionImplementMilestone:
		return "IMPLEMENT"
	case actionImplementNext:
		return "FIX"
	case actionVerify:
		return "VERIFY"
	case actionReplan:
		return "REPLAN"
	case actionSkipMilestone:
		return "SKIP"
	case actionDebug:
		return "DEBUG"
	case actionTriage:
		return "TRIAGE"
	case actionFixAll:
		return "FIX-ALL"
	default:
		return string(t)
	}
}

func describeMilestone(action *loopAction, report statusReport) string {
	if action.MilestoneID != "" {
		for _, m := range report.Milestones {
			if m.ID == action.MilestoneID {
				return m.ID + ": " + m.Name
			}
		}
	}
	if action.Type == actionVerify || action.Type == actionImplementNext {
		if report.NextMilestone != nil {
			return report.NextMilestone.ID + ": " + report.NextMilestone.Name
		}
	}
	return ""
}

func executeLoopAction(action loopAction, cfg loopConfig) executionResult {
	prompt := buildLoopPrompt(action, cfg.Feature)

	// Consume any pending user steering for this milestone. Runs before any
	// shell-out so a single injection maps to one agent run — the auto loop
	// re-enters the same milestone across phases and we don't want the same
	// instruction to fire repeatedly.
	steeringBlock, steeringCount := consumePendingSteering(cfg.Root, cfg.Feature, action.MilestoneID, string(action.Type))
	if steeringCount > 0 {
		logSteeringInjection(cfg.Feature, action.MilestoneID, steeringCount, steeringBlock)
	}

	// Triage uses its own prompt template
	if action.Type == actionTriage {
		return executeTriageAction(cfg, steeringBlock)
	}

	if steeringCount > 0 {
		prompt = steeringBlock + prompt
	}

	modelFlags := resolveModelFlags(cfg.Tool, tierForAction(action.Type, cfg.ModelTiers))

	var cmd *exec.Cmd
	switch cfg.Tool {
	case "claude":
		args := []string{"-p", prompt,
			"--permission-mode", "bypassPermissions",
			"--allowedTools", "Bash Read Write Edit Glob Grep Agent Skill WebFetch WebSearch mcp__*",
			"--output-format", "stream-json", "--verbose"}
		args = append(args, modelFlags...)
		cmd = exec.Command("claude", args...)
	case "codex":
		args := []string{"exec", prompt,
			"--dangerously-bypass-approvals-and-sandbox",
			"--json", "-C", cfg.Root}
		args = append(args, modelFlags...)
		cmd = exec.Command("codex", args...)
	case "gemini":
		args := []string{prompt, "--yolo", "--output-format", "json"}
		args = append(args, modelFlags...)
		cmd = exec.Command("gemini", args...)
	case "copilot":
		args := []string{"-p", prompt, "--yolo"}
		args = append(args, modelFlags...)
		cmd = exec.Command("copilot", args...)
	case "cursor":
		args := []string{"agent", "-p", prompt, "--force", "--output-format", "json"}
		args = append(args, modelFlags...)
		cmd = exec.Command("cursor", args...)
	default:
		return executionResult{Success: false, Error: fmt.Sprintf("unsupported tool: %s", cfg.Tool)}
	}

	cmd.Dir = cfg.Root

	// Worktree isolation: inject env vars and set process group
	if cfg.Port != 0 {
		cmd.Env = buildWorktreeEnv(cfg.Port, cfg.WorktreeEnv)
		setSysProcAttr(cmd)
	}

	var prefix string
	if cfg.Feature != "" {
		if action.MilestoneID != "" {
			prefix = fmt.Sprintf("\033[36m[%s][%s]\033[0m: ", cfg.Feature, action.MilestoneID)
		} else {
			prefix = fmt.Sprintf("\033[36m[%s]\033[0m: ", cfg.Feature)
		}
	}

	var tw *tailWriter
	if cfg.Tool == "claude" {
		tw = newTailWriter(os.Stderr, 1500, "")
		cmd.Stdout = &claudeStreamWriter{tw: tw, prefix: prefix}
		cmd.Stderr = tw
	} else {
		tw = newTailWriter(os.Stderr, 1500, prefix)
		cmd.Stdout = tw
		cmd.Stderr = tw
	}

	var stopTimer chan struct{}
	if cfg.Tool != "claude" {
		stopTimer = make(chan struct{})
		go func() {
			start := time.Now()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					fmt.Fprintf(os.Stderr, "\r\033[2m  ⏱ %s\033[0m", time.Since(start).Truncate(time.Second))
				case <-stopTimer:
					fmt.Fprintf(os.Stderr, "\r\033[K")
					return
				}
			}
		}()
	}

	start := time.Now()
	if startErr := cmd.Start(); startErr != nil {
		if stopTimer != nil {
			close(stopTimer)
		}
		return executionResult{
			Success: false,
			Error:   fmt.Sprintf("failed to start: %s", startErr),
		}
	}

	// Track the PGID so Ctrl-C cleanup works via worktreeTracker
	pid := cmd.Process.Pid
	if cfg.Tracker != nil && cfg.TrackerID != "" {
		cfg.Tracker.setPgid(cfg.TrackerID, pid)
	}

	err := cmd.Wait()
	if stopTimer != nil {
		close(stopTimer)
	}

	// Kill the entire process group to clean up orphaned child processes
	// (dev servers, test runners, etc.) that survive the AI tool exiting.
	if cfg.Port != 0 {
		killProcessGroup(pid)
	}

	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		return executionResult{
			Success:    false,
			Output:     tw.String(),
			Error:      err.Error(),
			DurationMs: durationMs,
		}
	}

	return executionResult{
		Success:    true,
		Output:     tw.String(),
		DurationMs: durationMs,
	}
}

// executeTriageAction runs the triage prompt template as a full tool-equipped invocation.
// steeringPrefix, if non-empty, is prepended to the prompt before the template body.
func executeTriageAction(cfg loopConfig, steeringPrefix string) executionResult {
	featureBase := filepath.Join(".belmont", "features", cfg.Feature)

	// Determine fix round from milestone states
	fixRound := 0
	// We'll pass 0 and let the template handle it; the prompt template itself
	// contains circuit breaker logic based on this value

	// Load the triage prompt template
	tmpl, tmplErr := loadPromptTemplate("post-verify-triage")
	var prompt string
	if tmplErr != nil {
		// Fallback inline prompt
		prompt = fmt.Sprintf(`You are a triage agent. Read %s/PROGRESS.md to find incomplete tasks (marked [ ] or [>]).
Classify each as blocking (real bug) or deferrable (polish).
If all are deferrable, move them to %s/NOTES.md under ## Polish and remove from PROGRESS.md.
Output JSON: {"decision":"defer_and_proceed|fix_and_proceed|fix_and_reverify","blocking_tasks":[],"deferred_tasks":[],"reason":"...","reverify_scope":"focused"}`, featureBase, featureBase)
	} else {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, map[string]string{
			"Feature":      cfg.Feature,
			"FeatureBase":  featureBase,
			"FixRound":     fmt.Sprintf("%d", fixRound),
			"VerifyOutput": "", // verify output is available from lastOutput in the loop
		}); err != nil {
			return executionResult{Success: false, Error: fmt.Sprintf("execute triage template: %s", err)}
		}
		prompt = buf.String()
	}

	if steeringPrefix != "" {
		prompt = steeringPrefix + prompt
	}

	triageFlags := resolveModelFlags(cfg.Tool, tierForAction(actionTriage, cfg.ModelTiers))
	cmd := buildToolCommand(cfg.Tool, prompt, cfg.Root, triageFlags...)

	// Worktree isolation: inject env vars and set process group
	if cfg.Port != 0 {
		cmd.Env = buildWorktreeEnv(cfg.Port, cfg.WorktreeEnv)
		setSysProcAttr(cmd)
	}

	var triagePrefix string
	if cfg.Port != 0 && cfg.Feature != "" {
		triagePrefix = fmt.Sprintf("\033[36m[%s]\033[0m: ", cfg.Feature)
	}

	var tw *tailWriter
	if cfg.Tool == "claude" {
		tw = newTailWriter(os.Stderr, 1500, "")
		cmd.Stdout = &claudeStreamWriter{tw: tw, prefix: triagePrefix}
		cmd.Stderr = tw
	} else {
		tw = newTailWriter(os.Stderr, 1500, triagePrefix)
		cmd.Stdout = tw
		cmd.Stderr = tw
	}

	var stopTimer chan struct{}
	if cfg.Tool != "claude" {
		stopTimer = make(chan struct{})
		go func() {
			start := time.Now()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					fmt.Fprintf(os.Stderr, "\r\033[2m  ⏱ %s\033[0m", time.Since(start).Truncate(time.Second))
				case <-stopTimer:
					fmt.Fprintf(os.Stderr, "\r\033[K")
					return
				}
			}
		}()
	}

	start := time.Now()
	if startErr := cmd.Start(); startErr != nil {
		if stopTimer != nil {
			close(stopTimer)
		}
		return executionResult{
			Success: false,
			Error:   fmt.Sprintf("failed to start: %s", startErr),
		}
	}

	pid := cmd.Process.Pid
	if cfg.Tracker != nil && cfg.TrackerID != "" {
		cfg.Tracker.setPgid(cfg.TrackerID, pid)
	}

	err := cmd.Wait()
	if stopTimer != nil {
		close(stopTimer)
	}

	// Kill orphaned child processes (dev servers, etc.)
	if cfg.Port != 0 {
		killProcessGroup(pid)
	}

	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		return executionResult{
			Success:    false,
			Output:     tw.String(),
			Error:      err.Error(),
			DurationMs: durationMs,
		}
	}

	return executionResult{
		Success:    true,
		Output:     tw.String(),
		DurationMs: durationMs,
	}
}

func buildLoopPrompt(action loopAction, feature string) string {
	switch action.Type {
	case actionImplementMilestone:
		prompt := fmt.Sprintf("/belmont:implement --feature %s", feature)
		if action.MilestoneID != "" {
			prompt += fmt.Sprintf("\n\nMILESTONE-SCOPED IMPLEMENTATION: Only implement tasks in milestone %s. Do NOT touch tasks in other milestones — they are either already complete, in progress elsewhere, or intentionally queued for later. Do NOT flip task checkboxes, add/remove tasks, or edit notes for any milestone other than %s.\n\nCRITICAL: In PROGRESS.md only the heading and tasks for %s may change. Other milestones may be intentionally incomplete (parallel worktree, queued re-verify, blocked). Treat their state as read-only context.", action.MilestoneID, action.MilestoneID, action.MilestoneID)
		}
		return prompt
	case actionImplementNext:
		prompt := fmt.Sprintf("/belmont:next --feature %s", feature)
		if action.MilestoneID != "" {
			prompt += fmt.Sprintf("\n\nSCOPE: Only work on tasks within milestone %s. Do NOT implement tasks from other milestones.", action.MilestoneID)
		}
		return prompt
	case actionVerify:
		prompt := fmt.Sprintf("/belmont:verify --feature %s", feature)
		if action.MilestoneID != "" {
			prompt += fmt.Sprintf("\n\nMILESTONE-SCOPED VERIFICATION: Only verify tasks in milestone %s. Do NOT verify tasks from other milestones — those were verified previously. Focus on: (1) the tasks in %s meet their acceptance criteria, (2) build passes, (3) tests pass.\n\nCRITICAL: Do NOT modify the status of ANY other milestone in PROGRESS.md. Only update the heading for %s. Other milestones may be intentionally incomplete (queued for re-verification) — do NOT change their task states.", action.MilestoneID, action.MilestoneID, action.MilestoneID)
		}
		if action.ReverifyScope == "focused" {
			prompt += "\n\nFOCUSED RE-VERIFICATION: This is a re-verify after follow-up fixes. Only verify: (1) the specific FWLUP tasks that were just fixed, (2) build/test pass, (3) any previously-failing acceptance criteria. Do NOT re-run Lighthouse. Do NOT re-check visual specs unless a FWLUP specifically addressed UI. Do NOT create new Polish-level issues."
		}
		return prompt
	case actionReplan:
		return fmt.Sprintf("/belmont:tech-plan --feature %s", feature)
	case actionDebug:
		return fmt.Sprintf("/belmont:debug-auto --feature %s", feature)
	case actionFixAll:
		milestoneClause := "the current milestone"
		if action.MilestoneID != "" {
			milestoneClause = action.MilestoneID
		}
		return fmt.Sprintf("/belmont:next --feature %s\n\nBATCH MODE: Implement ALL pending FWLUP tasks in %s sequentially. For each task: find it, create MILESTONE file, dispatch to implementation agent, process results, archive MILESTONE, then loop to the next pending FWLUP. Stop when no FWLUP tasks remain in %s.\n\nIMPORTANT: Only work on FWLUP tasks (tasks with \"FWLUP\" in their ID) that belong to %s. If there are NO pending FWLUP tasks in %s, stop immediately and report \"No FWLUP tasks to fix.\" Do NOT implement regular tasks — those require the full implementation pipeline.", feature, milestoneClause, milestoneClause, milestoneClause, milestoneClause)
	case actionTriage:
		// Triage uses its own prompt template — handled in executeLoopAction
		return ""
	default:
		return ""
	}
}

func shouldLoopCheckpoint(action loopAction, policy checkpointPolicy, last loopActionType) bool {
	// Terminal actions handled elsewhere
	if action.Type == actionPause || action.Type == actionError || action.Type == actionComplete {
		return false
	}

	switch policy {
	case policyAutonomous:
		return false
	case policyMilestone:
		if action.Type == actionImplementMilestone {
			return true
		}
		// Significant changes warrant a checkpoint
		if action.Type == actionReplan || action.Type == actionSkipMilestone {
			return true
		}
		// Auto-verify after implement
		if action.Type == actionVerify && last == actionImplementMilestone {
			return false
		}
		// Pause after verify results
		if last == actionVerify {
			return true
		}
		return false
	case policyEveryAction:
		return true
	}
	return false
}

func lastActionType(history []historyEntry) loopActionType {
	if len(history) == 0 {
		return ""
	}
	return history[len(history)-1].Action.Type
}

func consecutiveFailures(history []historyEntry) int {
	count := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Result != nil && !history[i].Result.Success {
			count++
		} else {
			break
		}
	}
	return count
}

func isLoopStuck(history []historyEntry) bool {
	if len(history) < 2 {
		return false
	}
	recent := history[len(history)-2:]
	// Both must have succeeded
	for _, e := range recent {
		if e.Result == nil || !e.Result.Success {
			return false
		}
	}
	// Compare state fingerprints
	fp0 := loopFingerprint(recent[0])
	fp1 := loopFingerprint(recent[1])
	return fp0 == fp1
}

func loopFingerprint(e historyEntry) string {
	return fmt.Sprintf("%d/%d|%d/%d|%d|%v|%s", e.TasksDone, e.TasksTotal, e.MsDone, e.MsTotal, e.BlockerCount, e.HasFwlup, e.PostGitSHA)
}

func countDoneMilestones(milestones []milestone) int {
	count := 0
	for _, m := range milestones {
		if milestoneAllDone(m) {
			count++
		}
	}
	return count
}

// captureGitSHA returns the current HEAD SHA, or "" on error.
func captureGitSHA(root string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// classifyChanges runs git diff between preSHA and HEAD and classifies the work type.
func classifyChanges(root, preSHA string) (workType, int) {
	if preSHA == "" {
		return workUnknown, 0
	}
	cmd := exec.Command("git", "diff", "--name-only", preSHA+"..HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return workUnknown, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			files = append(files, l)
		}
	}
	if len(files) == 0 {
		return workMinimal, 0
	}
	if len(files) < 3 {
		return workMinimal, len(files)
	}

	frontendExts := map[string]bool{
		".tsx": true, ".jsx": true, ".css": true, ".scss": true,
		".html": true, ".vue": true, ".svelte": true, ".less": true,
	}
	backendExts := map[string]bool{
		".go": true, ".py": true, ".rs": true, ".java": true,
		".rb": true, ".php": true, ".cs": true, ".kt": true,
		".scala": true, ".ex": true, ".exs": true,
	}
	configExts := map[string]bool{
		".yml": true, ".yaml": true, ".json": true, ".toml": true,
		".ini": true, ".env": true,
	}
	docExts := map[string]bool{
		".md": true, ".txt": true, ".rst": true,
	}

	var feCount, beCount, cfgCount, docCount int
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		switch {
		case frontendExts[ext]:
			feCount++
		case backendExts[ext]:
			beCount++
		case configExts[ext]:
			cfgCount++
		case docExts[ext]:
			docCount++
		}
	}

	total := len(files)
	if docCount == total {
		return workDocs, total
	}
	if cfgCount == total {
		return workConfig, total
	}
	if feCount*2 > total {
		return workFrontend, total
	}
	if beCount*2 > total {
		return workBackend, total
	}
	if feCount > 0 && beCount > 0 {
		return workMixed, total
	}
	if feCount > 0 {
		return workFrontend, total
	}
	if beCount > 0 {
		return workBackend, total
	}
	return workMixed, total
}

// isCriticalConfig returns true if changed files include runtime-affecting config.
func isCriticalConfig(files []string) bool {
	for _, f := range files {
		base := strings.ToLower(filepath.Base(f))
		ext := strings.ToLower(filepath.Ext(f))
		if ext == ".css" || ext == ".scss" || ext == ".less" {
			return true
		}
		if strings.Contains(base, ".env") || strings.Contains(base, "styles") {
			return true
		}
		if base == "tailwind.config.js" || base == "tailwind.config.ts" ||
			base == "postcss.config.js" || base == "vite.config.ts" ||
			base == "next.config.js" || base == "next.config.mjs" {
			return true
		}
	}
	return false
}

// buildMilestoneLoopStates derives per-milestone state from the loop history.
func buildMilestoneLoopStates(history []historyEntry, milestones []milestone) map[string]*milestoneLoopState {
	states := make(map[string]*milestoneLoopState)
	for _, m := range milestones {
		states[m.ID] = &milestoneLoopState{
			ID:   m.ID,
			Name: m.Name,
			Done: milestoneAllDone(m),
		}
	}

	var lastImplementedMS string
	for _, h := range history {
		switch h.Action.Type {
		case actionImplementMilestone:
			msID := h.Action.MilestoneID
			if msID != "" {
				if s, ok := states[msID]; ok && h.Result != nil && h.Result.Success {
					s.Implemented = true
					s.WorkType = h.WorkType
					s.FilesChanged = h.FilesChanged
					lastImplementedMS = msID
				}
			}
		case actionVerify:
			// Attribute verification to the most recently implemented milestone
			if lastImplementedMS != "" {
				if s, ok := states[lastImplementedMS]; ok {
					if h.Result != nil {
						if h.Result.Success {
							s.Verified = true
							s.VerifySucceeded++
						} else {
							s.VerifyFailed++
						}
					}
				}
			}
		case actionTriage:
			// Track triage rounds per milestone
			if lastImplementedMS != "" {
				if s, ok := states[lastImplementedMS]; ok {
					s.FwlupFixRounds++
				}
			}
		}
	}
	return states
}

// lastMilestoneID walks backward through history and returns the most recent non-empty MilestoneID.
func lastMilestoneID(history []historyEntry) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Action.MilestoneID != "" {
			return history[i].Action.MilestoneID
		}
	}
	return ""
}

// decideLoopActionSmart applies deterministic rules for ~80% of cases.
// Returns nil for ambiguous cases that should fall through to AI.
// pendingInRange and fwlupInRange are scoped to the --from/--to milestone range.
func decideLoopActionSmart(report statusReport, history []historyEntry, cfg loopConfig, hasFwlup bool, hasMsFwlup bool, pendingInRange bool, fwlupInRange bool, msStates map[string]*milestoneLoopState) *loopAction {
	if len(history) == 0 {
		// First iteration: implement first undone milestone in range
		inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
		for _, m := range inRange {
			if !milestoneAllDone(m) {
				return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("First iteration — implementing %s", m.ID), MilestoneID: m.ID}
			}
		}
		// All milestones marked done — but verify against actual task counts (range-scoped)
		if pendingInRange {
			// State drift: milestones marked done but tasks still pending within range
			lastMS := inRange[len(inRange)-1]
			if fwlupInRange {
				return &loopAction{Type: actionFixAll, Reason: fmt.Sprintf("State drift: %s marked complete but FWLUP tasks pending — fixing", lastMS.ID), MilestoneID: lastMS.ID}
			}
			return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("State drift: %s marked complete but tasks still pending — reimplementing", lastMS.ID), MilestoneID: lastMS.ID}
		}
		if !fwlupInRange {
			return &loopAction{Type: actionComplete, Reason: "All milestones in range already complete"}
		}
		msID := lastMilestoneID(history)
		return &loopAction{Type: actionImplementNext, Reason: "All milestones done but follow-up tasks remain in range", MilestoneID: msID}
	}

	last := history[len(history)-1]
	lastType := last.Action.Type
	lastSuccess := last.Result != nil && last.Result.Success

	// Rule 1: After IMPLEMENT_MILESTONE success → almost always VERIFY
	if lastType == actionImplementMilestone && lastSuccess {
		wt := last.WorkType
		fc := last.FilesChanged
		// Skip verify only for: 0 files changed, pure docs, or non-critical config ≤2 files
		if fc == 0 {
			msID := last.Action.MilestoneID
			return &loopAction{Type: actionImplementNext, Reason: "No files changed — skipping verification", MilestoneID: msID}
		}
		if wt == workDocs {
			// Docs-only: skip verification, move to next milestone
			inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
			for _, m := range inRange {
				if !milestoneAllDone(m) {
					return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Docs-only milestone — moving to %s", m.ID), MilestoneID: m.ID}
				}
			}
			if pendingInRange {
				msID := last.Action.MilestoneID
				return &loopAction{Type: actionImplementNext, Reason: "Docs-only done but tasks still pending in range", MilestoneID: msID}
			}
			if !fwlupInRange {
				return &loopAction{Type: actionComplete, Reason: "All milestones in range complete (last was docs-only)"}
			}
			msID := last.Action.MilestoneID
			return &loopAction{Type: actionImplementNext, Reason: "Docs-only milestone done, fixing follow-ups in range", MilestoneID: msID}
		}
		// Everything else: verify
		return &loopAction{Type: actionVerify, Reason: "Verifying completed milestone", MilestoneID: last.Action.MilestoneID}
	}

	// Rule 2: After VERIFY success + no follow-ups in this milestone → next undone milestone or COMPLETE
	if lastType == actionVerify && lastSuccess && !hasMsFwlup {
		inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
		for _, m := range inRange {
			if !milestoneAllDone(m) {
				return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Verification passed — implementing %s", m.ID), MilestoneID: m.ID}
			}
		}
		if pendingInRange {
			msID := lastMilestoneID(history)
			return &loopAction{Type: actionImplementNext, Reason: "All milestones marked done but tasks still pending in range after verification", MilestoneID: msID}
		}
		// All in-range milestones done and verified — complete
		return &loopAction{Type: actionComplete, Reason: "All milestones in range verified and complete"}
	}

	// Rule 3: After VERIFY success + follow-ups exist in this milestone → TRIAGE (AI reads the actual FWLUPs)
	if lastType == actionVerify && lastSuccess && hasMsFwlup {
		msID := lastMilestoneID(history)
		return &loopAction{Type: actionTriage, Reason: "Triaging follow-up tasks after verification", MilestoneID: msID}
	}

	// Rule 3b: After TRIAGE → decide based on triage decision
	if lastType == actionTriage && lastSuccess {
		decision := last.Action.TriageDecision
		switch decision {
		case "defer_and_proceed":
			// Triage already moved FWLUPs to NOTES.md — proceed to next milestone
			inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
			for _, m := range inRange {
				if !milestoneAllDone(m) {
					return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Triage deferred polish items — implementing %s", m.ID), MilestoneID: m.ID}
				}
			}
			if pendingInRange {
				msID := lastMilestoneID(history)
				return &loopAction{Type: actionImplementNext, Reason: "Triage deferred polish but tasks still pending in range", MilestoneID: msID}
			}
			return &loopAction{Type: actionComplete, Reason: "All milestones in range complete (remaining items deferred as polish)"}
		case "fix_and_proceed":
			return &loopAction{Type: actionFixAll, Reason: "Fixing all blocking follow-ups (will skip re-verification)", TriageDecision: "fix_and_proceed", MilestoneID: last.Action.MilestoneID}
		case "fix_and_reverify":
			return &loopAction{Type: actionFixAll, Reason: "Fixing all blocking follow-ups (will re-verify after)", TriageDecision: "fix_and_reverify", ReverifyScope: last.Action.ReverifyScope, MilestoneID: last.Action.MilestoneID}
		default:
			// Unknown triage decision — fall back to fix-all with re-verify
			return &loopAction{Type: actionFixAll, Reason: "Fixing follow-ups after triage", TriageDecision: "fix_and_reverify", ReverifyScope: "focused", MilestoneID: last.Action.MilestoneID}
		}
	}

	// Rule 3c: After FIX_ALL success → check triage decision for next step
	if lastType == actionFixAll && lastSuccess {
		decision := last.Action.TriageDecision
		if decision == "fix_and_reverify" {
			scope := last.Action.ReverifyScope
			if scope == "" {
				scope = "focused"
			}
			return &loopAction{Type: actionVerify, Reason: fmt.Sprintf("Re-verifying after follow-up fixes (scope: %s)", scope), ReverifyScope: scope, MilestoneID: last.Action.MilestoneID}
		}
		// fix_and_proceed: skip re-verification, move to next milestone
		inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
		for _, m := range inRange {
			if !milestoneAllDone(m) {
				return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Follow-ups fixed — implementing %s", m.ID), MilestoneID: m.ID}
			}
		}
		if pendingInRange {
			msID := lastMilestoneID(history)
			return &loopAction{Type: actionImplementNext, Reason: "Follow-ups fixed but tasks still pending in range", MilestoneID: msID}
		}
		if !fwlupInRange {
			return &loopAction{Type: actionComplete, Reason: "All milestones in range complete after follow-up fixes"}
		}
		return &loopAction{Type: actionTriage, Reason: "Follow-ups remain in range after fix-all — re-triaging"}
	}

	// Rule 4: After IMPLEMENT_NEXT success → verify only if milestone hasn't been verified yet
	if lastType == actionImplementNext && lastSuccess {
		msID := lastMilestoneID(history)
		// If this milestone already had a clean verify pass, skip re-verification
		if msID != "" {
			if s, ok := msStates[msID]; ok && s.VerifySucceeded >= 1 {
				// Move to next undone milestone or complete
				inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
				for _, m := range inRange {
					if !milestoneAllDone(m) {
						return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Milestone %s already verified — moving to %s", msID, m.ID), MilestoneID: m.ID}
					}
				}
				if !pendingInRange && !fwlupInRange {
					return &loopAction{Type: actionComplete, Reason: "All milestones in range complete (already verified)"}
				}
				// Still pending in range — continue fixing within scope
				return &loopAction{Type: actionImplementNext, Reason: "Fixing remaining in-range tasks", MilestoneID: msID}
			}
		}
		return &loopAction{Type: actionVerify, Reason: "Verifying after follow-up fix", MilestoneID: msID}
	}

	// Rule 5: After VERIFY failure — check verify failure count
	if lastType == actionVerify && !lastSuccess {
		// Find the milestone being verified
		var verifyFailCount int
		var targetMS string
		// Walk history backward to find which milestone we're verifying
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Action.Type == actionImplementMilestone && history[i].Action.MilestoneID != "" {
				targetMS = history[i].Action.MilestoneID
				break
			}
		}
		if targetMS != "" {
			if s, ok := msStates[targetMS]; ok {
				verifyFailCount = s.VerifyFailed
			}
		}

		if verifyFailCount >= 2 {
			// Delegate to AI for REPLAN/DEBUG decision
			return nil
		}
		// First failure: try fixing (scoped to the target milestone)
		return &loopAction{Type: actionImplementNext, Reason: "Verification failed — fixing issues", MilestoneID: targetMS}
	}

	// Rule 6: After DEBUG success → VERIFY
	if lastType == actionDebug && lastSuccess {
		return &loopAction{Type: actionVerify, Reason: "Re-verifying after debug"}
	}

	// Rule 7: All milestones done + verified + no follow-ups in range → COMPLETE
	inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
	allDone := true
	allVerified := true
	for _, m := range inRange {
		if !milestoneAllDone(m) {
			allDone = false
			break
		}
		if s, ok := msStates[m.ID]; ok {
			if s.VerifySucceeded == 0 {
				allVerified = false
			}
		}
	}
	if allDone && allVerified && !fwlupInRange && !pendingInRange {
		return &loopAction{Type: actionComplete, Reason: "All milestones in range implemented, verified, and no follow-ups"}
	}
	if allDone && allVerified && !fwlupInRange && pendingInRange {
		msID := lastMilestoneID(history)
		return &loopAction{Type: actionImplementNext, Reason: "All milestones in range marked done and verified but tasks still pending", MilestoneID: msID}
	}

	// Rule 8: All done but not all verified → VERIFY (only milestones never successfully verified)
	if allDone && !allVerified && !fwlupInRange {
		for _, m := range inRange {
			if s, ok := msStates[m.ID]; ok && s.VerifySucceeded == 0 {
				return &loopAction{Type: actionVerify, Reason: fmt.Sprintf("Verifying %s (not yet verified)", m.ID), MilestoneID: m.ID}
			}
		}
		// All have been verified at least once — complete
		return &loopAction{Type: actionComplete, Reason: "All milestones in range verified at least once"}
	}

	// Rule 9: All done but follow-ups remain in range → TRIAGE
	if allDone && fwlupInRange {
		return &loopAction{Type: actionTriage, Reason: "Triaging remaining follow-up tasks in range"}
	}

	// Rule 10: Next undone milestone
	for _, m := range inRange {
		if !milestoneAllDone(m) {
			return &loopAction{Type: actionImplementMilestone, Reason: fmt.Sprintf("Implementing milestone %s", m.ID), MilestoneID: m.ID}
		}
	}

	// Ambiguous — let AI decide
	return nil
}

// checkHardGuardrails runs safety checks that always apply before AI decisions.
// Returns nil if no guardrail triggers, otherwise a loopAction to take.
func checkHardGuardrails(report statusReport, history []historyEntry, cfg loopConfig) *loopAction {
	// Blocked tasks → PAUSE
	blocked := blockedTaskNames(report.Milestones)
	if len(blocked) > 0 {
		return &loopAction{Type: actionPause, Reason: fmt.Sprintf("Blocked tasks: %s", strings.Join(blocked, ", "))}
	}

	// Consecutive failures >= maxFailures → ERROR
	if consecutiveFailures(history) >= cfg.MaxFailures {
		return &loopAction{Type: actionError, Reason: fmt.Sprintf("%d consecutive failures", cfg.MaxFailures)}
	}

	return nil
}

// decideLoopActionAI shells out to the configured tool to make a strategic decision.
// Only called for ambiguous cases that decideLoopActionSmart couldn't handle.
func decideLoopActionAI(report statusReport, history []historyEntry, cfg loopConfig, hasFwlup bool, lastOutput string, msStates map[string]*milestoneLoopState) (*loopAction, error) {
	// Build rich milestone state JSON
	inRange := milestonesInRange(report.Milestones, cfg.From, cfg.To)
	type msStateJSON struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Done           bool   `json:"done"`
		Implemented    bool   `json:"implemented"`
		Verified       bool   `json:"verified"`
		VerifyFailures  int    `json:"verify_failures,omitempty"`
		VerifySuccesses int    `json:"verify_successes,omitempty"`
		WorkType        string `json:"work_type,omitempty"`
		FilesChanged   int    `json:"files_changed,omitempty"`
	}
	var milestones []msStateJSON
	for _, m := range inRange {
		ms := msStateJSON{ID: m.ID, Name: m.Name, Done: milestoneAllDone(m)}
		if s, ok := msStates[m.ID]; ok {
			ms.Implemented = s.Implemented
			ms.Verified = s.Verified
			ms.VerifyFailures = s.VerifyFailed
			ms.VerifySuccesses = s.VerifySucceeded
			ms.WorkType = string(s.WorkType)
			ms.FilesChanged = s.FilesChanged
		}
		milestones = append(milestones, ms)
	}

	// Build recent history (last 5)
	type histItem struct {
		Action    string `json:"action"`
		Milestone string `json:"milestone,omitempty"`
		Success   bool   `json:"success"`
		WorkType  string `json:"work_type,omitempty"`
		Output    string `json:"output,omitempty"`
	}
	var recentHistory []histItem
	start := len(history) - 5
	if start < 0 {
		start = 0
	}
	for _, h := range history[start:] {
		item := histItem{
			Action:    string(h.Action.Type),
			Milestone: h.Action.MilestoneID,
			WorkType:  string(h.WorkType),
		}
		if h.Result != nil {
			item.Success = h.Result.Success
			item.Output = truncateTail(h.Result.Output, 500)
		}
		recentHistory = append(recentHistory, item)
	}

	// Determine why we're asking the AI (ambiguity reason)
	ambiguityReason := "Smart rules could not determine the next action"
	if len(history) > 0 {
		last := history[len(history)-1]
		if last.Action.Type == actionVerify && last.Result != nil && !last.Result.Success {
			// Find which milestone
			var targetMS string
			for i := len(history) - 1; i >= 0; i-- {
				if history[i].Action.Type == actionImplementMilestone && history[i].Action.MilestoneID != "" {
					targetMS = history[i].Action.MilestoneID
					break
				}
			}
			if targetMS != "" {
				if s, ok := msStates[targetMS]; ok && s.VerifyFailed >= 2 {
					ambiguityReason = fmt.Sprintf("%s failed verification %d times — consider REPLAN or DEBUG", targetMS, s.VerifyFailed)
				}
			}
		}
	}

	state := map[string]interface{}{
		"tasks_done":       report.TaskCounts["done"] + report.TaskCounts["verified"],
		"tasks_total":      report.TaskCounts["total"],
		"milestone_states": milestones,
		"has_followup":     hasFwlup,
		"blocker_count":    blockedTaskCount(report.Milestones),
		"last_5_actions":   recentHistory,
		"ambiguity_reason": ambiguityReason,
	}
	if cfg.From != "" {
		state["milestone_from"] = cfg.From
	}
	if cfg.To != "" {
		state["milestone_to"] = cfg.To
	}
	if lastOutput != "" {
		state["previous_output"] = truncateTail(lastOutput, 1500)
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("marshal state: %w", err)
	}

	// Load prompt template
	tmpl, tmplErr := loadPromptTemplate("ai-decision")
	var prompt string
	if tmplErr != nil {
		// Fallback to inline prompt if template not found
		prompt = fmt.Sprintf(`You are a loop controller for an automated feature implementation system.
You are ONLY called for ambiguous cases — simple decisions are already handled by deterministic rules.

STATE:
%s

AVAILABLE ACTIONS:
- IMPLEMENT_MILESTONE: Implement next incomplete milestone (set milestone_id)
- IMPLEMENT_NEXT: Fix a single follow-up task
- VERIFY: Run verification on completed milestones
- TRIAGE: Run AI triage to classify follow-up tasks as blocking vs polish
- FIX_ALL: Fix all blocking follow-up tasks in batch before re-verification
- REPLAN: Re-run tech planning when current approach has systemic issues
- DEBUG: Run automated debugging when verification keeps failing on recurring issues
- SKIP_MILESTONE: Skip a blocked milestone (set milestone_id)
- COMPLETE: All work in scope is done and verified
- PAUSE: Stop for human intervention

HARD RULES:
1. You are ONLY called for ambiguous cases — simple decisions are already handled.
2. Never skip verification for frontend/UI milestones.
3. If verification failed 2+ times on the SAME issue, choose REPLAN or DEBUG.
4. If verification failed on DIFFERENT issues each time, one more VERIFY is reasonable.
5. If a milestone has recurring failures across multiple cycles, use DEBUG.
6. Use SKIP_MILESTONE only when a milestone truly cannot proceed due to external blockers.
7. If all milestones in range are done+verified with no follow-ups, COMPLETE.
8. Prefer TRIAGE over IMPLEMENT_NEXT when follow-ups exist — let triage classify issues before fixing.

Respond with ONLY valid JSON: {"action":"...","reason":"...","milestone_id":"..."}`, string(stateJSON))
	} else {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, map[string]string{"StateJSON": string(stateJSON)}); err != nil {
			return nil, fmt.Errorf("execute prompt template: %w", err)
		}
		prompt = buf.String()
	}

	// AI decision calls are short classification tasks — use the low tier.
	decisionFlags := resolveModelFlags(cfg.Tool, "low")
	cmd := buildToolCommand(cfg.Tool, prompt, cfg.Root, decisionFlags...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tool execution: %w (output: %s)", err, truncateTail(string(output), 200))
	}

	decisionJSON, err := extractDecisionJSON(string(output), cfg.Tool)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var decision aiDecision
	if err := json.Unmarshal([]byte(decisionJSON), &decision); err != nil {
		return nil, fmt.Errorf("unmarshal decision: %w", err)
	}

	// Validate action type
	actionType := loopActionType(decision.Action)
	switch actionType {
	case actionImplementMilestone, actionImplementNext, actionVerify,
		actionReplan, actionSkipMilestone, actionComplete, actionPause, actionDebug,
		actionTriage, actionFixAll:
		// valid
	default:
		return nil, fmt.Errorf("unknown action %q from AI", decision.Action)
	}

	// Validate milestone_id required for certain actions
	if (actionType == actionImplementMilestone || actionType == actionSkipMilestone) && decision.MilestoneID == "" {
		return nil, fmt.Errorf("action %s requires milestone_id", actionType)
	}

	return &loopAction{
		Type:        actionType,
		Reason:      decision.Reason,
		MilestoneID: decision.MilestoneID,
	}, nil
}

// buildToolCommand creates an exec.Cmd for the given tool with a prompt.
// Used by both AI decision calls and action execution.
func buildToolCommand(tool, prompt, root string, extraFlags ...string) *exec.Cmd {
	var cmd *exec.Cmd
	switch tool {
	case "claude":
		args := []string{"-p", prompt,
			"--permission-mode", "bypassPermissions",
			"--allowedTools", "Bash Read Write Edit Glob Grep Agent Skill WebFetch WebSearch mcp__*",
			"--output-format", "json"}
		args = append(args, extraFlags...)
		cmd = exec.Command("claude", args...)
	case "codex":
		args := []string{"exec", prompt,
			"--dangerously-bypass-approvals-and-sandbox",
			"--json", "-C", root}
		args = append(args, extraFlags...)
		cmd = exec.Command("codex", args...)
	case "gemini":
		args := []string{prompt, "--yolo", "--output-format", "json"}
		args = append(args, extraFlags...)
		cmd = exec.Command("gemini", args...)
	case "copilot":
		args := []string{"-p", prompt, "--yolo"}
		args = append(args, extraFlags...)
		cmd = exec.Command("copilot", args...)
	case "cursor":
		args := []string{"agent", "-p", prompt, "--force", "--output-format", "json"}
		args = append(args, extraFlags...)
		cmd = exec.Command("cursor", args...)
	default:
		cmd = exec.Command("echo", "unsupported tool")
	}
	cmd.Dir = root
	return cmd
}

// extractDecisionJSON finds the JSON object in tool output.
// Tools may wrap output in their own JSON structure.
func extractDecisionJSON(output, tool string) (string, error) {
	// For tools that return JSON wrapper (claude, codex, gemini, cursor),
	// try to extract the text content first
	text := output

	// Try to extract result text from claude's JSON wrapper
	if tool == "claude" || tool == "codex" || tool == "gemini" || tool == "cursor" {
		var wrapper struct {
			Result string `json:"result"`
		}
		if err := json.Unmarshal([]byte(output), &wrapper); err == nil && wrapper.Result != "" {
			text = wrapper.Result
		}
	}

	// Find the JSON object with action field
	re := regexp.MustCompile(`\{[^{}]*"action"\s*:\s*"[^"]+?"[^{}]*\}`)
	match := re.FindString(text)
	if match == "" {
		return "", fmt.Errorf("no decision JSON found in output: %s", truncateTail(text, 200))
	}
	return match, nil
}

type triageDecision struct {
	Decision      string   `json:"decision"`
	BlockingTasks []string `json:"blocking_tasks"`
	DeferredTasks []string `json:"deferred_tasks"`
	Reason        string   `json:"reason"`
	ReverifyScope string   `json:"reverify_scope"`
}

// parseTriageDecision extracts the triage JSON decision from the tool output.
func parseTriageDecision(output string) *triageDecision {
	// Find JSON object with "decision" field
	re := regexp.MustCompile(`\{[^{}]*"decision"\s*:\s*"[^"]+?"[^{}]*\}`)
	match := re.FindString(output)
	if match == "" {
		// Try to find it in the last 2000 chars (triage outputs it at the end)
		tail := output
		if len(tail) > 2000 {
			tail = tail[len(tail)-2000:]
		}
		match = re.FindString(tail)
		if match == "" {
			return nil
		}
	}
	var td triageDecision
	if err := json.Unmarshal([]byte(match), &td); err != nil {
		return nil
	}
	if td.Decision == "" {
		return nil
	}
	return &td
}

// truncateTail returns the last maxLen characters of s.
func truncateTail(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[len(s)-maxLen:]
}

// skipMilestoneInProgress marks all incomplete tasks in a milestone as done in PROGRESS.md.
func skipMilestoneInProgress(root, feature, milestoneID string) error {
	progressPath := filepath.Join(root, ".belmont", "features", feature, "PROGRESS.md")
	content, err := os.ReadFile(progressPath)
	if err != nil {
		return fmt.Errorf("read PROGRESS.md: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	msRe := regexp.MustCompile(`(?i)^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):`)
	taskRe := regexp.MustCompile(`^(\s*-\s+)\[[ >!]\](\s+.*)$`)

	inTarget := false
	changed := false
	for i, line := range lines {
		if m := msRe.FindStringSubmatch(line); len(m) >= 2 {
			inTarget = ("M"+m[1]) == milestoneID
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			inTarget = false
			continue
		}
		if inTarget {
			if taskMatch := taskRe.FindStringSubmatch(line); len(taskMatch) >= 3 {
				lines[i] = taskMatch[1] + "[x]" + taskMatch[2]
				changed = true
			}
		}
	}

	if !changed {
		return fmt.Errorf("milestone %s not found or already done", milestoneID)
	}

	return os.WriteFile(progressPath, []byte(strings.Join(lines, "\n")), 0644)
}

func detectFwlupTasks(root, feature string, report statusReport) bool {
	fwlupRe := regexp.MustCompile(`(?i)FWLUP`)
	// Check if any todo/in_progress tasks have FWLUP in their ID or name
	for _, t := range report.Tasks {
		if t.Status == taskTodo || t.Status == taskInProgress {
			if fwlupRe.MatchString(t.ID) || fwlupRe.MatchString(t.Name) {
				return true
			}
		}
	}
	return false
}

// extractMilestoneFromTaskID extracts the milestone ID from a task ID like "P5-M5-FWLUP-1" → "M5".
func extractMilestoneFromTaskID(taskID string) string {
	re := regexp.MustCompile(`P\d+-M(\d+)`)
	m := re.FindStringSubmatch(taskID)
	if len(m) >= 2 {
		return "M" + m[1]
	}
	return ""
}

// detectFwlupTasksForMilestone checks for pending FWLUP tasks scoped to a specific milestone.
func detectFwlupTasksForMilestone(root, feature string, report statusReport, milestoneID string) bool {
	if milestoneID == "" {
		return false
	}
	fwlupRe := regexp.MustCompile(`(?i)FWLUP`)
	for _, t := range report.Tasks {
		if t.Status == taskTodo || t.Status == taskInProgress {
			if fwlupRe.MatchString(t.ID) || fwlupRe.MatchString(t.Name) {
				// Tasks now carry their milestone ID from PROGRESS.md
				if t.MilestoneID == milestoneID || extractMilestoneFromTaskID(t.ID) == milestoneID {
					return true
				}
			}
		}
	}
	return false
}

// hasPendingTasks returns true if any task in the report is todo or in progress.
func hasPendingTasks(report statusReport) bool {
	for _, t := range report.Tasks {
		if t.Status == taskTodo || t.Status == taskInProgress {
			return true
		}
	}
	return false
}

// pendingTasksInRange checks for incomplete tasks under milestones
// that fall within the from/to range in the feature's PROGRESS.md.
// When from and to are both empty, falls back to checking all milestones.
func pendingTasksInRange(root, feature, from, to string) bool {
	progressPath := filepath.Join(root, ".belmont", "features", feature, "PROGRESS.md")
	data, err := os.ReadFile(progressPath)
	if err != nil {
		return false
	}

	fromNum := parseMilestoneNum(from)
	toNum := parseMilestoneNum(to)

	lines := strings.Split(string(data), "\n")
	msRe := regexp.MustCompile(`(?i)^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):`)
	// Match any incomplete task: [ ], [>], [!]
	taskRe := regexp.MustCompile(`^\s*-\s+\[[ >!]\]`)

	inRange := fromNum < 0 && toNum < 0 // if no range, all milestones are in range
	for _, line := range lines {
		if m := msRe.FindStringSubmatch(line); len(m) >= 2 {
			num, _ := strconv.Atoi(m[1])
			inRange = (fromNum < 0 || num >= fromNum) && (toNum < 0 || num <= toNum)
			continue
		}
		if inRange && taskRe.MatchString(line) {
			return true
		}
	}
	return false
}

// fwlupTasksInRange checks for unchecked FWLUP tasks under milestones within the from/to range.
// When from and to are both empty, falls back to the global detectFwlupTasks.
func fwlupTasksInRange(root, feature string, report statusReport, from, to string) bool {
	if from == "" && to == "" {
		return detectFwlupTasks(root, feature, report)
	}

	progressPath := filepath.Join(root, ".belmont", "features", feature, "PROGRESS.md")
	data, err := os.ReadFile(progressPath)
	if err != nil {
		return false
	}

	fromNum := parseMilestoneNum(from)
	toNum := parseMilestoneNum(to)

	lines := strings.Split(string(data), "\n")
	msRe := regexp.MustCompile(`(?i)^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):`)
	// Match any incomplete task with FWLUP in the text
	fwlupTaskRe := regexp.MustCompile(`(?i)^\s*-\s+\[[ >!]\].*FWLUP`)

	inRange := false
	for _, line := range lines {
		if m := msRe.FindStringSubmatch(line); len(m) >= 2 {
			num, _ := strconv.Atoi(m[1])
			inRange = (fromNum < 0 || num >= fromNum) && (toNum < 0 || num <= toNum)
			continue
		}
		if inRange && fwlupTaskRe.MatchString(line) {
			return true
		}
	}
	return false
}

// loadPromptTemplate loads a prompt template from embedded FS or source filesystem.
func loadPromptTemplate(name string) (*template.Template, error) {
	filename := name + ".md"

	// Try embedded first
	if hasEmbeddedFiles {
		data, err := fs.ReadFile(embeddedPrompts, filepath.Join("prompts", "belmont", filename))
		if err == nil {
			return template.New(name).Parse(string(data))
		}
	}

	// Try source resolution
	sourceRoot := resolveSourceForPrompts()
	if sourceRoot == "" {
		return nil, fmt.Errorf("prompt %q: no embedded files and no source directory found", name)
	}

	data, err := os.ReadFile(filepath.Join(sourceRoot, "prompts", "belmont", filename))
	if err != nil {
		return nil, fmt.Errorf("prompt %q: %w", name, err)
	}
	return template.New(name).Parse(string(data))
}

// resolveSourceForPrompts returns the belmont source directory path, or "" if not found.
func resolveSourceForPrompts() string {
	if src := os.Getenv("BELMONT_SOURCE"); src != "" {
		return src
	}

	configDir, err := os.UserConfigDir()
	if err == nil {
		configPath := filepath.Join(configDir, "belmont", "config.json")
		if data, err := os.ReadFile(configPath); err == nil {
			var cfg config
			if json.Unmarshal(data, &cfg) == nil && cfg.Source != "" {
				return cfg.Source
			}
		}
	}

	// Walk up from binary location
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	for {
		if _, err := os.Stat(filepath.Join(dir, "prompts", "belmont")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// isTerminal returns true if the given file is a terminal.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// interactiveMilestoneSelect shows milestones and lets user pick a range.
func interactiveMilestoneSelect(milestones []milestone) (from, to string, err error) {
	if len(milestones) == 0 {
		return "", "", nil
	}

	fmt.Fprintf(os.Stderr, "\033[1mMilestones:\033[0m\n")
	firstUndone := ""
	for _, m := range milestones {
		marker := "[ ]"
		if milestoneAllDone(m) {
			marker = "[x]"
		}
		if !milestoneAllDone(m) && firstUndone == "" {
			firstUndone = m.ID
		}
		depStr := ""
		if len(m.Deps) > 0 {
			depStr = fmt.Sprintf(" \033[2m(depends: %s)\033[0m", strings.Join(m.Deps, ", "))
		}
		fmt.Fprintf(os.Stderr, "  %s %s: %s%s\n", marker, m.ID, m.Name, depStr)
	}

	lastID := milestones[len(milestones)-1].ID
	defaultRange := ""
	if firstUndone != "" {
		defaultRange = fmt.Sprintf("%s → %s", firstUndone, lastID)
	}

	fmt.Fprintf(os.Stderr, "\n\033[2mDefault range: %s\033[0m\n", defaultRange)
	fmt.Fprintf(os.Stderr, "Press Enter to accept, 'q' to quit, or enter range (e.g. M2 M5): ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", "", fmt.Errorf("auto: no input")
	}
	input := strings.TrimSpace(scanner.Text())

	if input == "q" || input == "quit" || input == "exit" {
		return "", "", fmt.Errorf("auto: cancelled by user")
	}

	if input == "" {
		// Accept defaults
		return "", "", nil
	}

	// Parse custom range
	parts := strings.Fields(input)
	if len(parts) == 1 {
		return parts[0], parts[0], nil
	}
	if len(parts) == 2 {
		return parts[0], parts[1], nil
	}

	return "", "", fmt.Errorf("auto: invalid range %q — use 'M2 M5' format", input)
}

// wave represents a group of milestones that can execute in parallel.
type wave struct {
	Index      int
	Milestones []milestone
}

// computeWaves groups milestones into waves using Kahn's algorithm for topological sort.
// Milestones in the same wave have all deps satisfied by prior waves.
// Already-done milestones satisfy deps but don't execute.
func computeWaves(milestones []milestone) ([]wave, error) {
	if len(milestones) == 0 {
		return nil, nil
	}

	// Build ID -> milestone map
	byID := make(map[string]milestone)
	for _, m := range milestones {
		byID[m.ID] = m
	}

	// Compute in-degree for each undone milestone
	inDegree := make(map[string]int)
	for _, m := range milestones {
		if milestoneAllDone(m) {
			continue
		}
		count := 0
		for _, dep := range m.Deps {
			if dm, ok := byID[dep]; ok && !milestoneAllDone(dm) {
				count++
			}
		}
		inDegree[m.ID] = count
	}

	var waves []wave
	remaining := len(inDegree)
	waveIdx := 0

	for remaining > 0 {
		// Find all milestones with zero in-degree
		var ready []milestone
		for id, deg := range inDegree {
			if deg == 0 {
				ready = append(ready, byID[id])
			}
		}

		if len(ready) == 0 {
			// Cycle detected
			var cycleIDs []string
			for id := range inDegree {
				cycleIDs = append(cycleIDs, id)
			}
			sort.Strings(cycleIDs)
			return nil, fmt.Errorf("dependency cycle detected among milestones: %s", strings.Join(cycleIDs, ", "))
		}

		// Sort ready milestones by ID for deterministic ordering
		sort.Slice(ready, func(i, j int) bool {
			return parseMilestoneNum(ready[i].ID) < parseMilestoneNum(ready[j].ID)
		})

		waves = append(waves, wave{Index: waveIdx, Milestones: ready})
		waveIdx++

		// Remove completed milestones and update in-degrees
		for _, m := range ready {
			delete(inDegree, m.ID)
			remaining--
		}
		for id, deg := range inDegree {
			m := byID[id]
			newDeg := deg
			for _, dep := range m.Deps {
				for _, completed := range ready {
					if dep == completed.ID {
						newDeg--
					}
				}
			}
			inDegree[id] = newDeg
		}
	}

	return waves, nil
}

// runAutoParallel executes milestones with dependency-aware parallel waves using worktrees.
func runAutoParallel(cfg loopConfig, milestones []milestone) error {
	startTime := time.Now()

	// Pre-flight: ensure repo is in a clean state before starting
	if err := validateRepoState(cfg.Root); err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	// Record original branch and restore on exit if changed
	origBranch := getCurrentBranch(cfg.Root)
	defer func() {
		if origBranch != "" && origBranch != "HEAD" {
			if cur := getCurrentBranch(cfg.Root); cur != origBranch {
				fmt.Fprintf(os.Stderr, "\033[33m⚠ Branch changed from %s to %s — restoring...\033[0m\n", origBranch, cur)
				restoreCmd := exec.Command("git", "checkout", origBranch)
				restoreCmd.Dir = cfg.Root
				restoreCmd.Run()
			}
		}
	}()

	// Ensure the sibling worktree base directory exists
	if err := os.MkdirAll(worktreeBasePath(cfg.Root), 0755); err != nil {
		return fmt.Errorf("create worktree base dir: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\033[1mBelmont Auto (parallel) — %s\033[0m\n", cfg.Feature)
	fmt.Fprintf(os.Stderr, "\033[2mTool: %s | Max parallel: %d\033[0m\n", cfg.Tool, cfg.MaxParallel)

	waves, err := computeWaves(milestones)
	if err != nil {
		return fmt.Errorf("auto: %w", err)
	}

	if len(waves) == 0 {
		fmt.Fprintf(os.Stderr, "\n\033[32m✓ Complete\033[0m — all milestones already done\n")
		return nil
	}

	// Print wave plan
	fmt.Fprintf(os.Stderr, "\n\033[1mExecution plan:\033[0m\n")
	for _, w := range waves {
		var ids []string
		for _, m := range w.Milestones {
			ids = append(ids, m.ID)
		}
		parallel := ""
		if len(w.Milestones) > 1 {
			parallel = " (parallel)"
		}
		fmt.Fprintf(os.Stderr, "  Wave %d: %s%s\n", w.Index+1, strings.Join(ids, ", "), parallel)
	}
	fmt.Fprintln(os.Stderr)

	// Set up signal handler for cleanup
	activeWorktrees := &worktreeTracker{root: cfg.Root, entries: make(map[string]worktreeEntry), hooks: loadWorktreeHooks(cfg.Root)}
	sigCh := make(chan os.Signal, 1)
	notifySignals(sigCh)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ Interrupted — preserving worktrees for resume...\033[0m\n")
		activeWorktrees.gracefulShutdown(cfg.Root)
		os.Exit(1)
	}()

	for _, w := range waves {
		fmt.Fprintf(os.Stderr, "\033[1m━━ Wave %d ━━\033[0m\n", w.Index+1)

		if len(w.Milestones) == 1 && !singleMilestoneHasExistingWorktree(cfg, w.Milestones[0]) {
			// Single milestone with no existing worktree: run directly in
			// main tree (skip worktree overhead for fresh work). If a branch
			// or worktree dir already exists we fall through to the wave
			// path so the user gets a resume prompt and any worktree-local
			// state (STEERING.md, follow-up commits, etc.) is honoured.
			m := w.Milestones[0]
			fmt.Fprintf(os.Stderr, "  Running %s: %s\n", m.ID, m.Name)
			mCfg := cfg
			mCfg.From = m.ID
			mCfg.To = m.ID
			if err := runLoop(mCfg); err != nil {
				return fmt.Errorf("auto: wave %d, %s failed: %w", w.Index+1, m.ID, err)
			}
		} else {
			// Multiple milestones OR single milestone with an existing
			// worktree: run via worktrees so resume prompts fire and
			// isolation holds.
			if err := runWaveParallel(cfg, w, activeWorktrees); err != nil {
				return err
			}
		}

		fmt.Fprintf(os.Stderr, "\033[32m  ✓ Wave %d complete\033[0m\n\n", w.Index+1)
	}

	// Sync master PROGRESS.md with actual feature states
	featuresDir := filepath.Join(cfg.Root, ".belmont", "features")
	syncMasterFeatureStatuses(cfg.Root, listFeatures(featuresDir, 50))

	// Commit any remaining .belmont/ state changes
	if err := commitBelmontState(cfg.Root); err != nil {
		fmt.Fprintf(os.Stderr, "\033[33m⚠ Failed to commit final .belmont/ state: %s\033[0m\n", err)
	}

	// Clean up auto.json
	activeWorktrees.removeAutoJSON()

	fmt.Fprintf(os.Stderr, "\n\033[32m✓ All waves complete\033[0m (%.1fs total)\n", time.Since(startTime).Seconds())
	return nil
}

// worktreeEntry stores both the path and branch name for a worktree.
type worktreeEntry struct {
	Path   string
	Branch string
	Port   int
	Pgid   int // process group ID for cleanup
}

// worktreeTracker keeps track of active worktrees for cleanup on interrupt.
type worktreeTracker struct {
	mu      sync.Mutex
	root    string                   // project root for persisting auto.json
	entries map[string]worktreeEntry // ID -> worktree entry
	hooks   *worktreeHooks          // shared hooks config (nil if no worktree.json)
}

// autoJSON is the on-disk format for .belmont/auto.json, enabling belmont status
// to discover active worktrees and read live feature state from them.
type autoJSON struct {
	Active    bool                       `json:"active"`
	Started   string                     `json:"started"`
	Mode      string                     `json:"mode,omitempty"`    // "single-feature" or "parallel" or "multi-feature"
	Feature   string                     `json:"feature,omitempty"` // active feature slug (single-feature mode)
	From      string                     `json:"from,omitempty"`    // milestone range start
	To        string                     `json:"to,omitempty"`      // milestone range end
	Worktrees map[string]autoJSONEntry   `json:"worktrees"`
}

type autoJSONEntry struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

func (wt *worktreeTracker) add(id, path, branch string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	wt.entries[id] = worktreeEntry{Path: path, Branch: branch}
	wt.persistAutoJSON()
}

func (wt *worktreeTracker) setPort(id string, port int) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	if entry, ok := wt.entries[id]; ok {
		entry.Port = port
		wt.entries[id] = entry
	}
}

func (wt *worktreeTracker) setPgid(id string, pgid int) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	if entry, ok := wt.entries[id]; ok {
		entry.Pgid = pgid
		wt.entries[id] = entry
	}
}

func (wt *worktreeTracker) remove(id string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	delete(wt.entries, id)
	wt.persistAutoJSON()
}

// persistAutoJSON writes .belmont/auto.json so belmont status can discover active worktrees.
// Must be called with wt.mu held.
func (wt *worktreeTracker) persistAutoJSON() {
	if wt.root == "" {
		return
	}
	aj := autoJSON{
		Active:    len(wt.entries) > 0,
		Started:   time.Now().UTC().Format(time.RFC3339),
		Worktrees: make(map[string]autoJSONEntry),
	}
	for id, entry := range wt.entries {
		aj.Worktrees[id] = autoJSONEntry{Path: entry.Path, Branch: entry.Branch}
	}
	data, err := json.MarshalIndent(aj, "", "  ")
	if err != nil {
		return
	}
	autoPath := filepath.Join(wt.root, ".belmont", "auto.json")
	os.WriteFile(autoPath, data, 0644) // best-effort
}

// writeLoopAutoJSON writes a minimal auto.json for single-feature runLoop mode,
// enabling belmont status to show what's being worked on.
func writeLoopAutoJSON(autoPath string, cfg loopConfig) {
	aj := autoJSON{
		Active:    true,
		Started:   time.Now().UTC().Format(time.RFC3339),
		Mode:      "single-feature",
		Feature:   cfg.Feature,
		From:      cfg.From,
		To:        cfg.To,
		Worktrees: make(map[string]autoJSONEntry),
	}
	data, err := json.MarshalIndent(aj, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(autoPath, data, 0644) // best-effort
}

// removeAutoJSON deletes .belmont/auto.json when auto is done.
func (wt *worktreeTracker) removeAutoJSON() {
	if wt.root == "" {
		return
	}
	os.Remove(filepath.Join(wt.root, ".belmont", "auto.json"))
}

// teardownEntry runs teardown hooks for a worktree entry (if configured).
func (wt *worktreeTracker) teardownEntry(id string) {
	wt.mu.Lock()
	entry, ok := wt.entries[id]
	hooks := wt.hooks
	wt.mu.Unlock()
	if !ok {
		return
	}
	if entry.Pgid != 0 {
		signalProcessGroup(entry.Pgid)
	}
	if hooks != nil && len(hooks.Teardown) > 0 {
		_ = runWorktreeHookCommands(hooks.Teardown, entry.Path, entry.Port, hooks.Env)
	}
}

func (wt *worktreeTracker) cleanupAll(root string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	for id, entry := range wt.entries {
		fmt.Fprintf(os.Stderr, "  Cleaning up worktree for %s...\n", id)
		// Kill process group if running
		if entry.Pgid != 0 {
			signalProcessGroup(entry.Pgid)
		}
		// Run teardown hooks
		if wt.hooks != nil && len(wt.hooks.Teardown) > 0 {
			_ = runWorktreeHookCommands(wt.hooks.Teardown, entry.Path, entry.Port, wt.hooks.Env)
		}
		removeWorktree(root, entry.Path, id)
		// Also delete the branch to prevent stale branch on restart
		delCmd := exec.Command("git", "branch", "-D", entry.Branch)
		delCmd.Dir = root
		delCmd.Run() // best-effort
	}
	wt.entries = make(map[string]worktreeEntry)

	// Clean up the worktree base directory if empty
	baseDir := worktreeBasePath(root)
	if entries, err := os.ReadDir(baseDir); err == nil && len(entries) == 0 {
		os.Remove(baseDir)
	}
}

// gracefulShutdown stops processes and releases resources but preserves worktrees
// and branches so the user can resume on next run.
func (wt *worktreeTracker) gracefulShutdown(root string) {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	for id, entry := range wt.entries {
		// Kill process group if running
		if entry.Pgid != 0 {
			signalProcessGroup(entry.Pgid)
		}
		// Run teardown hooks (release ports, stop dev servers)
		if wt.hooks != nil && len(wt.hooks.Teardown) > 0 {
			_ = runWorktreeHookCommands(wt.hooks.Teardown, entry.Path, entry.Port, wt.hooks.Env)
		}
		// Preserve worktree and branch for resume
		recoverSlug := filepath.Base(entry.Path)
		fmt.Fprintf(os.Stderr, "  Worktree preserved for %s at %s\n", id, entry.Path)
		fmt.Fprintf(os.Stderr, "    Resume with: belmont auto (will prompt to resume)\n")
		fmt.Fprintf(os.Stderr, "    Or clean up: belmont recover --clean %s\n", recoverSlug)
	}
	wt.entries = make(map[string]worktreeEntry)
}

// runWaveParallel runs multiple milestones in parallel using git worktrees.
// singleMilestoneHasExistingWorktree reports whether this milestone already
// has a branch or worktree directory on disk from a prior run. When true,
// runAutoParallel skips its single-milestone master-tree shortcut so resume
// prompts fire and any worktree-local state (STEERING.md, in-progress
// commits) is honoured.
func singleMilestoneHasExistingWorktree(cfg loopConfig, m milestone) bool {
	branch := fmt.Sprintf("belmont/auto/%s/%s", cfg.Feature, strings.ToLower(m.ID))
	wtPath := filepath.Join(worktreeBasePath(cfg.Root), fmt.Sprintf("%s-%s", cfg.Feature, strings.ToLower(m.ID)))
	if dirExists(wtPath) {
		return true
	}
	// `git show-ref` exits 0 if the ref exists.
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = cfg.Root
	return cmd.Run() == nil
}

func runWaveParallel(cfg loopConfig, w wave, tracker *worktreeTracker) error {
	semaphore := make(chan struct{}, cfg.MaxParallel)
	var wg sync.WaitGroup

	type result struct {
		MilestoneID  string
		Branch       string
		WorktreePath string
		Err          error
	}
	results := make(chan result, len(w.Milestones))

	// Resolve stale worktrees sequentially (before parallel launch) to avoid stdin races
	msPreResolved := make(map[string]bool) // milestone ID -> resumed
	for _, m := range w.Milestones {
		branch := fmt.Sprintf("belmont/auto/%s/%s", cfg.Feature, strings.ToLower(m.ID))
		wtPath := filepath.Join(worktreeBasePath(cfg.Root), fmt.Sprintf("%s-%s", cfg.Feature, strings.ToLower(m.ID)))
		resumed, err := handleStaleWorktree(cfg.Root, m.ID, branch, wtPath)
		if err != nil {
			return err
		}
		msPreResolved[m.ID] = resumed
	}

	for _, m := range w.Milestones {
		wg.Add(1)
		go func(ms milestone, resumed bool) {
			defer wg.Done()
			semaphore <- struct{}{}        // acquire
			defer func() { <-semaphore }() // release

			branch := fmt.Sprintf("belmont/auto/%s/%s", cfg.Feature, strings.ToLower(ms.ID))
			wtPath := filepath.Join(worktreeBasePath(cfg.Root), fmt.Sprintf("%s-%s", cfg.Feature, strings.ToLower(ms.ID)))

			tracker.add(ms.ID, wtPath, branch)

			fmt.Fprintf(os.Stderr, "  \033[36m▶ %s: %s\033[0m (worktree)\n", ms.ID, ms.Name)

			err := runMilestoneInWorktree(cfg, ms, branch, wtPath, tracker, resumed)
			results <- result{
				MilestoneID:  ms.ID,
				Branch:       branch,
				WorktreePath: wtPath,
				Err:          err,
			}
		}(m, msPreResolved[m.ID])
	}

	// Wait for all goroutines then close results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var successes []result
	var failures []result
	for r := range results {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "  \033[31m✗ %s failed: %s\033[0m\n", r.MilestoneID, r.Err)
			failures = append(failures, r)
		} else {
			fmt.Fprintf(os.Stderr, "  \033[32m✓ %s complete\033[0m\n", r.MilestoneID)
			successes = append(successes, r)
		}
	}

	// Merge successful branches in milestone ID order
	sort.Slice(successes, func(i, j int) bool {
		return parseMilestoneNum(successes[i].MilestoneID) < parseMilestoneNum(successes[j].MilestoneID)
	})

	for i, s := range successes {
		// Ensure repo is in a clean merge state before each merge
		if err := ensureCleanMergeState(cfg.Root); err != nil {
			fmt.Fprintf(os.Stderr, "  \033[33m⚠ %s — skipping remaining %d merge(s)\033[0m\n", err, len(successes)-i)
			for _, remaining := range successes[i:] {
				failures = append(failures, result{MilestoneID: remaining.MilestoneID, WorktreePath: remaining.WorktreePath, Err: fmt.Errorf("skipped: unclean merge state")})
			}
			break
		}
		if err := mergeWorktreeBranch(cfg, s.MilestoneID, s.Branch, s.WorktreePath, tracker); err != nil {
			return fmt.Errorf("auto: merge failed for %s: %w", s.MilestoneID, err)
		}
	}

	// Clean up failed worktrees (preserve for manual intervention)
	if len(failures) > 0 {
		fmt.Fprintf(os.Stderr, "\n\033[33m⚠ %d milestone(s) failed in wave %d:\033[0m\n", len(failures), w.Index+1)
		for _, f := range failures {
			fmt.Fprintf(os.Stderr, "  %s: worktree preserved at %s\n", f.MilestoneID, f.WorktreePath)
			fmt.Fprintf(os.Stderr, "    Resume: cd %s && belmont auto --feature %s --from %s --to %s\n", f.WorktreePath, cfg.Feature, f.MilestoneID, f.MilestoneID)
		}
		return fmt.Errorf("auto: wave %d had %d failure(s)", w.Index+1, len(failures))
	}

	return nil
}

// runMilestoneInWorktree creates a worktree, installs belmont, copies state, and runs the loop.
func runMilestoneInWorktree(cfg loopConfig, ms milestone, branch, wtPath string, tracker *worktreeTracker, resumed bool) error {
	if !resumed {
		// Create worktree directory
		wtDir := filepath.Dir(wtPath)
		if err := os.MkdirAll(wtDir, 0755); err != nil {
			return fmt.Errorf("create worktree dir: %w", err)
		}

		// Create git worktree
		cmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath, "HEAD")
		cmd.Dir = cfg.Root
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git worktree add: %w (%s)", err, strings.TrimSpace(string(out)))
		}
	}

	// Copy .belmont state into worktree (isolated copy, not symlink)
	if err := copyBelmontStateToWorktree(cfg.Root, wtPath, cfg.Feature); err != nil {
		return fmt.Errorf("copy .belmont state to worktree: %w", err)
	}

	// Commit the initial feature state so the AI agent starts from a clean git state
	commitWorktreeFeatureState(wtPath, cfg.Feature)

	// Copy .env files (gitignored, so not present in fresh worktrees)
	copyEnvFiles(cfg.Root, wtPath)

	// Run belmont install in the worktree (shell out to self)
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}
	installCmd := exec.Command(exePath, "install", "--project", wtPath, "--no-prompt")
	installCmd.Dir = wtPath
	if out, err := installCmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "    \033[33mInstall warning for %s: %s\033[0m\n", ms.ID, strings.TrimSpace(string(out)))
	}

	// Allocate a port for this worktree
	port, err := allocatePort()
	if err != nil {
		fmt.Fprintf(os.Stderr, "    \033[33m⚠ Failed to allocate port for %s: %s\033[0m\n", ms.ID, err)
	} else {
		fmt.Fprintf(os.Stderr, "    Port %d assigned to %s\n", port, ms.ID)
	}
	if tracker != nil {
		tracker.setPort(ms.ID, port)
	}

	// Run worktree setup hooks
	hooks := loadWorktreeHooks(cfg.Root)
	if hooks != nil && len(hooks.Setup) > 0 {
		fmt.Fprintf(os.Stderr, "    Running worktree setup hooks for %s...\n", ms.ID)
		if err := runWorktreeHookCommands(hooks.Setup, wtPath, port, hooks.Env); err != nil {
			return fmt.Errorf("worktree setup for %s: %w", ms.ID, err)
		}
	} else if hooks == nil {
		// No worktree.json — auto-detect dependency install from lock files
		if cmds := detectAutoInstallCommands(cfg.Root); len(cmds) > 0 {
			fmt.Fprintf(os.Stderr, "    Auto-installing dependencies for %s (%s)...\n", ms.ID, strings.Join(cmds, ", "))
			if err := runWorktreeHookCommands(cmds, wtPath, port, nil); err != nil {
				fmt.Fprintf(os.Stderr, "    \033[33m⚠ Auto-install failed for %s: %s (continuing)\033[0m\n", ms.ID, err)
			}
		}
	}

	// Run loop for this single milestone
	mCfg := cfg
	mCfg.Root = wtPath
	mCfg.From = ms.ID
	mCfg.To = ms.ID
	mCfg.Port = port
	mCfg.Tracker = tracker
	mCfg.TrackerID = ms.ID
	if hooks != nil {
		mCfg.WorktreeEnv = hooks.Env
	}

	// Load per-feature model tiers from the worktree's copy of models.yaml.
	if t, err := parseModelTiers(filepath.Join(wtPath, ".belmont", "features", cfg.Feature, "models.yaml")); err == nil {
		mCfg.ModelTiers = t
	}

	return runLoop(mCfg)
}

// mergeWorktreeBranch merges a milestone branch back and cleans up the worktree.
func mergeWorktreeBranch(cfg loopConfig, milestoneID, branch, wtPath string, tracker *worktreeTracker) error {
	// Find the milestone name for the commit message
	var msName string
	progressPath := filepath.Join(cfg.Root, ".belmont", "features", cfg.Feature, "PROGRESS.md")
	if data, err := os.ReadFile(progressPath); err == nil {
		for _, m := range parseMilestones(string(data)) {
			if m.ID == milestoneID {
				msName = m.Name
				break
			}
		}
	}

	// Commit any uncommitted changes in the worktree before merging
	if err := commitWorktreeChanges(wtPath, milestoneID); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Failed to commit worktree changes for %s: %s\033[0m\n", milestoneID, err)
	}

	commitMsg := fmt.Sprintf("belmont: merge %s (%s)", milestoneID, msName)

	if err := attemptMerge(cfg, commitMsg, branch, milestoneID); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[31m✗ Merge failed for %s\033[0m\n", milestoneID)
		fmt.Fprintf(os.Stderr, "    Worktree preserved at: %s\n", wtPath)
		fmt.Fprintf(os.Stderr, "    Branch: %s\n", branch)
		fmt.Fprintf(os.Stderr, "    Resolve manually: git merge --no-ff %s\n", branch)
		fmt.Fprintf(os.Stderr, "    Or use: belmont recover --merge %s\n", filepath.Base(wtPath))
		return err
	}

	// Copy the feature's updated state from the worktree back to the main repo
	syncFeatureStateAfterMerge(cfg.Root, wtPath, cfg.Feature)

	// Clean up reconciliation report if it exists
	os.Remove(filepath.Join(cfg.Root, ".belmont", "reconciliation-report.json"))

	// Run teardown hooks and clean up worktree
	tracker.teardownEntry(milestoneID)
	removeWorktree(cfg.Root, wtPath, milestoneID)
	tracker.remove(milestoneID)

	// Delete the branch
	delCmd := exec.Command("git", "branch", "-d", branch)
	delCmd.Dir = cfg.Root
	delCmd.Run() // best-effort

	return nil
}

// runReconciliationAgent orchestrates two-pass merge conflict resolution.
// Pass 1: AI analyzes conflicts and writes a structured report.
// Pass 2: Go auto-applies high-confidence resolutions and prompts for low-confidence ones.
// Falls back to legacy full-resolve if the report is invalid.
func runReconciliationAgent(cfg loopConfig, milestoneID, branch string) error {
	// Get list of conflicted files
	conflictCmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	conflictCmd.Dir = cfg.Root
	conflictOut, err := conflictCmd.Output()
	if err != nil {
		return fmt.Errorf("list conflicts: %w", err)
	}

	conflictedFiles := strings.TrimSpace(string(conflictOut))
	if conflictedFiles == "" {
		return fmt.Errorf("no conflicted files found")
	}

	reportPath := filepath.Join(cfg.Root, ".belmont", "reconciliation-report.json")

	// Pass 1: AI analysis — writes structured report to disk
	if err := runReconciliationAnalysis(cfg, milestoneID, branch, conflictedFiles, reportPath); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Analysis failed, falling back to legacy resolve...\033[0m\n")
		return runLegacyReconciliation(cfg, milestoneID, branch, conflictedFiles)
	}

	// Read and parse the report
	report, err := parseReconciliationReport(reportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Invalid report (%v), falling back to legacy resolve...\033[0m\n", err)
		os.Remove(reportPath)
		return runLegacyReconciliation(cfg, milestoneID, branch, conflictedFiles)
	}

	// Pass 2: Apply resolutions
	if err := applyReconciliationReport(cfg, report); err != nil {
		os.Remove(reportPath)
		return err
	}

	// Verify no conflict markers remain
	var resolvedFiles []string
	for _, f := range report.Files {
		resolvedFiles = append(resolvedFiles, f.File)
	}
	if err := verifyNoConflictMarkers(cfg.Root, resolvedFiles); err != nil {
		os.Remove(reportPath)
		return err
	}

	// Clean up report file
	os.Remove(reportPath)
	return nil
}

// runReconciliationAnalysis invokes the AI to analyze conflicts and write a JSON report.
func runReconciliationAnalysis(cfg loopConfig, milestoneID, branch, conflictedFiles, reportPath string) error {
	prompt := fmt.Sprintf(`You are a merge conflict analysis agent. Read the reconciliation-agent instructions first, then analyze all merge conflicts.

CRITICAL: Read the file .agents/belmont/reconciliation-agent.md (or agents/belmont/reconciliation-agent.md) for your full instructions and merge strategies by file type. Those instructions are authoritative.

CORE PRINCIPLE: Every merge MUST produce a strictly better state than either side alone. Both branches represent intentional, completed, tested work. You are COMBINING parallel features — never choosing between them. If a resolution would lose ANY code, functionality, dependencies, or tracking state from either side, mark it as "unresolvable" instead of attempting a lossy merge. A blocked merge is ALWAYS preferable to a destructive one.

Conflicted files:
%s

Milestone/Feature: %s
Branch: %s

TASK: For each conflicted file:
1. Read the file to see the conflict markers
2. Understand what each side intended (both sides are valid completed work)
3. Determine the merge strategy based on file type (see agent instructions)
4. Combine BOTH sides — verify nothing is lost
5. Classify your confidence

CONFIDENCE LEVELS:
- "high": Both sides combined with certainty nothing is lost (import unions, additive functions, config entries from different features)
- "low": Both sides combined but semantic interaction possible (same function modified, overlapping config). Operator will review.
- "unresolvable": Cannot combine without losing something. Leave resolved_content empty. Merge will abort.

Write a JSON file to: %s

The JSON must have this exact structure:
{
  "files": [
    {
      "file": "path/to/file",
      "confidence": "high",
      "strategy": "brief strategy label (e.g. import-union, package-manifest-union, additive-functions, lock-regen)",
      "reason": "Why this confidence level",
      "conflict_summary": "Brief: what Side A did vs what Side B did",
      "resolved_content": "The complete resolved file content (no conflict markers)",
      "post_resolve_command": "optional: shell command to run after (e.g. npm install, npx prisma generate)"
    }
  ]
}

RULES:
1. ALWAYS combine both sides — never choose one side over the other. This is non-negotiable.
2. Include all imports from both sides (remove exact duplicates only)
3. Never delete functionality from either side — all completed work must survive
4. For lock files (package-lock.json, yarn.lock, etc.): set resolved_content to empty string and post_resolve_command to the install command
5. For package manifests: take the union of all dependency additions from both sides
6. The resolved_content must be the COMPLETE file with conflicts resolved
7. Do NOT modify any files on disk — only write the JSON report
8. Do NOT run git add — only write the report
9. Include ALL conflicted files in the report
10. When in doubt, mark "unresolvable" — blocking is safer than losing work`, conflictedFiles, milestoneID, branch, reportPath)

	// Reconciliation needs strong reasoning — use configured tier (defaults to high).
	flags := resolveModelFlags(cfg.Tool, reconciliationTier(cfg.ModelTiers))
	cmd := buildToolCommand(cfg.Tool, prompt, cfg.Root, flags...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// parseReconciliationReport reads and validates the JSON report file.
func parseReconciliationReport(reportPath string) (reconciliationReport, error) {
	var report reconciliationReport

	data, err := os.ReadFile(reportPath)
	if err != nil {
		return report, fmt.Errorf("read report: %w", err)
	}

	if err := json.Unmarshal(data, &report); err != nil {
		return report, fmt.Errorf("parse report: %w", err)
	}

	if len(report.Files) == 0 {
		return report, fmt.Errorf("report contains no files")
	}

	// Validate each file entry
	for i, f := range report.Files {
		if f.File == "" {
			return report, fmt.Errorf("file entry %d missing file path", i)
		}
		if f.Confidence != "high" && f.Confidence != "low" && f.Confidence != "unresolvable" {
			return report, fmt.Errorf("file %q has invalid confidence %q", f.File, f.Confidence)
		}
		if f.ResolvedContent == "" && f.Confidence != "unresolvable" && f.PostResolveCmd == "" {
			return report, fmt.Errorf("file %q has empty resolved_content (mark as unresolvable if it cannot be resolved)", f.File)
		}
	}

	return report, nil
}

// applyReconciliationReport applies resolved content from the report.
// High-confidence files are auto-applied. Low-confidence files are shown
// to the user interactively (if terminal) or auto-applied (if non-interactive).
// If ANY file is unresolvable, the entire merge is aborted.
func applyReconciliationReport(cfg loopConfig, report reconciliationReport) error {
	interactive := isTerminal(os.Stdin)
	autoAll := false

	// Check for unresolvable files first — abort before applying anything
	var unresolvable []reconciliationFile
	var highCount, lowCount int
	for _, f := range report.Files {
		switch f.Confidence {
		case "unresolvable":
			unresolvable = append(unresolvable, f)
		case "high":
			highCount++
		default:
			lowCount++
		}
	}

	if len(unresolvable) > 0 {
		fmt.Fprintf(os.Stderr, "  \033[31m✗ %d file(s) marked unresolvable — aborting merge:\033[0m\n", len(unresolvable))
		for _, f := range unresolvable {
			fmt.Fprintf(os.Stderr, "    %s: %s\n", f.File, f.Reason)
		}
		return fmt.Errorf("unresolvable conflicts in %d file(s)", len(unresolvable))
	}

	if highCount > 0 {
		fmt.Fprintf(os.Stderr, "  \033[32m✓ Auto-applying %d high-confidence resolution(s)\033[0m\n", highCount)
	}
	if lowCount > 0 && interactive {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ %d file(s) need review\033[0m\n", lowCount)
	}

	// Collect post-resolve commands (deduped, ordered)
	var postCmds []string
	seenCmds := make(map[string]bool)

	for _, f := range report.Files {
		filePath := filepath.Join(cfg.Root, f.File)

		// Track post-resolve commands
		if f.PostResolveCmd != "" && !seenCmds[f.PostResolveCmd] {
			postCmds = append(postCmds, f.PostResolveCmd)
			seenCmds[f.PostResolveCmd] = true
		}

		// Skip writing files that will be regenerated by post-resolve commands
		// (e.g., lock files with empty resolved_content)
		if f.ResolvedContent == "" && f.PostResolveCmd != "" {
			// Delete the conflicted file so the post-resolve command regenerates it
			os.Remove(filePath)
			addCmd := exec.Command("git", "add", f.File)
			addCmd.Dir = cfg.Root
			addCmd.Run()
			continue
		}

		if f.Confidence == "high" || !interactive || autoAll {
			// Auto-apply
			if err := writeReconciliationResolution(filePath, f.ResolvedContent); err != nil {
				return fmt.Errorf("write %s: %w", f.File, err)
			}
			addCmd := exec.Command("git", "add", f.File)
			addCmd.Dir = cfg.Root
			if err := addCmd.Run(); err != nil {
				return fmt.Errorf("git add %s: %w", f.File, err)
			}
			continue
		}

		// Low confidence + interactive: prompt user
		choice, err := reviewConflict(cfg.Root, f)
		if err != nil {
			return err
		}

		switch choice {
		case "auto":
			autoAll = true
			// Apply this file and all remaining
			if err := writeReconciliationResolution(filePath, f.ResolvedContent); err != nil {
				return fmt.Errorf("write %s: %w", f.File, err)
			}
			addCmd := exec.Command("git", "add", f.File)
			addCmd.Dir = cfg.Root
			if err := addCmd.Run(); err != nil {
				return fmt.Errorf("git add %s: %w", f.File, err)
			}
		case "accept", "edited":
			// File already written by reviewConflict for "edited", write for "accept"
			if choice == "accept" {
				if err := writeReconciliationResolution(filePath, f.ResolvedContent); err != nil {
					return fmt.Errorf("write %s: %w", f.File, err)
				}
			}
			addCmd := exec.Command("git", "add", f.File)
			addCmd.Dir = cfg.Root
			if err := addCmd.Run(); err != nil {
				return fmt.Errorf("git add %s: %w", f.File, err)
			}
		}
	}

	// Run post-resolve commands (e.g., npm install to regen lock files)
	if len(postCmds) > 0 {
		for _, cmd := range postCmds {
			fmt.Fprintf(os.Stderr, "  \033[2mRunning post-resolve: %s\033[0m\n", cmd)
			parts := strings.Fields(cmd)
			postCmd := exec.Command(parts[0], parts[1:]...)
			postCmd.Dir = cfg.Root
			if out, err := postCmd.CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "  \033[33m⚠ Post-resolve command failed: %s\n%s\033[0m\n", cmd, strings.TrimSpace(string(out)))
				// Don't fail the merge — the operator can fix this
			}
		}
		// Stage any files generated by post-resolve commands
		addCmd := exec.Command("git", "add", "-A")
		addCmd.Dir = cfg.Root
		addCmd.Run()

		// `git add -A` sweeps up transient reconciliation artifacts too.
		// Unstage the report explicitly so the merge commit doesn't include it;
		// the caller deletes it from disk right after this returns.
		unstage := exec.Command("git", "rm", "--cached", "--ignore-unmatch", "--", ".belmont/reconciliation-report.json")
		unstage.Dir = cfg.Root
		unstage.Run()
	}

	return nil
}

// writeReconciliationResolution writes a resolved conflict file to disk, handling:
//   - Missing parent directories (git conflict handling can leave them unpopulated)
//   - Existing symlinks at the target path (os.WriteFile would follow the symlink
//     and fail if the symlink is broken or points at a directory)
//   - Resolved content that is itself a symlink target (short single-line path) —
//     recreates as a symlink to preserve the original file type
func writeReconciliationResolution(filePath, content string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}

	// If the existing path is a symlink, remove it so we can decide fresh
	// whether to write a regular file or recreate a symlink. Otherwise WriteFile
	// follows the symlink and fails when the target is a dir or a broken path.
	wasSymlink := false
	if info, err := os.Lstat(filePath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		wasSymlink = true
		if err := os.Remove(filePath); err != nil {
			return err
		}
	}

	// If the original was a symlink and the resolved content looks like a
	// single-line path (no embedded newlines, reasonable length), recreate as
	// a symlink rather than writing it as a text file.
	trimmed := strings.TrimSpace(content)
	if wasSymlink && !strings.ContainsAny(trimmed, "\n\x00") && len(trimmed) > 0 && len(trimmed) < 1024 {
		return os.Symlink(trimmed, filePath)
	}

	return os.WriteFile(filePath, []byte(content), 0o644)
}

// reviewConflict prompts the user to review a low-confidence conflict resolution.
func reviewConflict(root string, f reconciliationFile) (string, error) {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Fprintf(os.Stderr, "\n  \033[1;33m⚠ Uncertain conflict in %s\033[0m\n", f.File)
		fmt.Fprintf(os.Stderr, "    %s\n\n", f.Reason)
		fmt.Fprintf(os.Stderr, "    %s\n\n", f.ConflictSummary)
		fmt.Fprintf(os.Stderr, "    [a] Accept AI's resolution  [v] View proposed resolution\n")
		fmt.Fprintf(os.Stderr, "    [e] Edit in $EDITOR         [s] Auto-resolve all remaining  [q] Abort\n\n")
		fmt.Fprintf(os.Stderr, "    Choice [a]: ")

		if !scanner.Scan() {
			return "", fmt.Errorf("reconciliation: no input")
		}
		input := strings.TrimSpace(strings.ToLower(scanner.Text()))

		if input == "" || input == "a" {
			return "accept", nil
		}

		switch input {
		case "v":
			// Show resolved content with line numbers
			fmt.Fprintf(os.Stderr, "\n    \033[2m--- Proposed resolution for %s ---\033[0m\n", f.File)
			lines := strings.Split(f.ResolvedContent, "\n")
			for i, line := range lines {
				fmt.Fprintf(os.Stderr, "    \033[2m%4d\033[0m  %s\n", i+1, line)
			}
			fmt.Fprintf(os.Stderr, "    \033[2m--- End ---\033[0m\n")
			// Re-prompt
			continue

		case "e":
			// Write proposed resolution to file for editing
			filePath := filepath.Join(root, f.File)
			if err := os.WriteFile(filePath, []byte(f.ResolvedContent), 0644); err != nil {
				return "", fmt.Errorf("write for edit %s: %w", f.File, err)
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			editorCmd := exec.Command(editor, filePath)
			editorCmd.Stdin = os.Stdin
			editorCmd.Stdout = os.Stdout
			editorCmd.Stderr = os.Stderr
			if err := editorCmd.Run(); err != nil {
				return "", fmt.Errorf("editor for %s: %w", f.File, err)
			}
			return "edited", nil

		case "s":
			return "auto", nil

		case "q":
			return "", fmt.Errorf("reconciliation: aborted by user")

		default:
			fmt.Fprintf(os.Stderr, "    Invalid choice. Try again.\n")
		}
	}
}

// verifyNoConflictMarkers scans resolved files for leftover conflict markers.
func verifyNoConflictMarkers(root string, files []string) error {
	markers := []string{"<<<<<<<", "=======", ">>>>>>>"}
	var badFiles []string

	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			continue
		}
		content := string(data)
		for _, marker := range markers {
			if strings.Contains(content, marker) {
				badFiles = append(badFiles, file)
				break
			}
		}
	}

	if len(badFiles) > 0 {
		return fmt.Errorf("conflict markers remain in: %s", strings.Join(badFiles, ", "))
	}
	return nil
}

// runLegacyReconciliation is the fallback: AI resolves everything directly on disk.
func runLegacyReconciliation(cfg loopConfig, milestoneID, branch, conflictedFiles string) error {
	prompt := fmt.Sprintf(`You are a merge conflict reconciliation agent. Read your instructions first, then resolve all merge conflicts.

CRITICAL: Read the file .agents/belmont/reconciliation-agent.md (or agents/belmont/reconciliation-agent.md) for full instructions and merge strategies by file type.

CORE PRINCIPLE: Every merge MUST produce a strictly better state than either side alone. Both branches are intentional, completed, tested work. You are COMBINING parallel features — never choosing between them. If resolving a file would lose ANY code, functionality, dependencies, or state from either side, leave it conflicted and report the failure. A blocked merge is ALWAYS preferable to a destructive one.

Conflicted files:
%s

Milestone/Feature: %s
Branch: %s

Rules:
1. ALWAYS combine both sides — never choose one side over the other. This is non-negotiable.
2. Include all imports from both sides (remove exact duplicates only)
3. Never delete functionality from either side — all completed work must survive
4. Only modify conflicted files
5. For lock files (package-lock.json, yarn.lock, etc.): delete the conflicted lock, resolve the manifest, then run the package manager to regenerate
6. For package manifests: take the union of ALL dependency additions from both sides
7. After resolving each file, run "git add <file>"
8. Do NOT commit — the caller handles the commit
9. If you cannot safely resolve a file without losing work, leave it conflicted and report which files could not be resolved

Read each conflicted file, resolve the conflict markers by combining both sides, write the resolved version, and git add it.`, conflictedFiles, milestoneID, branch)

	// Reconciliation needs strong reasoning — use configured tier (defaults to high).
	flags := resolveModelFlags(cfg.Tool, reconciliationTier(cfg.ModelTiers))
	cmd := buildToolCommand(cfg.Tool, prompt, cfg.Root, flags...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// removeWorktree removes a git worktree and its directory.
func removeWorktree(root, wtPath, _ string) {
	cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		// Try manual cleanup
		os.RemoveAll(wtPath)
		pruneCmd := exec.Command("git", "worktree", "prune")
		pruneCmd.Dir = root
		pruneCmd.Run()
	}
}

// validateRepoState checks that the repo is in a clean state suitable for auto mode.
// Returns an error if there's an in-progress merge, rebase, or unmerged files.
func validateRepoState(root string) error {
	// Resolve .git dir (could be a file in a worktree)
	gitDir := filepath.Join(root, ".git")

	// Check no in-progress merge
	if fileExists(filepath.Join(gitDir, "MERGE_HEAD")) {
		return fmt.Errorf("repository has an in-progress merge — resolve with 'git merge --abort' or 'git merge --continue' first")
	}
	// Check no in-progress rebase
	if dirExists(filepath.Join(gitDir, "rebase-merge")) || dirExists(filepath.Join(gitDir, "rebase-apply")) {
		return fmt.Errorf("repository has an in-progress rebase — resolve first")
	}
	// Check no unmerged files
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = root
	out, err := cmd.Output()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("repository has unmerged files — resolve conflicts first:\n%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// getCurrentBranch returns the current branch name, or "HEAD" if detached.
func getCurrentBranch(root string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ensureCleanMergeState aborts any in-progress merge and cleans up unmerged files.
// Called between sequential merges to prevent cascade failures.
func ensureCleanMergeState(root string) error {
	gitDir := filepath.Join(root, ".git")

	// Abort any in-progress merge
	if fileExists(filepath.Join(gitDir, "MERGE_HEAD")) {
		abortCmd := exec.Command("git", "merge", "--abort")
		abortCmd.Dir = root
		abortCmd.Run()
	}

	// Check for remaining unmerged files
	diffCmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	diffCmd.Dir = root
	out, _ := diffCmd.Output()
	if strings.TrimSpace(string(out)) == "" {
		return nil // clean
	}

	// Try harder: reset index and checkout
	resetCmd := exec.Command("git", "reset", "HEAD")
	resetCmd.Dir = root
	resetCmd.Run()
	checkoutCmd := exec.Command("git", "checkout", "--", ".")
	checkoutCmd.Dir = root
	checkoutCmd.Run()

	// Re-check
	recheckCmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	recheckCmd.Dir = root
	out2, _ := recheckCmd.Output()
	if strings.TrimSpace(string(out2)) != "" {
		return fmt.Errorf("unable to clean merge state — unmerged files remain:\n%s", strings.TrimSpace(string(out2)))
	}
	return nil
}

// prepareWorktreesGitignore adds .belmont/worktrees/ and .belmont/auto.json to .gitignore and commits the change.
// This must happen before creating worktrees so they branch from a clean HEAD with the entry.
func prepareWorktreesGitignore(root string) {
	ensureGitignoreEntry(root, ".belmont/worktrees/")
	ensureGitignoreEntry(root, ".belmont/auto.json")

	// Check if .gitignore was modified
	statusCmd := exec.Command("git", "status", "--porcelain", ".gitignore")
	statusCmd.Dir = root
	out, err := statusCmd.Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return // nothing to commit
	}

	// Commit the .gitignore change so worktrees branch from clean HEAD
	addCmd := exec.Command("git", "add", ".gitignore")
	addCmd.Dir = root
	if _, err := addCmd.CombinedOutput(); err != nil {
		return
	}
	commitCmd := exec.Command("git", "commit", "-m", "belmont: add worktrees to gitignore")
	commitCmd.Dir = root
	commitCmd.CombinedOutput() // best-effort
}

// excludeBelmontInWorktree adds .belmont/ to the worktree's local git exclude file.
// Unlike modifying .gitignore (which gets committed and causes merge conflicts),
// the local exclude is never committed and is specific to the worktree.
func excludeBelmontInWorktree(wtPath string) {
	// Read .git file to find worktree git dir
	gitFile := filepath.Join(wtPath, ".git")
	data, err := os.ReadFile(gitFile)
	if err != nil {
		return
	}
	// Parse "gitdir: /path/to/.git/worktrees/{name}"
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir: ") {
		return
	}
	gitDir := strings.TrimPrefix(line, "gitdir: ")
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(wtPath, gitDir)
	}

	// Write to info/exclude
	infoDir := filepath.Join(gitDir, "info")
	os.MkdirAll(infoDir, 0755)
	excludePath := filepath.Join(infoDir, "exclude")
	existing, _ := os.ReadFile(excludePath)
	if strings.Contains(string(existing), ".belmont/") {
		return // already present
	}
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		f.WriteString("\n")
	}
	f.WriteString(".belmont/\n")
}

// copyBelmontStateToWorktree copies feature state and read-only context into a worktree.
// The feature's own state (features/<slug>/) is copied as writable — the agent commits these.
// Master context files (PRD.md, PROGRESS.md, etc.) are copied for reference but excluded from git.
// A .worktree marker file is written so belmont sync can detect worktree context and no-op.
func copyBelmontStateToWorktree(root, wtPath, slug string) error {
	srcBelmont := filepath.Join(root, ".belmont")
	dstBelmont := filepath.Join(wtPath, ".belmont")

	// IMPORTANT: Do NOT remove the existing .belmont/ directory. The worktree
	// inherits the full .belmont/ from HEAD, including all features' state.
	// Removing it would cause git to see those deletions, and merging the
	// worktree branch back would delete other features' state from the main branch.
	//
	// Instead, we overlay the current feature's latest state on top and use git
	// excludes to prevent the worktree from committing changes to other features.
	if err := os.MkdirAll(dstBelmont, 0755); err != nil {
		return fmt.Errorf("create .belmont dir in worktree: %w", err)
	}

	// 1. Copy feature's own state (writable — agent commits these)
	// This overlays the latest state on top of whatever was checked out from HEAD
	srcFeature := filepath.Join(srcBelmont, "features", slug)
	dstFeature := filepath.Join(dstBelmont, "features", slug)
	if dirExists(srcFeature) {
		// Preserve worktree-local STEERING.md / STEERING.log.md (written by
		// `belmont steer` and by consumption) across the wipe-and-recopy.
		// Master never holds these, so without the preserve they would be
		// silently clobbered when auto resumes a preserved worktree.
		var steeringData []byte
		steeringPath := filepath.Join(dstFeature, "STEERING.md")
		if data, err := os.ReadFile(steeringPath); err == nil {
			steeringData = data
		}
		// Remove just this feature's dir to get a clean copy
		os.RemoveAll(dstFeature)
		if err := copyDir(srcFeature, dstFeature); err != nil {
			return fmt.Errorf("copy feature state: %w", err)
		}
		if steeringData != nil {
			if err := os.WriteFile(filepath.Join(dstFeature, "STEERING.md"), steeringData, 0644); err != nil {
				return fmt.Errorf("restore STEERING.md: %w", err)
			}
		}
	}

	// 2. Copy read-only context files (master PRD, PROGRESS, etc.)
	contextFiles := []string{"PRD.md", "PROGRESS.md", "PR_FAQ.md", "TECH_PLAN.md", "worktree.json"}
	for _, f := range contextFiles {
		src := filepath.Join(srcBelmont, f)
		if fileExists(src) {
			copyFile(src, filepath.Join(dstBelmont, f)) // best-effort
		}
	}

	// 3. Copy prompts/ if present (needed for AI decision templates)
	promptsSrc := filepath.Join(srcBelmont, "prompts")
	if dirExists(promptsSrc) {
		copyDir(promptsSrc, filepath.Join(dstBelmont, "prompts")) // best-effort
	}

	// 4. Write .worktree marker file (used by sync to detect worktree context)
	markerPath := filepath.Join(dstBelmont, ".worktree")
	os.WriteFile(markerPath, []byte(slug+"\n"), 0644)

	// 5. Write git excludes — exclude all .belmont/ except this feature's state.
	// This prevents the worktree from accidentally committing changes to (or
	// deletions of) other features' state, read-only context files, etc.
	writeWorktreeGitExcludes(wtPath)

	// 6. Mark all .belmont/ files as assume-unchanged so git ignores them entirely.
	// This prevents the worktree from committing .belmont/ changes (which would
	// delete other features' state when merged back). The files remain on disk
	// so the AI agent can read cross-feature state.
	untrackBelmontInWorktree(wtPath, slug)

	return nil
}

// writeWorktreeGitExcludes adds .belmont/ to the worktree's .git/info/exclude.
// This prevents new .belmont/ files from being tracked. Combined with
// assume-unchanged on existing files, it fully isolates .belmont/ from git.
func writeWorktreeGitExcludes(wtPath string) {
	gitFile := filepath.Join(wtPath, ".git")
	data, err := os.ReadFile(gitFile)
	if err != nil {
		return
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir: ") {
		return
	}
	gitDir := strings.TrimPrefix(line, "gitdir: ")
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(wtPath, gitDir)
	}

	infoDir := filepath.Join(gitDir, "info")
	os.MkdirAll(infoDir, 0755)
	excludePath := filepath.Join(infoDir, "exclude")

	// Exclude all .belmont/ from git in this worktree. Combined with
	// assume-unchanged on already-tracked files, this ensures no .belmont/
	// state leaks into commits or merges. State is synced by the orchestrator.
	excludeContent := "# belmont worktree excludes — all .belmont/ state managed by orchestrator\n" +
		".belmont/\n"

	existing, _ := os.ReadFile(excludePath)
	// If we've already written our excludes, skip
	if strings.Contains(string(existing), "belmont worktree excludes") {
		return
	}

	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		f.WriteString("\n")
	}
	f.WriteString(excludeContent)
}

// untrackBelmontInWorktree marks all .belmont/ files as assume-unchanged in the
// worktree's git index. This prevents git from detecting modifications or deletions
// of .belmont/ files, so they won't be included in commits or merges. The files
// remain on disk for the AI agent to read (cross-feature visibility).
func untrackBelmontInWorktree(wtPath, slug string) {
	// Get list of all .belmont/ files currently in git's index
	lsCmd := exec.Command("git", "ls-files", ".belmont/")
	lsCmd.Dir = wtPath
	out, err := lsCmd.Output()
	if err != nil {
		return
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// Mark every .belmont/ file as assume-unchanged so git ignores
		// any modifications or deletions in this worktree
		cmd := exec.Command("git", "update-index", "--assume-unchanged", line)
		cmd.Dir = wtPath
		cmd.Run() // best-effort
	}
}

// commitWorktreeFeatureState commits the initial .belmont/features/ state in a worktree
// so the AI agent starts from a clean git state.
func commitWorktreeFeatureState(wtPath, slug string) {
	// .belmont/ is marked assume-unchanged to prevent worktree merges from
	// deleting other features' state. No .belmont/ commit needed here —
	// the orchestrator copies feature state back after merge.
}

// ensureGitignoreEntry adds an entry to .gitignore if not already present.
func ensureGitignoreEntry(root, entry string) {
	gitignorePath := filepath.Join(root, ".gitignore")

	content, err := os.ReadFile(gitignorePath)
	if err == nil {
		if strings.Contains(string(content), entry) {
			return // already present
		}
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	// Add newline before if file doesn't end with one
	if len(content) > 0 && content[len(content)-1] != '\n' {
		f.WriteString("\n")
	}
	f.WriteString(entry + "\n")
}

// syncFeatureStateAfterMerge copies the feature's .belmont/ state from a worktree
// back to the main repo after a successful merge. Since .belmont/ is excluded from
// git tracking in worktrees (assume-unchanged), state must be synced separately.
func syncFeatureStateAfterMerge(mainRoot, wtPath, slug string) {
	srcFeature := filepath.Join(wtPath, ".belmont", "features", slug)
	dstFeature := filepath.Join(mainRoot, ".belmont", "features", slug)

	if !dirExists(srcFeature) {
		return
	}

	// Replace the main repo's feature state with the worktree's version
	os.RemoveAll(dstFeature)
	if err := copyDir(srcFeature, dstFeature); err != nil {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Failed to sync feature state for %s: %s\033[0m\n", slug, err)
	}
}

// commitWorktreeChanges commits all uncommitted changes in a worktree before merge.
// AI agents may leave uncommitted work (code changes, state files) when the loop
// completes. Without this, git merge --no-ff only sees committed changes and the
// worktree's working directory changes are silently lost.
func commitWorktreeChanges(wtPath, label string) error {
	// Check for any uncommitted changes (tracked + untracked)
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = wtPath
	out, err := statusCmd.Output()
	if err != nil {
		return nil // can't check, skip gracefully
	}
	if strings.TrimSpace(string(out)) == "" {
		return nil // nothing to commit
	}

	// Stage everything
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = wtPath
	if _, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add -A in worktree: %w", err)
	}

	// Commit
	commitCmd := exec.Command("git", "commit", "-m", fmt.Sprintf("belmont: finalize %s", label))
	commitCmd.Dir = wtPath
	if _, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit in worktree: %w", err)
	}

	fmt.Fprintf(os.Stderr, "  \033[2m(committed uncommitted changes for %s)\033[0m\n", label)
	return nil
}

// commitBelmontState commits any uncommitted .belmont/ state files in the main repo.
// Used after belmont sync updates master PROGRESS.md.
func commitBelmontState(root string) error {
	// Don't try to commit if there's an in-progress merge — git commit would
	// either fail or finalize the merge unintentionally
	if fileExists(filepath.Join(root, ".git", "MERGE_HEAD")) {
		return fmt.Errorf("skipping: merge in progress")
	}

	statusCmd := exec.Command("git", "status", "--porcelain", ".belmont/")
	statusCmd.Dir = root
	out, err := statusCmd.Output()
	if err != nil {
		return nil // can't check, skip gracefully
	}
	if strings.TrimSpace(string(out)) == "" {
		return nil // nothing to commit
	}

	addCmd := exec.Command("git", "add", ".belmont/")
	addCmd.Dir = root
	if _, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add .belmont/: %w", err)
	}

	commitCmd := exec.Command("git", "commit", "-m", "belmont: update state files")
	commitCmd.Dir = root
	if _, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit .belmont/: %w", err)
	}
	return nil
}

// mergeFailureKind classifies the type of git merge failure.
type mergeFailureKind int

const (
	mergeConflict            mergeFailureKind = iota // file-level conflicts
	mergeUntrackedOverwrite                          // untracked files would be overwritten
	mergeDirtyWorktree                               // local changes would be overwritten
	mergeUnmergedFiles                               // stale unmerged files from previous merge
	mergeOtherFailure                                // unknown merge failure
)

// classifyMergeError determines what kind of merge failure occurred from git output.
func classifyMergeError(output string) mergeFailureKind {
	if strings.Contains(output, "untracked working tree files would be overwritten") {
		return mergeUntrackedOverwrite
	}
	if strings.Contains(output, "Your local changes to the following files would be overwritten") {
		return mergeDirtyWorktree
	}
	if strings.Contains(output, "CONFLICT") || strings.Contains(output, "Automatic merge failed") {
		return mergeConflict
	}
	if strings.Contains(output, "unmerged files") {
		return mergeUnmergedFiles
	}
	return mergeOtherFailure
}

// parseOverwrittenFiles extracts file paths from git's "untracked working tree files would be overwritten" error.
func parseOverwrittenFiles(output string) []string {
	var files []string
	lines := strings.Split(output, "\n")
	inFileList := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(line, "untracked working tree files would be overwritten") {
			inFileList = true
			continue
		}
		if inFileList {
			if trimmed == "" || strings.HasPrefix(trimmed, "Please move or remove") || strings.HasPrefix(trimmed, "Aborting") {
				break
			}
			if trimmed != "" {
				files = append(files, trimmed)
			}
		}
	}
	return files
}

// attemptMerge tries to merge a branch, handling various failure modes automatically.
// It handles untracked file overwrites, dirty worktrees, and merge conflicts.
func attemptMerge(cfg loopConfig, commitMsg, branch, id string) error {
	// Try the merge
	cmd := exec.Command("git", "merge", "--no-ff", branch, "-m", commitMsg)
	cmd.Dir = cfg.Root
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil // merge succeeded
	}

	output := string(out)
	kind := classifyMergeError(output)

	switch kind {
	case mergeUntrackedOverwrite:
		// Temporarily stash untracked files that conflict
		files := parseOverwrittenFiles(output)
		if len(files) == 0 {
			return fmt.Errorf("merge failed (untracked overwrite) but could not parse file list: %s", output)
		}

		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Untracked files would be overwritten for %s — auto-stashing %d files...\033[0m\n", id, len(files))

		stashDir := filepath.Join(cfg.Root, ".belmont", "merge-stash")
		if err := os.MkdirAll(stashDir, 0755); err != nil {
			return fmt.Errorf("create merge-stash dir: %w", err)
		}

		// Move conflicting files to stash
		for _, f := range files {
			src := filepath.Join(cfg.Root, f)
			dst := filepath.Join(stashDir, f)
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				continue
			}
			os.Rename(src, dst)
		}

		// Retry the merge
		retryCmd := exec.Command("git", "merge", "--no-ff", branch, "-m", commitMsg)
		retryCmd.Dir = cfg.Root
		retryOut, retryErr := retryCmd.CombinedOutput()

		if retryErr == nil {
			// Merge succeeded — clean up stash
			os.RemoveAll(stashDir)
			fmt.Fprintf(os.Stderr, "  \033[32m✓ Merge succeeded after stashing untracked files for %s\033[0m\n", id)
			return nil
		}

		// Retry failed — restore stashed files and fall through
		for _, f := range files {
			src := filepath.Join(stashDir, f)
			dst := filepath.Join(cfg.Root, f)
			os.MkdirAll(filepath.Dir(dst), 0755)
			os.Rename(src, dst)
		}
		os.RemoveAll(stashDir)

		// Classify the retry failure
		retryOutput := string(retryOut)
		retryKind := classifyMergeError(retryOutput)
		if retryKind == mergeConflict {
			// Fall through to conflict handling below
			goto handleConflict
		}
		return fmt.Errorf("merge failed for %s after stashing untracked files: %s", id, retryOutput)

	case mergeDirtyWorktree:
		// Stash local changes and retry merge
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Local changes would be overwritten for %s — stashing...\033[0m\n", id)

		stashCmd := exec.Command("git", "stash", "push", "--include-untracked", "-m", "belmont: pre-merge stash")
		stashCmd.Dir = cfg.Root
		if _, stashErr := stashCmd.CombinedOutput(); stashErr != nil {
			return fmt.Errorf("git stash failed for %s: %w", id, stashErr)
		}

		retryCmd2 := exec.Command("git", "merge", "--no-ff", branch, "-m", commitMsg)
		retryCmd2.Dir = cfg.Root
		retryOut2, retryErr2 := retryCmd2.CombinedOutput()

		// Pop the stash — handle failure to avoid orphaned stash entries
		popCmd := exec.Command("git", "stash", "pop")
		popCmd.Dir = cfg.Root
		if popOut, popErr := popCmd.CombinedOutput(); popErr != nil {
			fmt.Fprintf(os.Stderr, "  \033[33m⚠ stash pop had conflicts for %s — your local changes are preserved in 'git stash list', resolve with 'git stash pop': %s\033[0m\n", id, strings.TrimSpace(string(popOut)))
		}

		if retryErr2 == nil {
			fmt.Fprintf(os.Stderr, "  \033[32m✓ Merge succeeded after stashing local changes for %s\033[0m\n", id)
			return nil
		}

		retryOutput2 := string(retryOut2)
		retryKind2 := classifyMergeError(retryOutput2)
		if retryKind2 == mergeConflict {
			goto handleConflict
		}
		return fmt.Errorf("merge failed for %s after stashing local changes: %s", id, retryOutput2)

	case mergeConflict:
		goto handleConflict

	case mergeUnmergedFiles:
		// Stale merge state from a previous operation — abort and retry once
		fmt.Fprintf(os.Stderr, "  \033[33m⚠ Stale unmerged files for %s — aborting previous merge and retrying...\033[0m\n", id)
		abortCmd := exec.Command("git", "merge", "--abort")
		abortCmd.Dir = cfg.Root
		abortCmd.Run()
		retryCmd := exec.Command("git", "merge", "--no-ff", branch, "-m", commitMsg)
		retryCmd.Dir = cfg.Root
		retryOut, retryErr := retryCmd.CombinedOutput()
		if retryErr == nil {
			fmt.Fprintf(os.Stderr, "  \033[32m✓ Merge succeeded after aborting stale merge for %s\033[0m\n", id)
			return nil
		}
		retryKind := classifyMergeError(string(retryOut))
		if retryKind == mergeConflict {
			goto handleConflict
		}
		return fmt.Errorf("merge failed for %s after aborting stale merge: %s", id, string(retryOut))

	default:
		return fmt.Errorf("merge failed for %s: %s", id, output)
	}

handleConflict:
	// Try auto-resolving .belmont/ conflicts first (common with parallel milestones)
	autoResolveBelmontConflicts(cfg.Root)

	// Try auto-resolving lock files (delete + regenerate via package manager)
	autoResolveLockFiles(cfg.Root)

	// Check if all conflicts are now resolved
	{
		checkCmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
		checkCmd.Dir = cfg.Root
		if checkOut, err := checkCmd.Output(); err == nil && strings.TrimSpace(string(checkOut)) == "" {
			// All conflicts resolved — commit the merge
			commitCmd2 := exec.Command("git", "commit", "--no-edit")
			commitCmd2.Dir = cfg.Root
			if _, err := commitCmd2.CombinedOutput(); err == nil {
				fmt.Fprintf(os.Stderr, "  \033[32m✓ Merge conflicts auto-resolved for %s\033[0m\n", id)
				return nil
			}
		}
	}

	fmt.Fprintf(os.Stderr, "  \033[33m⚠ Merge conflict for %s — invoking reconciliation agent...\033[0m\n", id)

	reconcileErr := runReconciliationAgent(cfg, id, branch)
	if reconcileErr != nil {
		abortCmd := exec.Command("git", "merge", "--abort")
		abortCmd.Dir = cfg.Root
		abortCmd.Run()
		return fmt.Errorf("merge conflict resolution failed for %s: %w", id, reconcileErr)
	}

	// Reconciliation succeeded — commit the merge
	commitCmd := exec.Command("git", "commit", "-m", commitMsg)
	commitCmd.Dir = cfg.Root
	if _, commitErr := commitCmd.CombinedOutput(); commitErr != nil {
		// Abort the merge to prevent stale MERGE_HEAD from poisoning subsequent operations
		abortCmd2 := exec.Command("git", "merge", "--abort")
		abortCmd2.Dir = cfg.Root
		abortCmd2.Run()
		return fmt.Errorf("commit after reconciliation for %s: %w", id, commitErr)
	}

	fmt.Fprintf(os.Stderr, "  \033[32m✓ Reconciliation resolved merge conflict for %s\033[0m\n", id)
	return nil
}

// autoResolveBelmontConflicts attempts to auto-resolve merge conflicts on .belmont/ files.
// For PROGRESS.md files, it takes the union of milestone completions (each milestone marks
// only its own [x] status, so union is safe). Returns true if any conflicts were resolved.
func autoResolveBelmontConflicts(root string) bool {
	// Get list of conflicted files
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	resolved := false
	for _, file := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		if !strings.HasPrefix(file, ".belmont/") {
			continue
		}

		filePath := filepath.Join(root, file)

		if strings.HasSuffix(file, "PROGRESS.md") {
			// For PROGRESS.md: get both sides and merge milestone completions
			if resolveProgressConflict(root, file, filePath) {
				resolved = true
			}
		} else {
			// For other .belmont/ files (e.g., MILESTONE.md): take "theirs" (the branch being merged)
			checkoutCmd := exec.Command("git", "checkout", "--theirs", "--", file)
			checkoutCmd.Dir = root
			if _, err := checkoutCmd.CombinedOutput(); err == nil {
				addCmd := exec.Command("git", "add", file)
				addCmd.Dir = root
				addCmd.Run()
				resolved = true
			}
		}
	}
	return resolved
}

// resolveProgressConflict merges a conflicted PROGRESS.md by taking the most-advanced
// task state from both sides. State ordering: [v] > [x] > [>] > [ ], [!] preserved.
// Returns true if successfully resolved.
func resolveProgressConflict(root, relPath, filePath string) bool {
	// Get "ours" version
	oursCmd := exec.Command("git", "show", ":2:"+relPath)
	oursCmd.Dir = root
	oursOut, err := oursCmd.Output()
	if err != nil {
		return false
	}

	// Get "theirs" version
	theirsCmd := exec.Command("git", "show", ":3:"+relPath)
	theirsCmd.Dir = root
	theirsOut, err := theirsCmd.Output()
	if err != nil {
		return false
	}

	// State priority: higher = more advanced
	statePriority := map[string]int{" ": 0, ">": 1, "x": 2, "v": 3, "!": -1}

	// Parse task states from "theirs"
	theirsStates := make(map[string]string) // task ID → checkbox marker
	taskRe := regexp.MustCompile(`^\s*-\s+\[(.)\]\s+(P\d+-[\w][\w-]*)`)
	theirsLines := strings.Split(string(theirsOut), "\n")
	for _, line := range theirsLines {
		if m := taskRe.FindStringSubmatch(line); m != nil {
			theirsStates[m[2]] = m[1]
		}
	}

	// Collect unique activity entries from "theirs"
	theirsActivityLines := make(map[string]bool)
	inActivity := false
	for _, line := range theirsLines {
		if strings.Contains(line, "## Recent Activity") || strings.Contains(line, "## Activity") || strings.Contains(line, "## Session History") {
			inActivity = true
			continue
		}
		if inActivity && strings.HasPrefix(line, "##") {
			inActivity = false
		}
		if inActivity && strings.HasPrefix(strings.TrimSpace(line), "|") && !strings.Contains(line, "---") {
			theirsActivityLines[strings.TrimSpace(line)] = true
		}
	}

	// Merge: start from "ours", upgrade task states from "theirs"
	oursLines := strings.Split(string(oursOut), "\n")
	var merged []string
	inActivitySection := false
	activityInserted := make(map[string]bool)

	for _, line := range oursLines {
		// Upgrade task checkboxes to the more-advanced state
		if m := taskRe.FindStringSubmatch(line); m != nil {
			oursMarker := m[1]
			taskID := m[2]
			if theirsMarker, ok := theirsStates[taskID]; ok {
				// Take the more-advanced state (but preserve [!] blocked)
				if oursMarker == "!" || theirsMarker == "!" {
					// Keep blocked as-is from ours
				} else if statePriority[theirsMarker] > statePriority[oursMarker] {
					line = strings.Replace(line, "["+oursMarker+"]", "["+theirsMarker+"]", 1)
				}
			}
		}

		// Track activity section for merging entries
		if strings.Contains(line, "## Recent Activity") || strings.Contains(line, "## Activity") || strings.Contains(line, "## Session History") {
			inActivitySection = true
		} else if inActivitySection && strings.HasPrefix(line, "##") {
			for theirsLine := range theirsActivityLines {
				if !activityInserted[theirsLine] {
					merged = append(merged, theirsLine)
					activityInserted[theirsLine] = true
				}
			}
			inActivitySection = false
		}

		if inActivitySection && strings.HasPrefix(strings.TrimSpace(line), "|") && !strings.Contains(line, "---") {
			activityInserted[strings.TrimSpace(line)] = true
		}

		merged = append(merged, line)
	}

	result := strings.Join(merged, "\n")
	if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
		return false
	}

	addCmd := exec.Command("git", "add", relPath)
	addCmd.Dir = root
	addCmd.Run()
	return true
}

// autoResolveLockFiles detects conflicted lock files and regenerates them.
// Only handles lock files whose corresponding manifest is NOT conflicted
// (if the manifest is also conflicted, the AI agent needs to handle both together).
func autoResolveLockFiles(root string) {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return
	}

	// Map lock file → (package manager install command, manifest file)
	lockFileMap := map[string]struct {
		installCmd string
		manifest   string
	}{
		"package-lock.json": {"npm install", "package.json"},
		"pnpm-lock.yaml":    {"pnpm install", "package.json"},
		"yarn.lock":         {"yarn install", "package.json"},
		"bun.lockb":         {"bun install", "package.json"},
		"Cargo.lock":        {"cargo generate-lockfile", "Cargo.toml"},
		"go.sum":            {"go mod tidy", "go.mod"},
		"Gemfile.lock":      {"bundle install", "Gemfile"},
		"poetry.lock":       {"poetry lock --no-update", "pyproject.toml"},
	}

	conflicted := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			conflicted[line] = true
		}
	}

	for _, file := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		file = strings.TrimSpace(file)
		// Only handle lock files in the repo root (basename match)
		baseName := filepath.Base(file)
		info, isLock := lockFileMap[baseName]
		if !isLock {
			continue
		}

		// Check if the corresponding manifest is also conflicted
		manifestPath := filepath.Join(filepath.Dir(file), info.manifest)
		if conflicted[manifestPath] {
			// Both conflicted — leave for the AI agent to handle together
			continue
		}

		fmt.Fprintf(os.Stderr, "  \033[2mAuto-resolving %s via %s\033[0m\n", file, info.installCmd)

		// Delete the conflicted lock file
		os.Remove(filepath.Join(root, file))

		// Run the package manager to regenerate
		parts := strings.Fields(info.installCmd)
		installCmd := exec.Command(parts[0], parts[1:]...)
		installCmd.Dir = root
		if installOut, err := installCmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "  \033[33m⚠ Failed to regenerate %s: %s\033[0m\n", file, strings.TrimSpace(string(installOut)))
			// Restore the conflicted version so git knows it's still unresolved
			checkoutCmd := exec.Command("git", "checkout", "--merge", "--", file)
			checkoutCmd.Dir = root
			checkoutCmd.Run()
			continue
		}

		// Stage the regenerated lock file
		addCmd := exec.Command("git", "add", file)
		addCmd.Dir = root
		addCmd.Run()
	}
}

// listPreservedWorktrees finds worktrees that still exist.
// Checks both the new sibling location and the legacy .belmont/worktrees/ path.
func listPreservedWorktrees(root string) []worktreeEntry {
	var result []worktreeEntry
	// New location: sibling directory
	result = append(result, scanWorktreeDir(worktreeBasePath(root))...)
	// Legacy location: inside project
	legacyDir := filepath.Join(root, ".belmont", "worktrees")
	result = append(result, scanWorktreeDir(legacyDir)...)
	return result
}

// scanWorktreeDir scans a directory for preserved git worktrees.
func scanWorktreeDir(wtDir string) []worktreeEntry {
	entries, err := os.ReadDir(wtDir)
	if err != nil {
		return nil
	}

	var result []worktreeEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		wtPath := filepath.Join(wtDir, e.Name())
		// Check if it's actually a git worktree by looking for .git file
		gitFile := filepath.Join(wtPath, ".git")
		if _, err := os.Stat(gitFile); err != nil {
			continue
		}
		// Detect the actual branch from the worktree's HEAD
		branch := detectWorktreeBranch(wtPath)
		if branch == "" {
			branch = "belmont/auto/" + e.Name() // fallback
		}
		result = append(result, worktreeEntry{Path: wtPath, Branch: branch})
	}
	return result
}

// detectWorktreeBranch reads the actual branch name from a git worktree.
func detectWorktreeBranch(wtPath string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = wtPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return ""
	}
	return branch
}

// runReverifyCmd handles the "belmont reverify" command.
// Walks through completed milestones and runs verification on each sequentially.
// Reports which milestones passed and which had follow-up tasks created.
func runReverifyCmd(args []string) error {
	fs := flag.NewFlagSet("reverify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root, feature, from, to, format, tool string
	fs.StringVar(&root, "root", ".", "project root")
	fs.StringVar(&feature, "feature", "", "feature slug")
	fs.StringVar(&from, "from", "", "start milestone (e.g. M3)")
	fs.StringVar(&to, "to", "", "end milestone (e.g. M10)")
	fs.StringVar(&format, "format", "text", "output format (text|json)")
	fs.StringVar(&tool, "tool", "", "CLI tool (claude|codex|gemini|copilot|cursor)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("reverify: %w", err)
	}
	root, _ = filepath.Abs(root)

	// Auto-detect tool if not specified
	if tool == "" {
		tool = detectTool()
		if tool == "" {
			return fmt.Errorf("reverify: no supported AI tool CLI found on PATH\n\nSupported tools: claude, codex, gemini, copilot, cursor\nInstall one or use --tool to specify")
		}
	}

	// Resolve feature — auto-detect if only one exists
	featuresDir := filepath.Join(root, ".belmont", "features")
	if feature == "" {
		entries, err := os.ReadDir(featuresDir)
		if err != nil {
			return fmt.Errorf("reverify: no features directory at %s", featuresDir)
		}
		var dirs []string
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, e.Name())
			}
		}
		if len(dirs) == 0 {
			return fmt.Errorf("reverify: no features found")
		}
		if len(dirs) > 1 {
			return fmt.Errorf("reverify: multiple features found, use --feature to specify one: %s", strings.Join(dirs, ", "))
		}
		feature = dirs[0]
	}

	progressPath := filepath.Join(featuresDir, feature, "PROGRESS.md")
	progressContent, err := os.ReadFile(progressPath)
	if err != nil {
		return fmt.Errorf("reverify: cannot read %s: %w", progressPath, err)
	}

	milestones := parseMilestones(string(progressContent))
	inRange := milestonesInRange(milestones, from, to)

	// Reset [v] (verified) tasks to [x] (done) in targeted milestones so the
	// verification agent will pick them up again. Only milestones in range that
	// have [v] or [x] tasks are candidates for re-verification.
	//
	// Build a set of milestone IDs that contain [v] tasks and need resetting.
	resetIDs := map[string]bool{}
	for _, m := range inRange {
		for _, t := range m.Tasks {
			if t.Status == taskVerified {
				resetIDs[m.ID] = true
				break
			}
		}
	}

	if len(resetIDs) > 0 {
		// Line-by-line replacement scoped to target milestones.
		msHeaderRe := regexp.MustCompile(`^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):\s*`)
		verifiedTaskRe := regexp.MustCompile(`^(\s*-\s+)\[v\](\s+.*)$`)
		lines := strings.Split(string(progressContent), "\n")
		currentMSID := ""
		changed := false
		for i, line := range lines {
			if hm := msHeaderRe.FindStringSubmatch(line); len(hm) >= 2 {
				currentMSID = "M" + hm[1]
			}
			if resetIDs[currentMSID] {
				if vm := verifiedTaskRe.FindStringSubmatch(line); len(vm) >= 3 {
					lines[i] = vm[1] + "[x]" + vm[2]
					changed = true
				}
			}
		}
		if changed {
			newContent := strings.Join(lines, "\n")
			if err := os.WriteFile(progressPath, []byte(newContent), 0644); err != nil {
				return fmt.Errorf("reverify: failed to reset verified tasks: %w", err)
			}
			// Re-parse after rewrite so downstream filtering sees [x] tasks.
			progressContent = []byte(newContent)
			milestones = parseMilestones(newContent)
			inRange = milestonesInRange(milestones, from, to)
		}
	}

	// Find milestones that have [x] (done but not verified) tasks
	var targets []milestone
	for _, m := range inRange {
		hasDoneTasks := false
		for _, t := range m.Tasks {
			if t.Status == taskDone {
				hasDoneTasks = true
				break
			}
		}
		if hasDoneTasks {
			targets = append(targets, m)
		}
	}

	if len(targets) == 0 {
		if format == "json" {
			fmt.Println(`{"verified":0,"results":[]}`)
		} else {
			fmt.Fprintln(os.Stderr, "No milestones with unverified tasks to re-verify in the specified range.")
		}
		return nil
	}

	// Print header
	ids := make([]string, len(targets))
	for i, m := range targets {
		ids[i] = m.ID
	}
	msWord := "milestones"
	if len(targets) == 1 {
		msWord = "milestone"
	}
	fmt.Fprintf(os.Stderr, "\033[1mBelmont Reverify\033[0m — %s (%d %s)\n", feature, len(targets), msWord)
	fmt.Fprintf(os.Stderr, "Tool: %s | Milestones: %s\n\n", tool, strings.Join(ids, ", "))

	// Walk milestones sequentially, running verification on each
	type msResult struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Passed   bool     `json:"passed"`
		Fwlups   []string `json:"fwlups,omitempty"`
		Duration float64  `json:"duration_s"`
		Error    string   `json:"error,omitempty"`
	}
	results := make([]msResult, 0, len(targets))

	// Load per-feature model tiers once; reverify maps to the verification agent tier.
	tiers, _ := parseModelTiers(filepath.Join(featuresDir, feature, "models.yaml"))
	verifyModelFlags := resolveModelFlags(tool, tiers.Tiers["verification"])

	for i, m := range targets {
		fmt.Fprintf(os.Stderr, "━━ [%d/%d] VERIFY ━━ %s › %s: %s ━━\n", i+1, len(targets), feature, m.ID, m.Name)

		// Build milestone-scoped verify prompt
		prompt := fmt.Sprintf("/belmont:verify --feature %s", feature)
		prompt += fmt.Sprintf("\n\nMILESTONE-SCOPED VERIFICATION: Only verify tasks marked [x] (done) in milestone %s. Do NOT verify tasks from other milestones. Focus on: (1) the tasks in %s meet their acceptance criteria, (2) build passes, (3) tests pass.\n\nOn success: mark verified tasks as [v] in PROGRESS.md.\nOn failure: add new [ ] follow-up tasks to milestone %s and leave originals as [x].\n\nCRITICAL: Do NOT modify tasks in any other milestone.", m.ID, m.ID, m.ID)

		// Build and run the tool command
		var cmd *exec.Cmd
		switch tool {
		case "claude":
			args := []string{"-p", prompt,
				"--permission-mode", "bypassPermissions",
				"--allowedTools", "Bash Read Write Edit Glob Grep Agent Skill WebFetch WebSearch mcp__*",
				"--output-format", "stream-json", "--verbose"}
			args = append(args, verifyModelFlags...)
			cmd = exec.Command("claude", args...)
		case "codex":
			args := []string{"exec", prompt,
				"--dangerously-bypass-approvals-and-sandbox",
				"--json", "-C", root}
			args = append(args, verifyModelFlags...)
			cmd = exec.Command("codex", args...)
		case "gemini":
			args := []string{prompt, "--yolo", "--output-format", "json"}
			args = append(args, verifyModelFlags...)
			cmd = exec.Command("gemini", args...)
		case "copilot":
			args := []string{"-p", prompt, "--yolo"}
			args = append(args, verifyModelFlags...)
			cmd = exec.Command("copilot", args...)
		case "cursor":
			args := []string{"agent", "-p", prompt, "--force", "--output-format", "json"}
			args = append(args, verifyModelFlags...)
			cmd = exec.Command("cursor", args...)
		default:
			return fmt.Errorf("reverify: unsupported tool: %s", tool)
		}
		cmd.Dir = root

		prefix := fmt.Sprintf("\033[36m[%s][%s]\033[0m: ", feature, m.ID)
		var tw *tailWriter
		if tool == "claude" {
			tw = newTailWriter(os.Stderr, 1500, "")
			cmd.Stdout = &claudeStreamWriter{tw: tw, prefix: prefix}
			cmd.Stderr = tw
		} else {
			tw = newTailWriter(os.Stderr, 1500, prefix)
			cmd.Stdout = tw
			cmd.Stderr = tw
		}

		start := time.Now()
		runErr := cmd.Run()
		duration := time.Since(start)

		res := msResult{
			ID:       m.ID,
			Name:     m.Name,
			Duration: duration.Seconds(),
		}

		if runErr != nil {
			res.Passed = false
			res.Error = runErr.Error()
			fmt.Fprintf(os.Stderr, "\n\033[31m  ✗ %s failed (%.1fs): %s\033[0m\n\n", m.ID, res.Duration, runErr)
		} else {
			fmt.Fprintf(os.Stderr, "\n\033[32m  ✓ %s (%.1fs)\033[0m\n", m.ID, res.Duration)

			// Re-read status to detect verification results
			report, statusErr := buildStatus(root, 55, feature)
			if statusErr == nil {
				// Check for new incomplete tasks (follow-ups) in this milestone
				var followups []string
				for _, t := range report.Tasks {
					if (t.Status == taskTodo || t.Status == taskInProgress) &&
						t.MilestoneID == m.ID {
						label := t.ID
						if label == "" {
							label = t.Name
						}
						followups = append(followups, label)
					}
				}
				// Also check if any [x] tasks remain (verification didn't pass them)
				hasUnverified := false
				for _, t := range report.Tasks {
					if t.Status == taskDone && t.MilestoneID == m.ID {
						hasUnverified = true
						break
					}
				}
				res.Fwlups = followups
				res.Passed = len(followups) == 0 && !hasUnverified
			} else {
				res.Passed = true // assume passed if we can't check
			}

			if len(res.Fwlups) > 0 {
				fmt.Fprintf(os.Stderr, "  \033[33m  %d follow-up(s): %s\033[0m\n\n", len(res.Fwlups), strings.Join(res.Fwlups, ", "))
			} else {
				fmt.Fprintln(os.Stderr)
			}
		}

		results = append(results, res)
	}

	// Print summary
	passed := 0
	var allFwlups []string
	for _, r := range results {
		if r.Passed {
			passed++
		}
		allFwlups = append(allFwlups, r.Fwlups...)
	}

	if format == "json" {
		// Build JSON manually to avoid importing encoding/json just for this
		fmt.Printf(`{"feature":%q,"verified":%d,"passed":%d,"total_fwlups":%d,"results":[`, feature, len(results), passed, len(allFwlups))
		for i, r := range results {
			if i > 0 {
				fmt.Print(",")
			}
			fwlupsJSON := "[]"
			if len(r.Fwlups) > 0 {
				fwlupsJSON = `["` + strings.Join(r.Fwlups, `","`) + `"]`
			}
			errJSON := "null"
			if r.Error != "" {
				errJSON = fmt.Sprintf("%q", r.Error)
			}
			fmt.Printf(`{"id":%q,"name":%q,"passed":%t,"fwlups":%s,"duration_s":%.1f,"error":%s}`,
				r.ID, r.Name, r.Passed, fwlupsJSON, r.Duration, errJSON)
		}
		fmt.Println("]}")
	} else {
		fmt.Fprintln(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Fprintf(os.Stderr, "\033[1mReverify Summary\033[0m — %s\n\n", feature)
		for _, r := range results {
			if r.Error != "" {
				fmt.Fprintf(os.Stderr, "  \033[31m✗\033[0m %s: %s — error: %s\n", r.ID, r.Name, r.Error)
			} else if !r.Passed {
				fmt.Fprintf(os.Stderr, "  \033[33m⚠\033[0m %s: %s — %d follow-up(s): %s\n", r.ID, r.Name, len(r.Fwlups), strings.Join(r.Fwlups, ", "))
			} else {
				fmt.Fprintf(os.Stderr, "  \033[32m✓\033[0m %s: %s\n", r.ID, r.Name)
			}
		}
		fmt.Fprintf(os.Stderr, "\n  %d/%d passed", passed, len(results))
		if len(allFwlups) > 0 {
			fmt.Fprintf(os.Stderr, ", %d follow-up task(s) created", len(allFwlups))
		}
		fmt.Fprintln(os.Stderr)

		if len(allFwlups) > 0 {
			fmt.Fprintf(os.Stderr, "\nTo fix follow-ups: belmont auto --feature %s\n", feature)
		}
	}

	return nil
}

func runSyncCmd(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root string
	fs.StringVar(&root, "root", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	root, _ = filepath.Abs(root)

	// If running in a worktree context (detected via .worktree marker), no-op.
	// Worktrees have isolated state; sync only makes sense on the main repo.
	if fileExists(filepath.Join(root, ".belmont", ".worktree")) {
		return nil
	}

	featuresDir := filepath.Join(root, ".belmont", "features")
	features := listFeatures(featuresDir, 50)
	if len(features) == 0 {
		return nil
	}

	syncMasterFeatureStatuses(root, features)

	// Commit if anything changed (best-effort)
	commitBelmontState(root)
	return nil
}

// runRecover handles the "belmont recover" command.
// migrateToUnifiedTracking detects and converts old dual-file state tracking to the new
// unified PROGRESS.md format. Old format: PRD.md had emoji on task headers, PROGRESS.md had
// emoji on milestone headers and ## Blockers/## Status sections. New format: PROGRESS.md
// has task checkboxes with [ ]/[>]/[x]/[v]/[!] states, no milestone emojis, no Blockers/Status sections.
func migrateToUnifiedTracking(root string) {
	featuresDir := filepath.Join(root, ".belmont", "features")
	entries, err := os.ReadDir(featuresDir)
	if err != nil {
		return
	}

	// Detect if migration is needed by checking first feature
	needsMigration := false
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		progressPath := filepath.Join(featuresDir, e.Name(), "PROGRESS.md")
		data, err := os.ReadFile(progressPath)
		if err != nil {
			continue
		}
		content := string(data)
		// Old format indicators: emoji on milestone headers or ## Blockers section
		if regexp.MustCompile(`(?m)^###\s+[✅⬜🔄🚫]\s*M\d+:`).MatchString(content) ||
			strings.Contains(content, "## Blockers") ||
			strings.Contains(content, "## Status:") {
			needsMigration = true
		}
		break
	}

	if !needsMigration {
		return
	}

	fmt.Println("\nMigrating to unified state tracking...")

	migratedCount := 0
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		slug := e.Name()
		featurePath := filepath.Join(featuresDir, slug)

		// Step 1: Parse task statuses from old PRD.md
		prdPath := filepath.Join(featurePath, "PRD.md")
		prdTaskStatuses := make(map[string]string) // task ID → new checkbox marker
		if prdData, err := os.ReadFile(prdPath); err == nil {
			prdRe := regexp.MustCompile(`(?m)^###\s+(P\d+-[\w][\w-]*):\s*(.+)$`)
			for _, match := range prdRe.FindAllStringSubmatch(string(prdData), -1) {
				id := strings.TrimSpace(match[1])
				text := match[2]
				marker := " " // default: todo
				if strings.Contains(text, "✅") || regexp.MustCompile(`(?i)\[done\]`).MatchString(text) {
					marker = "x" // done (not verified — conservative)
				} else if strings.Contains(text, "🚫") || regexp.MustCompile(`(?i)blocked`).MatchString(text) {
					marker = "!"
				}
				prdTaskStatuses[id] = marker
			}

			// Step 2: Strip emoji from PRD.md task headers
			updated := string(prdData)
			for _, emoji := range []string{"✅", "🚫", "🔄", "⬜", "🔵"} {
				updated = strings.ReplaceAll(updated, emoji, "")
			}
			// Clean up extra spaces from removed emojis
			updated = regexp.MustCompile(`(?m)^(###\s+P\d+-[\w][\w-]*:\s*)\s+`).ReplaceAllString(updated, "$1")
			updated = regexp.MustCompile(`\s+$`).ReplaceAllString(updated, "")
			os.WriteFile(prdPath, []byte(strings.TrimRight(updated, "\n")+"\n"), 0644)
		}

		// Step 3: Update PROGRESS.md
		progressPath := filepath.Join(featurePath, "PROGRESS.md")
		progressData, err := os.ReadFile(progressPath)
		if err != nil {
			continue
		}

		lines := strings.Split(string(progressData), "\n")
		var newLines []string
		skipBlockers := false
		skipStatus := false

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)

			// Remove ## Status: line
			if strings.HasPrefix(trimmed, "## Status:") {
				skipStatus = true
				continue
			}
			if skipStatus {
				if trimmed == "" {
					skipStatus = false
					continue
				}
				skipStatus = false
			}

			// Remove ## Blockers section
			if strings.HasPrefix(trimmed, "## Blockers") {
				skipBlockers = true
				continue
			}
			if skipBlockers {
				if strings.HasPrefix(trimmed, "## ") {
					skipBlockers = false
					// Fall through to process this line
				} else {
					continue
				}
			}

			// Remove emoji from milestone headers: ### ✅ M1: → ### M1:
			msRe := regexp.MustCompile(`^(###\s+)[✅⬜🔄🚫]\s*(M\d+:.*)$`)
			if m := msRe.FindStringSubmatch(line); m != nil {
				line = m[1] + m[2]
			}

			// Upgrade task checkboxes using PRD.md statuses
			taskRe := regexp.MustCompile(`^(\s*-\s+)\[[ xX]\](\s+)(P\d+-[\w][\w-]*)(.*)$`)
			if m := taskRe.FindStringSubmatch(line); m != nil {
				taskID := m[3]
				if marker, ok := prdTaskStatuses[taskID]; ok {
					line = m[1] + "[" + marker + "]" + m[2] + m[3] + m[4]
				}
			}

			newLines = append(newLines, line)
		}

		os.WriteFile(progressPath, []byte(strings.Join(newLines, "\n")), 0644)
		migratedCount++
		fmt.Printf("  Migrated feature '%s'\n", slug)
	}

	// Step 4: Migrate master PRD.md — remove features table
	masterPrdPath := filepath.Join(root, ".belmont", "PRD.md")
	if prdData, err := os.ReadFile(masterPrdPath); err == nil {
		content := string(prdData)
		// Remove the features table section
		featuresTableRe := regexp.MustCompile(`(?ms)^## Features\s*\n.*?(?:\n## |\z)`)
		if featuresTableRe.MatchString(content) {
			// Extract priority/deps from old table before removing
			oldDeps, oldPriorities := parseMasterPRDTableLegacy(content)

			// Remove the features table from PRD
			updated := featuresTableRe.ReplaceAllString(content, "")
			// Add global doc sections if not present
			if !strings.Contains(updated, "## Cross-Cutting Decisions") {
				updated = strings.TrimRight(updated, "\n") + "\n\n## Cross-Cutting Decisions\n\n(Add cross-cutting product decisions here)\n"
			}
			if !strings.Contains(updated, "## Constraints") {
				updated = strings.TrimRight(updated, "\n") + "\n\n## Constraints\n\n(Add project-wide constraints here)\n"
			}
			os.WriteFile(masterPrdPath, []byte(updated), 0644)

			// Step 5: Add Priority + Dependencies columns to master PROGRESS.md
			masterProgressPath := filepath.Join(root, ".belmont", "PROGRESS.md")
			if progressData, err := os.ReadFile(masterProgressPath); err == nil {
				migratedProgress := migrateMasterProgressTable(string(progressData), oldDeps, oldPriorities)
				os.WriteFile(masterProgressPath, []byte(migratedProgress), 0644)
			}
		}
	}

	if migratedCount > 0 {
		fmt.Printf("\n  Migrated %d feature(s). Done tasks mapped to [x] (not yet verified).\n", migratedCount)
		fmt.Println("  Run 'belmont reverify' to verify completed work.")
	}
}

// parseMasterPRDTableLegacy extracts deps and priorities from the old-format master PRD features table.
func parseMasterPRDTableLegacy(content string) (deps map[string][]string, priorities map[string]string) {
	deps = make(map[string][]string)
	priorities = make(map[string]string)
	lines := strings.Split(content, "\n")
	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## Features") {
			inTable = true
			continue
		}
		if inTable && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if !inTable || !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := splitTableCells(trimmed)
		if len(cells) < 4 {
			continue
		}
		slug := strings.TrimSpace(cells[1])
		if slug == "Slug" || strings.HasPrefix(slug, "-") || strings.HasPrefix(slug, ":") {
			continue
		}
		priorities[slug] = strings.TrimSpace(cells[2])
		depStr := strings.TrimSpace(cells[3])
		if depStr != "" && !strings.EqualFold(depStr, "None") && depStr != "-" {
			for _, d := range strings.Split(depStr, ",") {
				d = strings.TrimSpace(d)
				if d != "" {
					deps[slug] = append(deps[slug], d)
				}
			}
		}
	}
	return
}

// migrateMasterProgressTable adds Priority and Dependencies columns to the master PROGRESS.md features table.
func migrateMasterProgressTable(content string, deps map[string][]string, priorities map[string]string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inTable := false
	headerDone := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## Features") {
			inTable = true
			result = append(result, line)
			continue
		}
		if inTable && strings.HasPrefix(trimmed, "## ") {
			inTable = false
		}

		if inTable && strings.HasPrefix(trimmed, "|") {
			cells := splitTableCells(trimmed)
			if !headerDone {
				// Check if it's a header row
				if len(cells) >= 2 && cells[0] == "Feature" {
					// Replace header: add Priority and Dependencies after Slug
					result = append(result, "| Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |")
					result = append(result, "|---------|------|----------|-------------|--------|------------|-------|")
					headerDone = true
					continue
				}
				// Separator row
				if strings.Contains(trimmed, "---") {
					continue // already added separator above
				}
			} else {
				// Data row — add priority and deps columns
				if len(cells) >= 2 && !strings.HasPrefix(cells[0], "-") {
					slug := strings.TrimSpace(cells[1])
					if strings.HasPrefix(slug, "-") || slug == "Slug" {
						result = append(result, line)
						continue
					}
					priority := "P1"
					if p, ok := priorities[slug]; ok {
						priority = p
					}
					depStr := "None"
					if d, ok := deps[slug]; ok && len(d) > 0 {
						depStr = strings.Join(d, ", ")
					}
					status := ""
					ms := ""
					tasks := ""
					if len(cells) >= 3 {
						status = cells[2]
					}
					if len(cells) >= 4 {
						ms = cells[3]
					}
					if len(cells) >= 5 {
						tasks = cells[4]
					}
					result = append(result, fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |",
						cells[0], slug, priority, depStr, status, ms, tasks))
					continue
				}
			}
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

func runRecover(args []string) error {
	fs := flag.NewFlagSet("recover", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root string
	var format string
	var list bool
	var merge string
	var clean string
	var cleanAll bool
	var tool string
	fs.StringVar(&root, "root", ".", "project root")
	fs.StringVar(&format, "format", "text", "text or json")
	fs.BoolVar(&list, "list", false, "list preserved worktrees")
	fs.StringVar(&merge, "merge", "", "retry merge for slug")
	fs.StringVar(&clean, "clean", "", "delete worktree and branch for slug")
	fs.BoolVar(&cleanAll, "clean-all", false, "clean all preserved worktrees")
	fs.StringVar(&tool, "tool", "", "CLI tool for reconciliation (claude|codex|gemini|copilot|cursor) — auto-detected if omitted")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("recover: %w", err)
	}

	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	worktrees := listPreservedWorktrees(root)

	if merge != "" {
		return recoverMerge(root, merge, tool, worktrees)
	}
	if clean != "" {
		return recoverClean(root, clean, worktrees)
	}
	if cleanAll {
		return recoverCleanAll(root, worktrees, format)
	}

	// Default: list (explicit or implicit)
	return recoverList(root, worktrees, format)
}

func recoverList(root string, worktrees []worktreeEntry, format string) error {
	if format == "json" {
		type wtJSON struct {
			Slug   string `json:"slug"`
			Path   string `json:"path"`
			Branch string `json:"branch"`
		}
		var items []wtJSON
		for _, wt := range worktrees {
			slug := filepath.Base(wt.Path)
			items = append(items, wtJSON{Slug: slug, Path: wt.Path, Branch: wt.Branch})
		}
		if items == nil {
			items = []wtJSON{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	if len(worktrees) == 0 {
		fmt.Println("No preserved worktrees found.")
		return nil
	}

	fmt.Printf("Preserved worktrees (%d):\n\n", len(worktrees))
	for _, wt := range worktrees {
		slug := filepath.Base(wt.Path)
		fmt.Printf("  %s\n", slug)
		fmt.Printf("    Path:   %s\n", wt.Path)
		fmt.Printf("    Branch: %s\n", wt.Branch)
		fmt.Println()
	}
	fmt.Println("Actions:")
	fmt.Println("  belmont recover --merge <slug>    Retry merge with improved logic (uses --tool for reconciliation)")
	fmt.Println("  belmont recover --clean <slug>    Delete worktree and branch")
	fmt.Println("  belmont recover --clean-all       Clean all preserved worktrees")
	return nil
}

func findWorktree(worktrees []worktreeEntry, slug string) *worktreeEntry {
	for _, wt := range worktrees {
		if filepath.Base(wt.Path) == slug {
			return &wt
		}
	}
	return nil
}

func recoverMerge(root, slug, tool string, worktrees []worktreeEntry) error {
	wt := findWorktree(worktrees, slug)
	if wt == nil {
		return fmt.Errorf("no preserved worktree found for slug: %s", slug)
	}

	// Reconciliation needs an AI tool to analyze conflicts. Honour the explicit
	// --tool flag if given, otherwise auto-detect (mirrors the `auto` command).
	if tool == "" {
		tool = detectTool()
		if tool == "" {
			return fmt.Errorf("recover: no supported AI tool CLI found on PATH\n\nSupported tools: claude, codex, gemini, copilot, cursor\nInstall one or use --tool to specify")
		}
	} else {
		switch tool {
		case "claude", "codex", "gemini", "copilot", "cursor":
			// ok
		default:
			return fmt.Errorf("recover: unsupported tool %q (use claude, codex, gemini, copilot, or cursor)", tool)
		}
	}

	commitMsg := fmt.Sprintf("belmont: merge recovered %s", slug)
	cfg := loopConfig{Root: root, Tool: tool}

	if err := attemptMerge(cfg, commitMsg, wt.Branch, slug); err != nil {
		return fmt.Errorf("merge failed for %s: %w", slug, err)
	}

	// Clean up reconciliation report if it exists
	os.Remove(filepath.Join(root, ".belmont", "reconciliation-report.json"))

	// Clean up worktree and branch
	removeWorktree(root, wt.Path, slug)

	delCmd := exec.Command("git", "branch", "-d", wt.Branch)
	delCmd.Dir = root
	delCmd.Run()

	fmt.Fprintf(os.Stderr, "  \033[32m✓ Recovered and merged %s\033[0m\n", slug)
	return nil
}

func recoverClean(root, slug string, worktrees []worktreeEntry) error {
	wt := findWorktree(worktrees, slug)
	if wt == nil {
		return fmt.Errorf("no preserved worktree found for slug: %s", slug)
	}

	removeWorktree(root, wt.Path, slug)

	delCmd := exec.Command("git", "branch", "-D", wt.Branch)
	delCmd.Dir = root
	delCmd.Run()

	fmt.Fprintf(os.Stderr, "  \033[32m✓ Cleaned up %s\033[0m\n", slug)
	return nil
}

func recoverCleanAll(root string, worktrees []worktreeEntry, format string) error {
	if len(worktrees) == 0 {
		if format != "json" {
			fmt.Println("No preserved worktrees to clean.")
		}
		return nil
	}

	for _, wt := range worktrees {
		slug := filepath.Base(wt.Path)
		removeWorktree(root, wt.Path, slug)

		delCmd := exec.Command("git", "branch", "-D", wt.Branch)
		delCmd.Dir = root
		delCmd.Run()

		fmt.Fprintf(os.Stderr, "  \033[32m✓ Cleaned up %s\033[0m\n", slug)
	}
	return nil
}

// ============================================================================
// belmont steer — inject user instructions into an in-flight auto run.
//
// The auto loop runs headless AI CLI invocations inside isolated worktrees;
// there is no channel for the user to interject. `belmont steer` writes an
// append-only STEERING.md in each active worktree (or the master feature
// directory for non-parallel runs). executeLoopAction consumes pending
// entries before each phase and prepends them to the agent prompt as a
// higher-priority block than NOTES.md.
// ============================================================================

// steeringEntry represents a single block in STEERING.md.
type steeringEntry struct {
	Timestamp string // RFC3339 UTC from the header
	Milestone string // optional — empty means applies to any milestone
	State     string // "pending" or "consumed <ts> by <phase>"
	Body      string // free-form text between this header and the next
}

var steeringHeaderRe = regexp.MustCompile(`^##\s+(\S+)(?:\s+\[([^\]]+)\])?\s+\(([^)]+)\)\s*$`)

// parseSteeringEntries walks STEERING.md and returns every block it finds.
// Unrecognised preamble or garbage between entries is ignored.
func parseSteeringEntries(data string) []steeringEntry {
	lines := strings.Split(data, "\n")
	var entries []steeringEntry
	var current *steeringEntry
	var body []string
	flush := func() {
		if current == nil {
			return
		}
		current.Body = strings.TrimRight(strings.Join(body, "\n"), "\n")
		entries = append(entries, *current)
		current = nil
		body = nil
	}
	for _, line := range lines {
		if m := steeringHeaderRe.FindStringSubmatch(line); m != nil {
			flush()
			current = &steeringEntry{
				Timestamp: m[1],
				Milestone: m[2],
				State:     m[3],
			}
			continue
		}
		if current != nil {
			body = append(body, line)
		}
	}
	flush()
	return entries
}

// renderSteeringEntries serialises entries back to STEERING.md format.
// Preserves order.
func renderSteeringEntries(entries []steeringEntry) string {
	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## ")
		b.WriteString(e.Timestamp)
		if e.Milestone != "" {
			b.WriteString(" [")
			b.WriteString(e.Milestone)
			b.WriteString("]")
		}
		b.WriteString(" (")
		b.WriteString(e.State)
		b.WriteString(")\n")
		if trimmed := strings.TrimSpace(e.Body); trimmed != "" {
			b.WriteString(trimmed)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// steeringHeader returns the prompt-side framing for injected instructions.
// The wording is deliberately forceful — the point of steering is to override
// the normal prompt flow when the user has new information.
func steeringHeader() string {
	return `## URGENT — User steering (higher priority than NOTES.md)

The user has injected the following instruction(s) into this feature loop. They override or amend the surrounding task. Read them carefully, apply them to your current action, and acknowledge them at the start of your reply.

`
}

// consumePendingSteering reads STEERING.md for the given feature, takes any
// pending entry that matches the current milestone (or has no milestone tag),
// and returns the formatted user-steering block (prefixed with steeringHeader)
// plus a count. Returns ("", 0) when there is nothing pending.
//
// Invariant: STEERING.md contains only (pending) entries. Consumed entries
// are dropped from disk entirely (the audit lives in the auto run's stderr
// stream — the `[STEERING] injected …` line and its timestamp). STEERING.md
// is deleted when no pending entries remain so agents exploring the feature
// dir don't see a stale file and burn input tokens re-reading text that's
// already in the prompt.
//
// Legacy (consumed) entries written by older versions of this code are
// silently dropped on first encounter, same migration path.
//
// All filesystem errors are non-fatal.
func consumePendingSteering(root, feature, milestoneID, phase string) (string, int) {
	if root == "" || feature == "" {
		return "", 0
	}
	path := filepath.Join(root, ".belmont", "features", feature, "STEERING.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0
	}
	entries := parseSteeringEntries(string(data))
	now := time.Now().UTC().Format(time.RFC3339)

	var remainingPending []steeringEntry
	var newlyConsumed []steeringEntry
	for _, e := range entries {
		if e.State != "pending" {
			// Legacy consumed entries from older code — drop silently.
			continue
		}
		if e.Milestone == "" || e.Milestone == milestoneID {
			e.State = fmt.Sprintf("consumed %s by %s", now, phase)
			newlyConsumed = append(newlyConsumed, e)
		} else {
			remainingPending = append(remainingPending, e)
		}
	}

	// Rewrite STEERING.md with only pending entries; delete when empty so
	// the live file acts purely as the agent-facing inbox.
	if len(remainingPending) == 0 {
		_ = os.Remove(path)
	} else {
		_ = os.WriteFile(path, []byte(renderSteeringEntries(remainingPending)), 0644)
	}

	if len(newlyConsumed) == 0 {
		return "", 0
	}

	var b strings.Builder
	b.WriteString(steeringHeader())
	for i, e := range newlyConsumed {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		if e.Milestone != "" {
			fmt.Fprintf(&b, "Scope: milestone %s\n\n", e.Milestone)
		}
		b.WriteString(strings.TrimSpace(e.Body))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return b.String(), len(newlyConsumed)
}

// logSteeringInjection prints a one-line notice to stderr matching the
// `[feature][milestone]: ...` prefix used by the auto loop. The preview
// is the first ~100 chars of the first consumed entry's body, truncated
// with `…` so the stream stays one line per injection.
func logSteeringInjection(feature, milestoneID string, count int, block string) {
	preview := steeringPreview(block)
	noun := "instruction"
	if count != 1 {
		noun = "instructions"
	}
	var prefix string
	if feature != "" {
		if milestoneID != "" {
			prefix = fmt.Sprintf("\033[36m[%s][%s]\033[0m: ", feature, milestoneID)
		} else {
			prefix = fmt.Sprintf("\033[36m[%s]\033[0m: ", feature)
		}
	}
	fmt.Fprintf(os.Stderr, "%s\033[35m[STEERING]\033[0m injected %d %s — \"%s\"\n", prefix, count, noun, preview)
}

// steeringPreview extracts the first non-header line of the injected block
// and truncates to ~100 chars.
func steeringPreview(block string) string {
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "##") || strings.HasPrefix(trimmed, "---") {
			continue
		}
		if strings.HasPrefix(trimmed, "The user has injected") {
			continue
		}
		if strings.HasPrefix(trimmed, "Scope:") {
			continue
		}
		if len(trimmed) > 100 {
			return trimmed[:99] + "…"
		}
		return trimmed
	}
	return ""
}

// steeringTarget identifies a single worktree (or master root) to write
// STEERING.md into.
type steeringTarget struct {
	MilestoneID string // empty for non-parallel runs
	Root        string // absolute worktree path (or master root)
	Label       string // e.g. "M5" or "serial" — for log output
}

// runSteerCmd implements `belmont steer`.
func runSteerCmd(args []string) error {
	fs := flag.NewFlagSet("steer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root, feature, milestone, message, file string
	fs.StringVar(&root, "root", ".", "project root")
	fs.StringVar(&feature, "feature", "", "feature slug (auto-detected if only one active)")
	fs.StringVar(&milestone, "milestone", "", "narrow steering to a single milestone (e.g. M5)")
	fs.StringVar(&message, "message", "", "steering text (inline)")
	fs.StringVar(&file, "file", "", "read steering text from file")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("steer: %w", err)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("steer: resolve root: %w", err)
	}

	// Resolve the active auto run and its feature.
	aj, err := readActiveAutoJSON(absRoot)
	if err != nil {
		return err
	}
	resolvedFeature, err := resolveSteerFeature(aj, feature)
	if err != nil {
		return err
	}

	// Read the steering text from exactly one source.
	text, err := readSteeringInput(fs.Args(), message, file, resolvedFeature, milestone)
	if err != nil {
		return err
	}

	// Figure out targets: every active worktree for the feature, optionally
	// narrowed to one milestone. In serial mode the only target is the master
	// feature directory.
	targets, err := resolveSteeringTargets(absRoot, aj, resolvedFeature, milestone)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("steer: no active worktree matched feature=%q milestone=%q", resolvedFeature, milestone)
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	for _, t := range targets {
		entryMilestone := milestone
		if entryMilestone == "" && t.MilestoneID != "" && len(targets) > 1 {
			// Broadcast into a parallel run: tag each entry with the target
			// milestone so it only fires when that milestone runs.
			entryMilestone = t.MilestoneID
		}
		path := filepath.Join(t.Root, ".belmont", "features", resolvedFeature, "STEERING.md")
		if err := appendSteeringEntry(path, timestamp, entryMilestone, text); err != nil {
			return fmt.Errorf("steer: write %s: %w", path, err)
		}
		fmt.Fprintf(os.Stderr, "  \033[32m✓\033[0m injected → %s", path)
		if entryMilestone != "" {
			fmt.Fprintf(os.Stderr, " \033[2m[%s]\033[0m", entryMilestone)
		}
		fmt.Fprintln(os.Stderr)
	}
	return nil
}

// readActiveAutoJSON returns auto.json if there's an active run; errors
// otherwise with a helpful message.
func readActiveAutoJSON(root string) (autoJSON, error) {
	autoPath := filepath.Join(root, ".belmont", "auto.json")
	data, err := os.ReadFile(autoPath)
	if err != nil {
		return autoJSON{}, fmt.Errorf("steer: no active auto run (missing %s). steering only applies to in-flight auto mode — start one with `belmont auto`, or steer a manual CLI session by typing directly into it", autoPath)
	}
	var aj autoJSON
	if err := json.Unmarshal(data, &aj); err != nil {
		return autoJSON{}, fmt.Errorf("steer: parse auto.json: %w", err)
	}
	if !aj.Active {
		return autoJSON{}, fmt.Errorf("steer: auto.json exists but no active run — nothing to steer")
	}
	return aj, nil
}

// resolveSteerFeature picks the feature slug from --feature or auto.json.
func resolveSteerFeature(aj autoJSON, requested string) (string, error) {
	if requested != "" {
		if aj.Feature != "" && aj.Feature != requested {
			return "", fmt.Errorf("steer: feature %q is not the active auto run (active: %q)", requested, aj.Feature)
		}
		return requested, nil
	}
	if aj.Feature != "" {
		return aj.Feature, nil
	}
	// Multi-feature runs don't set aj.Feature; require explicit selection.
	return "", fmt.Errorf("steer: --feature required (auto.json does not record a single active feature)")
}

// resolveSteeringTargets returns the writable targets for a steer request.
// When auto.json has per-milestone worktree entries, each entry becomes a
// target; otherwise the single target is the master feature directory.
func resolveSteeringTargets(root string, aj autoJSON, feature, milestone string) ([]steeringTarget, error) {
	// Parallel: one target per active worktree entry for the feature.
	if len(aj.Worktrees) > 0 {
		var targets []steeringTarget
		for id, entry := range aj.Worktrees {
			if milestone != "" && id != milestone {
				continue
			}
			// Verify the worktree still exists — stale entries shouldn't
			// silently accept writes.
			if !dirExists(entry.Path) {
				continue
			}
			targets = append(targets, steeringTarget{
				MilestoneID: id,
				Root:        entry.Path,
				Label:       id,
			})
		}
		sort.Slice(targets, func(i, j int) bool { return targets[i].MilestoneID < targets[j].MilestoneID })
		return targets, nil
	}
	// Serial: single target is the master feature directory under root.
	featureDir := filepath.Join(root, ".belmont", "features", feature)
	if !dirExists(featureDir) {
		return nil, fmt.Errorf("steer: feature directory not found: %s", featureDir)
	}
	return []steeringTarget{{Root: root, Label: "serial"}}, nil
}

// readSteeringInput reads steering text from exactly one source (errors if
// zero or two+ sources are provided). Sources are, in order: --message,
// --file, "-" positional for stdin, or $EDITOR fallback when stdin is a
// TTY and $EDITOR is set.
func readSteeringInput(positional []string, message, file, feature, milestone string) (string, error) {
	wantStdin := false
	for _, a := range positional {
		if a == "-" {
			wantStdin = true
		}
	}
	sourceCount := 0
	if message != "" {
		sourceCount++
	}
	if file != "" {
		sourceCount++
	}
	if wantStdin {
		sourceCount++
	}
	if sourceCount > 1 {
		return "", fmt.Errorf("steer: provide exactly one of --message, --file, or `-` (stdin)")
	}

	var text string
	switch {
	case message != "":
		text = message
	case file != "":
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("steer: read --file: %w", err)
		}
		text = string(data)
	case wantStdin:
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("steer: read stdin: %w", err)
		}
		text = string(data)
	default:
		// $EDITOR fallback — only when attached to a TTY and EDITOR is set.
		if !isTerminal(os.Stdin) {
			return "", fmt.Errorf("steer: no input provided — pass --message \"text\", --file PATH, or `-` (stdin)")
		}
		editor := os.Getenv("EDITOR")
		if editor == "" {
			return "", fmt.Errorf("steer: no input provided and $EDITOR is unset — pass --message \"text\", --file PATH, or `-` (stdin)")
		}
		edited, err := runSteerEditor(editor, feature, milestone)
		if err != nil {
			return "", err
		}
		text = edited
	}

	// Strip lines beginning with `#` when the text came through $EDITOR;
	// harmless for other sources (users rarely write literal `#` lines at
	// the column 0 position).
	text = stripSteerComments(text)
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", fmt.Errorf("steer: empty steering text — aborting")
	}
	return trimmed, nil
}

// stripSteerComments drops lines beginning with `#` and any trailing blank
// lines left behind. Used for $EDITOR input so the seeded template comments
// don't end up in STEERING.md.
func stripSteerComments(text string) string {
	var keep []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n")
}

// runSteerEditor opens $EDITOR on a seeded temp file and returns the saved
// contents. An empty edit (after comment stripping) returns an error.
func runSteerEditor(editor, feature, milestone string) (string, error) {
	tmp, err := os.CreateTemp("", "belmont-steer-*.md")
	if err != nil {
		return "", fmt.Errorf("steer: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	seed := "# Belmont steer — lines starting with `#` are ignored.\n"
	seed += fmt.Sprintf("# Feature: %s\n", feature)
	if milestone != "" {
		seed += fmt.Sprintf("# Milestone: %s\n", milestone)
	}
	seed += "# Write the instructions for the agent below this comment and save.\n\n"
	if _, err := tmp.WriteString(seed); err != nil {
		tmp.Close()
		return "", fmt.Errorf("steer: seed temp file: %w", err)
	}
	tmp.Close()

	cmd := exec.Command("sh", "-c", fmt.Sprintf("%s %q", editor, tmpPath))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("steer: $EDITOR exited non-zero: %w", err)
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("steer: re-read temp file: %w", err)
	}
	return string(data), nil
}

// appendSteeringEntry appends a pending entry to STEERING.md, creating the
// file (and parent dir) if needed.
func appendSteeringEntry(path, timestamp, milestone, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	var header strings.Builder
	header.WriteString("## ")
	header.WriteString(timestamp)
	if milestone != "" {
		header.WriteString(" [")
		header.WriteString(milestone)
		header.WriteString("]")
	}
	header.WriteString(" (pending)\n")
	header.WriteString(strings.TrimSpace(body))
	header.WriteString("\n\n")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	// Ensure there's a blank line before the new entry when the file was
	// non-empty and didn't end with one.
	info, _ := f.Stat()
	if info != nil && info.Size() > 0 {
		last := make([]byte, 2)
		// Best-effort peek at the last two bytes; ignore errors.
		_, _ = f.Seek(-2, io.SeekEnd)
		_, _ = f.Read(last)
		_, _ = f.Seek(0, io.SeekEnd)
		if !(last[0] == '\n' && last[1] == '\n') {
			if _, err := f.WriteString("\n"); err != nil {
				return err
			}
		}
	}
	_, err = f.WriteString(header.String())
	return err
}


