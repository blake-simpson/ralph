package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// localLLMsConfig is the parsed shape of `local-llms.json`. Top-level keyed by
// tool name so future tools that gain local-endpoint support can be added
// without a schema break. Today only Pi reads from this file.
type localLLMsConfig struct {
	Pi *toolLocalLLMs `json:"pi,omitempty"`
}

// toolLocalLLMs is the per-tool entry. `Tiers` maps "low"/"medium"/"high" to a
// (provider, model) pair. Tools without local-endpoint configurability stay
// nil here.
type toolLocalLLMs struct {
	Tiers map[string]localLLMTier `json:"tiers,omitempty"`
}

// localLLMTier names the provider + model that should drive a given tier. The
// names are passed verbatim to the tool's CLI (`--provider <provider> --model
// <model>` for Pi). Both fields are optional — partial entries fall through to
// the next priority level in the resolution chain.
type localLLMTier struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// userLocalLLMsPath returns the user-level config path
// (~/.belmont/local-llms.json). Lives at ~/.belmont/ rather than
// ~/.config/belmont/ to match the conventional .tool/-in-home pattern (cf.
// .npm, .cargo, .docker) and stay consistent with Belmont's project-level
// .belmont/ directory.
func userLocalLLMsPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".belmont", "local-llms.json")
}

// projectLocalLLMsPath returns the project-level config path. Empty
// projectRoot returns "" so the caller skips the project layer.
func projectLocalLLMsPath(projectRoot string) string {
	if projectRoot == "" {
		return ""
	}
	return filepath.Join(projectRoot, ".belmont", "local-llms.json")
}

// loadLocalLLMs reads .belmont/local-llms.json (project-level) layered on top
// of ~/.belmont/local-llms.json (user-level). Project values override user
// values per-tier-per-field. Returns nil, nil when neither file exists; that's
// the "no config, fall through" path. A malformed file is reported as an
// error rather than silently ignored — easier to diagnose than mysterious
// fallback behaviour.
func loadLocalLLMs(projectRoot string) (*localLLMsConfig, error) {
	user, userErr := readLocalLLMsFile(userLocalLLMsPath())
	if userErr != nil {
		return nil, userErr
	}
	proj, projErr := readLocalLLMsFile(projectLocalLLMsPath(projectRoot))
	if projErr != nil {
		return nil, projErr
	}
	if user == nil && proj == nil {
		return nil, nil
	}
	merged := &localLLMsConfig{}
	if user != nil {
		merged.Pi = clonePiConfig(user.Pi)
	}
	if proj != nil && proj.Pi != nil {
		if merged.Pi == nil {
			merged.Pi = &toolLocalLLMs{}
		}
		mergePiTiers(merged.Pi, proj.Pi)
	}
	return merged, nil
}

func readLocalLLMsFile(path string) (*localLLMsConfig, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	var cfg localLLMsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func clonePiConfig(src *toolLocalLLMs) *toolLocalLLMs {
	if src == nil {
		return nil
	}
	out := &toolLocalLLMs{}
	if src.Tiers != nil {
		out.Tiers = make(map[string]localLLMTier, len(src.Tiers))
		for k, v := range src.Tiers {
			out.Tiers[k] = v
		}
	}
	return out
}

// mergePiTiers overlays src tiers onto dst, field-by-field. A non-empty
// project Provider replaces the user Provider (independently of Model), and
// vice versa, so users can override just one field per tier.
func mergePiTiers(dst, src *toolLocalLLMs) {
	if src == nil || src.Tiers == nil {
		return
	}
	if dst.Tiers == nil {
		dst.Tiers = map[string]localLLMTier{}
	}
	for tier, srcTier := range src.Tiers {
		merged := dst.Tiers[tier]
		if srcTier.Provider != "" {
			merged.Provider = srcTier.Provider
		}
		if srcTier.Model != "" {
			merged.Model = srcTier.Model
		}
		dst.Tiers[tier] = merged
	}
}

// resolvePiModelFlags returns the `--provider X --model Y` flag pair to pass
// to `pi -p`, applying this priority order (highest first):
//
//  1. BELMONT_PI_PROVIDER_<TIER> + BELMONT_PI_MODEL_<TIER> env vars
//     (per-tier override, e.g. BELMONT_PI_MODEL_HIGH)
//  2. BELMONT_PI_PROVIDER + BELMONT_PI_MODEL env vars (single value applied
//     to every tier)
//  3. .belmont/local-llms.json `pi.tiers.<tier>` (project-level override)
//  4. ~/.belmont/local-llms.json `pi.tiers.<tier>` (user-level)
//  5. nothing — Pi uses its own default model
//
// Provider and Model are independent: each falls through the chain
// separately, so a user can set BELMONT_PI_MODEL_HIGH without touching
// provider, or define provider in the user file and model in the project
// file. Returns an empty slice (NOT nil) when no source produces any flag —
// callers append the result to argv unconditionally.
func resolvePiModelFlags(projectRoot, tier string) []string {
	provider, model := resolvePiTierValues(projectRoot, tier)
	var flags []string
	if provider != "" {
		flags = append(flags, "--provider", provider)
	}
	if model != "" {
		flags = append(flags, "--model", model)
	}
	if flags == nil {
		return []string{}
	}
	return flags
}

func resolvePiTierValues(projectRoot, tier string) (provider, model string) {
	tierUpper := strings.ToUpper(strings.TrimSpace(tier))

	if tierUpper != "" {
		provider = os.Getenv("BELMONT_PI_PROVIDER_" + tierUpper)
		model = os.Getenv("BELMONT_PI_MODEL_" + tierUpper)
	}
	if provider == "" {
		provider = os.Getenv("BELMONT_PI_PROVIDER")
	}
	if model == "" {
		model = os.Getenv("BELMONT_PI_MODEL")
	}
	if provider != "" && model != "" {
		return provider, model
	}

	cfg, err := loadLocalLLMs(projectRoot)
	if err != nil || cfg == nil || cfg.Pi == nil {
		return provider, model
	}
	tierKey := strings.ToLower(strings.TrimSpace(tier))
	entry, ok := cfg.Pi.Tiers[tierKey]
	if !ok {
		return provider, model
	}
	if provider == "" {
		provider = entry.Provider
	}
	if model == "" {
		model = entry.Model
	}
	return provider, model
}
