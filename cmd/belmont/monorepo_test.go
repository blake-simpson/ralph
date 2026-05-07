package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// writeFile is a tiny helper used by tests in this file.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

// workspaceIDs returns a sorted slice of workspace IDs.
func workspaceIDs(ws []workspaceInfo) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.ID
	}
	sort.Strings(out)
	return out
}

func TestDetectWorkspaces_SinglePackageReturnsNil(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"single","version":"1.0.0"}`)
	ws, mType := detectWorkspaces(dir)
	if mType != monorepoNone || len(ws) != 0 {
		t.Errorf("expected single-package early-return, got type=%q workspaces=%v", mType, workspaceIDs(ws))
	}
}

func TestDetectWorkspaces_NpmWorkspacesArray(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"root","workspaces":["packages/*"]}`)
	writeFile(t, dir, "package-lock.json", `{}`)
	writeFile(t, dir, "packages/web/package.json", `{"name":"@org/web","scripts":{"dev":"next dev"}}`)
	writeFile(t, dir, "packages/api/package.json", `{"name":"@org/api"}`)

	ws, mType := detectWorkspaces(dir)
	if mType != monorepoNpm {
		t.Errorf("type = %q, want %q", mType, monorepoNpm)
	}
	got := workspaceIDs(ws)
	want := []string{"@org/api", "@org/web"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("workspaces = %v, want %v", got, want)
	}
}

func TestDetectWorkspaces_PnpmWorkspaceYaml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"root"}`)
	writeFile(t, dir, "pnpm-workspace.yaml", "packages:\n  - 'packages/*'\n  - apps/*\n")
	writeFile(t, dir, "packages/web/package.json", `{"name":"web"}`)
	writeFile(t, dir, "apps/api/package.json", `{"name":"api"}`)

	ws, mType := detectWorkspaces(dir)
	if mType != monorepoPnpm {
		t.Errorf("type = %q, want %q", mType, monorepoPnpm)
	}
	got := workspaceIDs(ws)
	if len(got) != 2 {
		t.Errorf("expected 2 workspaces, got %v", got)
	}
}

func TestDetectWorkspaces_TurborepoOverridesPnpm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"root"}`)
	writeFile(t, dir, "pnpm-workspace.yaml", "packages:\n  - packages/*\n")
	writeFile(t, dir, "turbo.json", `{"$schema":"https://turbo.build/schema.json","tasks":{}}`)
	writeFile(t, dir, "packages/web/package.json", `{"name":"web"}`)

	_, mType := detectWorkspaces(dir)
	if mType != monorepoTurborepo {
		t.Errorf("type = %q, want %q (turbo.json should win over pnpm-workspace.yaml)", mType, monorepoTurborepo)
	}
}

func TestDetectWorkspaces_CargoWorkspace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", "[workspace]\nmembers = [\"crates/*\"]\nresolver = \"2\"\n")
	writeFile(t, dir, "crates/cli/Cargo.toml", "[package]\nname = \"cli\"\nversion = \"0.1.0\"\n")
	writeFile(t, dir, "crates/lib/Cargo.toml", "[package]\nname = \"lib\"\nversion = \"0.1.0\"\n")

	ws, mType := detectWorkspaces(dir)
	if mType != monorepoCargo {
		t.Errorf("type = %q, want %q", mType, monorepoCargo)
	}
	got := workspaceIDs(ws)
	if len(got) != 2 || got[0] != "cli" || got[1] != "lib" {
		t.Errorf("workspaces = %v, want [cli lib]", got)
	}
}

func TestDetectWorkspaces_CargoWorkspaceWithBuildRsSignal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", "[workspace]\nmembers = [\"crates/*\"]\n")
	writeFile(t, dir, "crates/svc/Cargo.toml", "[package]\nname = \"svc\"\n")
	writeFile(t, dir, "crates/svc/build.rs", "fn main() {}\n")
	writeFile(t, dir, "crates/lib/Cargo.toml", "[package]\nname = \"lib\"\n")

	ws, _ := detectWorkspaces(dir)
	for _, w := range ws {
		if w.ID == "svc" && !w.Signals.BuildRs {
			t.Errorf("svc workspace should have BuildRs signal")
		}
		if w.ID == "lib" && w.Signals.BuildRs {
			t.Errorf("lib workspace should not have BuildRs signal")
		}
	}
}

func TestDetectWorkspaces_GoWork(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.work", "go 1.22\n\nuse (\n    ./svc-a\n    ./svc-b\n)\n")
	writeFile(t, dir, "svc-a/go.mod", "module example.com/svc-a\n\ngo 1.22\n")
	writeFile(t, dir, "svc-b/go.mod", "module example.com/svc-b\n\ngo 1.22\n")

	ws, mType := detectWorkspaces(dir)
	if mType != monorepoGo {
		t.Errorf("type = %q, want %q", mType, monorepoGo)
	}
	got := workspaceIDs(ws)
	if len(got) != 2 {
		t.Errorf("expected 2 workspaces, got %v", got)
	}
}

func TestDetectWorkspaces_UvWorkspace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml",
		"[project]\nname = \"root\"\nversion = \"0.1\"\n\n[tool.uv.workspace]\nmembers = [\"packages/*\"]\n")
	writeFile(t, dir, "packages/svc-a/pyproject.toml",
		"[project]\nname = \"svc_a\"\nversion = \"0.1\"\n\n[project.scripts]\nsvc-a = \"svc_a:main\"\n")
	writeFile(t, dir, "packages/svc-b/pyproject.toml",
		"[project]\nname = \"svc_b\"\nversion = \"0.1\"\n")

	ws, mType := detectWorkspaces(dir)
	if mType != monorepoUv {
		t.Errorf("type = %q, want %q", mType, monorepoUv)
	}
	if len(ws) != 2 {
		t.Errorf("expected 2 workspaces, got %v", workspaceIDs(ws))
	}
	for _, w := range ws {
		if w.ID == "svc_a" && !w.Signals.PythonScripts {
			t.Errorf("svc_a should have PythonScripts signal")
		}
	}
}

func TestJsManifestSignals_PrismaPostinstall(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "package.json")
	if err := os.WriteFile(manifest,
		[]byte(`{"name":"web","scripts":{"dev":"next dev","postinstall":"prisma generate"},"dependencies":{"@prisma/client":"5.0.0"},"devDependencies":{"prisma":"5.0.0"}}`),
		0o644); err != nil {
		t.Fatal(err)
	}
	sig, hasDev := jsManifestSignals(manifest)
	if !sig.Postinstall {
		t.Errorf("expected Postinstall signal")
	}
	if !sig.PrismaDep {
		t.Errorf("expected PrismaDep signal")
	}
	if !hasDev {
		t.Errorf("expected hasDev = true")
	}
	if !sig.consumesEnv() {
		t.Errorf("expected consumesEnv = true")
	}
}

func TestJsManifestSignals_NoSignals(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "package.json")
	if err := os.WriteFile(manifest,
		[]byte(`{"name":"types","version":"1.0.0"}`),
		0o644); err != nil {
		t.Fatal(err)
	}
	sig, hasDev := jsManifestSignals(manifest)
	if sig.consumesEnv() {
		t.Errorf("types-only workspace should not consume env, got %+v", sig)
	}
	if hasDev {
		t.Errorf("types-only workspace should not have dev script")
	}
}

func TestPickPrimary_PrefersDevScript(t *testing.T) {
	ws := []workspaceInfo{
		{ID: "lib", HasDev: false},
		{ID: "web", HasDev: true},
		{ID: "api", HasDev: true},
	}
	got := pickPrimary(ws, "")
	if got != "web" {
		t.Errorf("pickPrimary = %q, want \"web\" (first with dev script)", got)
	}
}

func TestPickPrimary_HonorsExplicitOverride(t *testing.T) {
	ws := []workspaceInfo{
		{ID: "lib"}, {ID: "web", HasDev: true}, {ID: "api", HasDev: true},
	}
	if got := pickPrimary(ws, "api"); got != "api" {
		t.Errorf("pickPrimary with override 'api' = %q, want \"api\"", got)
	}
	// Override that doesn't match any workspace falls back to first-with-dev
	if got := pickPrimary(ws, "nope"); got != "web" {
		t.Errorf("pickPrimary with bogus override = %q, want fallback \"web\"", got)
	}
}

func TestResolveWorkspaces_ExplicitOverridesAutoDetect(t *testing.T) {
	dir := t.TempDir()
	// Auto-detect would find this:
	writeFile(t, dir, "package.json", `{"name":"root","workspaces":["packages/*"]}`)
	writeFile(t, dir, "package-lock.json", `{}`)
	writeFile(t, dir, "packages/auto-only/package.json", `{"name":"auto-only"}`)

	// But hooks override declares a different layout:
	hooks := &worktreeHooks{
		PrimaryWorkspace: "explicit",
		Workspaces: map[string]workspaceOverride{
			"explicit": {Path: "packages/auto-only"},
		},
	}
	ws, primary, mType := resolveWorkspaces(dir, hooks)
	if primary != "explicit" {
		t.Errorf("primary = %q, want \"explicit\"", primary)
	}
	if len(ws) != 1 || ws[0].ID != "explicit" {
		t.Errorf("workspaces = %v, want one entry with ID 'explicit'", workspaceIDs(ws))
	}
	if mType != monorepoNpm {
		t.Errorf("type = %q, want %q (still reports detected type for context)", mType, monorepoNpm)
	}
}

func TestMonorepoEnvVars_EmptyForSinglePackage(t *testing.T) {
	got := monorepoEnvVars(nil, "", monorepoNone)
	if got != nil {
		t.Errorf("expected nil env vars for single-package, got %v", got)
	}
}

func TestMonorepoEnvVars_FullSet(t *testing.T) {
	ws := []workspaceInfo{
		{ID: "web", Path: "packages/web"},
		{ID: "api", Path: "apps/api"},
	}
	got := monorepoEnvVars(ws, "web", monorepoTurborepo)
	wantContains := []string{
		"BELMONT_MONOREPO=1",
		"BELMONT_MONOREPO_TYPE=turborepo",
		"BELMONT_PRIMARY_WORKSPACE=web",
		"BELMONT_PRIMARY_WORKSPACE_PATH=packages/web",
	}
	for _, want := range wantContains {
		found := false
		for _, v := range got {
			if v == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in env, got %v", want, got)
		}
	}
	// BELMONT_WORKSPACES should be a JSON array containing both entries
	jsonFound := false
	for _, v := range got {
		if len(v) > len("BELMONT_WORKSPACES=") && v[:len("BELMONT_WORKSPACES=")] == "BELMONT_WORKSPACES=" {
			jsonFound = true
			body := v[len("BELMONT_WORKSPACES="):]
			if !contains(body, "\"web\"") || !contains(body, "\"api\"") {
				t.Errorf("BELMONT_WORKSPACES JSON missing workspaces, got %q", body)
			}
		}
	}
	if !jsonFound {
		t.Errorf("BELMONT_WORKSPACES not found in env")
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSeedWorkspaceEnv_SkipsPureCodeWorkspaces(t *testing.T) {
	root := t.TempDir()
	wt := t.TempDir()

	// Pure code workspace — no env signals.
	wsDir := filepath.Join(wt, "packages/types")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, ".env", "DATABASE_URL=postgres://localhost\n")

	ws := workspaceInfo{
		ID:      "types",
		Path:    "packages/types",
		Signals: envSignals{}, // no signals
	}
	seedWorkspaceEnv(root, wt, ws, workspaceOverride{}, []string{".env"})

	if _, err := os.Stat(filepath.Join(wsDir, ".env")); err == nil {
		t.Errorf("seedWorkspaceEnv should not have copied .env into pure-code workspace")
	}
}

func TestSeedWorkspaceEnv_CopiesIntoPrismaWorkspace(t *testing.T) {
	root := t.TempDir()
	wt := t.TempDir()

	wsDir := filepath.Join(wt, "packages/web")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, ".env", "DATABASE_URL=postgres://localhost\n")
	writeFile(t, root, ".env.local", "FOO=bar\n")

	ws := workspaceInfo{
		ID:      "web",
		Path:    "packages/web",
		Signals: envSignals{PrismaDep: true, Postinstall: true},
	}
	// Pass both .env and .env.local as root env files (mimicking copyEnvFiles' behavior)
	seedWorkspaceEnv(root, wt, ws, workspaceOverride{}, []string{".env", ".env.local"})

	for _, name := range []string{".env", ".env.local"} {
		if _, err := os.Stat(filepath.Join(wsDir, name)); err != nil {
			t.Errorf("expected %s to be copied into workspace dir, got: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(wsDir, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "DATABASE_URL=postgres://localhost\n" {
		t.Errorf("seeded .env content mismatch: %q", data)
	}
}

func TestSeedWorkspaceEnv_HonorsExplicitEnvFiles(t *testing.T) {
	root := t.TempDir()
	wt := t.TempDir()

	wsDir := filepath.Join(wt, "apps/api")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No root .env; user lists an explicit alternate path.
	writeFile(t, root, "configs/api.env", "API_KEY=secret\n")

	ws := workspaceInfo{
		ID:      "api",
		Path:    "apps/api",
		Signals: envSignals{}, // no auto signals
	}
	override := workspaceOverride{
		EnvFiles: []string{"configs/api.env"},
	}
	seedWorkspaceEnv(root, wt, ws, override, nil)

	// File lands as basename in the workspace dir
	if _, err := os.Stat(filepath.Join(wsDir, "api.env")); err != nil {
		t.Errorf("expected configs/api.env to be copied into workspace as api.env, got: %v", err)
	}
}

func TestExpandWorkspaceGlob_LiteralAndStar(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "packages/web/package.json", `{"name":"web"}`)
	writeFile(t, dir, "packages/api/package.json", `{"name":"api"}`)
	writeFile(t, dir, "apps/site/package.json", `{"name":"site"}`)

	got := expandWorkspaceGlob(dir, "packages/*", "package.json")
	sort.Strings(got)
	want := []string{"packages/api", "packages/web"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("expandWorkspaceGlob = %v, want %v", got, want)
	}

	got = expandWorkspaceGlob(dir, "apps/site", "package.json")
	if len(got) != 1 || got[0] != "apps/site" {
		t.Errorf("literal glob = %v, want [apps/site]", got)
	}
}

func TestParseCargoWorkspaces_TolerantToExtraSections(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml",
		"[workspace]\nmembers = [\"crates/a\", \"crates/b\"]\nresolver = \"2\"\n\n"+
			"[workspace.package]\nedition = \"2021\"\n\n"+
			"[workspace.dependencies]\nserde = \"1\"\n")
	writeFile(t, dir, "crates/a/Cargo.toml", "[package]\nname = \"a\"\n")
	writeFile(t, dir, "crates/b/Cargo.toml", "[package]\nname = \"b\"\n")
	ws, ok := parseCargoWorkspaces(dir)
	if !ok || len(ws) != 2 {
		t.Errorf("expected 2 workspaces, got %v ok=%v", workspaceIDs(ws), ok)
	}
}
