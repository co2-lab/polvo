// Package doctor provides diagnostic checks for a polvo project.
package doctor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/guide"
	"github.com/co2-lab/polvo/internal/provider"
)

// Diagnosis is a single check result.
type Diagnosis struct {
	Category string
	Label    string
	OK       bool
	Detail   string
	Fix      string
	Fixable  bool // true if FixFn can auto-fix the issue
	FixFn    func() error
}

// Input holds everything the doctor needs to run checks.
type Input struct {
	Cfg        *config.Config
	Registry   *provider.Registry
	Resolver   *guide.Resolver
	Watching   bool
	StateOK    bool // true if state file loaded successfully
	StateFile  string
	ChunkIndex interface{ Count() (int, error) }        // nil = check skipped
	Indexer    interface{ IndexAll(context.Context) error } // nil = fix not available
}

// Run executes all checks and returns the results.
func Run(in Input) []Diagnosis {
	var diags []Diagnosis

	diags = append(diags, checkConfigFile())

	if in.Cfg != nil {
		diags = append(diags, checkProjectName(in.Cfg))
		diags = append(diags, checkProvidersExist(in.Cfg))
		diags = append(diags, checkProviderConnectivity(in.Registry)...)
		diags = append(diags, checkInterfacePatterns(in.Cfg))
		diags = append(diags, checkGuides(in.Resolver))
		diags = append(diags, checkChain(in.Cfg, in.Resolver))
	}

	diags = append(diags, checkPolvoDir())
	diags = append(diags, checkStateFile(in.StateOK, in.StateFile))
	diags = append(diags, checkWatchState(in.Watching, in.StateFile))
	diags = append(diags, checkGuidesDir())
	diags = append(diags, checkChunkIndex(in.ChunkIndex, in.Indexer))

	return diags
}

func checkConfigFile() Diagnosis {
	d := Diagnosis{Category: "config", Label: "polvo.yaml exists"}
	if _, err := os.Stat("polvo.yaml"); err != nil {
		d.Detail = "polvo.yaml not found in current directory"
		d.Fix = "Use the Init button on the welcome screen to create polvo.yaml"
		return d
	}
	d.OK = true
	return d
}

func checkProjectName(cfg *config.Config) Diagnosis {
	d := Diagnosis{Category: "config", Label: "project name"}
	if cfg.Project.Name == "" {
		d.Detail = "project name is empty"
		d.Fix = "Add 'project: name: \"your-project\"' to polvo.yaml"
		return d
	}
	d.OK = true
	return d
}

func checkProvidersExist(cfg *config.Config) Diagnosis {
	d := Diagnosis{Category: "provider", Label: "providers configured"}
	if len(cfg.Providers) == 0 {
		d.Detail = "no providers configured"
		d.Fix = "Add at least one provider to polvo.yaml"
		return d
	}
	d.OK = true
	d.Detail = fmt.Sprintf("%d provider(s)", len(cfg.Providers))
	return d
}

func checkProviderConnectivity(registry *provider.Registry) []Diagnosis {
	if registry == nil {
		return nil
	}
	var diags []Diagnosis
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for name, p := range registry.All() {
		d := Diagnosis{Category: "provider", Label: fmt.Sprintf("provider %q reachable", name)}
		if err := p.Available(ctx); err != nil {
			d.Detail = err.Error()
			switch {
			case strings.Contains(d.Detail, "API key"):
				d.Fix = fmt.Sprintf("Set the API key for provider %q (check env vars in polvo.yaml)", name)
			case strings.Contains(d.Detail, "not available"):
				d.Fix = fmt.Sprintf("Ensure the service for provider %q is running and accessible", name)
			default:
				d.Fix = fmt.Sprintf("Check configuration for provider %q", name)
			}
		} else {
			d.OK = true
		}
		diags = append(diags, d)
	}
	return diags
}

func checkInterfacePatterns(cfg *config.Config) Diagnosis {
	d := Diagnosis{Category: "config", Label: "interface patterns"}
	patterns := cfg.AllInterfacePatterns()
	if len(patterns) == 0 {
		d.Detail = "no interface patterns configured — watch mode won't match any files"
		d.Fix = "Add an interface group to polvo.yaml, e.g. 'interfaces: web-api: patterns: [\"**/*.go\"]'"
		return d
	}
	d.OK = true
	d.Detail = strings.Join(patterns, ", ")
	return d
}

func checkGuides(resolver *guide.Resolver) Diagnosis {
	d := Diagnosis{Category: "guides", Label: "guides resolvable"}
	if resolver == nil {
		d.Detail = "guide resolver not initialized"
		d.Fix = "Ensure polvo.yaml is valid and use the Init button to reload"
		return d
	}
	guides, err := resolver.ResolveAll()
	if err != nil {
		d.Detail = fmt.Sprintf("error resolving guides: %v", err)
		d.Fix = "Check guide file paths and names in polvo.yaml"
		return d
	}
	d.OK = true
	d.Detail = fmt.Sprintf("%d guide(s) available", len(guides))
	return d
}

func checkChain(cfg *config.Config, resolver *guide.Resolver) Diagnosis {
	d := Diagnosis{Category: "config", Label: "chain steps"}
	steps := cfg.Chain.Steps
	if len(steps) == 0 {
		d.OK = true
		d.Detail = "no chain configured (optional)"
		return d
	}
	if resolver == nil {
		d.Detail = "cannot verify chain — resolver not available"
		d.Fix = "Use the Init button to reload config"
		return d
	}
	var missing []string
	for _, step := range steps {
		gcfg := cfg.Guides[step.Agent]
		if _, err := resolver.Resolve(step.Agent, gcfg); err != nil {
			missing = append(missing, step.Agent)
		}
	}
	if len(missing) > 0 {
		d.Detail = fmt.Sprintf("chain references unknown agents: %s", strings.Join(missing, ", "))
		d.Fix = "Add guides for the missing agents or fix agent names in polvo.yaml"
		return d
	}
	d.OK = true
	d.Detail = fmt.Sprintf("%d step(s)", len(steps))
	return d
}

func checkPolvoDir() Diagnosis {
	d := Diagnosis{Category: "filesystem", Label: ".polvo directory"}
	info, err := os.Stat(".polvo")
	if err != nil || !info.IsDir() {
		d.Detail = ".polvo directory does not exist"
		d.Fix = "Create the .polvo directory"
		d.Fixable = true
		d.FixFn = func() error {
			return os.MkdirAll(".polvo/reports", 0755)
		}
		return d
	}
	d.OK = true
	return d
}

func checkStateFile(stateOK bool, stateFile string) Diagnosis {
	d := Diagnosis{Category: "filesystem", Label: "state file"}
	if _, err := os.Stat(stateFile); err != nil {
		d.Detail = "no state file — session state won't persist"
		d.Fix = "State file will be created automatically on next state change"
		d.OK = true
		return d
	}
	if !stateOK {
		d.Detail = "state file exists but failed to load"
		d.Fix = "Delete the corrupted state file and restart"
		d.Fixable = true
		d.FixFn = func() error {
			return os.Remove(stateFile)
		}
		return d
	}
	d.OK = true
	return d
}

func checkWatchState(watching bool, stateFile string) Diagnosis {
	d := Diagnosis{Category: "state", Label: "watch state consistency"}
	// Read watch flag from state file if present
	data, err := os.ReadFile(stateFile)
	if err != nil {
		d.OK = true
		return d
	}
	stateWatchEnabled := strings.Contains(string(data), "watch: true")
	if stateWatchEnabled && !watching {
		d.Detail = "state says watch is enabled but watcher is not running"
		d.Fix = "Run /watch to start watching"
		return d
	}
	d.OK = true
	return d
}

func checkChunkIndex(idx interface{ Count() (int, error) }, ix interface{ IndexAll(context.Context) error }) Diagnosis {
	d := Diagnosis{Category: "index", Label: "chunk index populated"}
	if idx == nil {
		d.OK = true
		d.Detail = "indexer not configured"
		return d
	}
	n, err := idx.Count()
	if err != nil {
		d.Detail = fmt.Sprintf("error querying index: %v", err)
		d.Fix = "Check .polvo/memory.db permissions"
		return d
	}
	if n == 0 {
		d.Detail = "chunk index is empty — search_code tool has no data"
		d.Fix = "Run full reindex"
		if ix != nil {
			d.Fixable = true
			d.FixFn = func() error {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				return ix.IndexAll(ctx)
			}
		}
		return d
	}
	d.OK = true
	d.Detail = fmt.Sprintf("%d chunks indexed", n)
	return d
}

func checkGuidesDir() Diagnosis {
	d := Diagnosis{Category: "filesystem", Label: "guides directory"}
	info, err := os.Stat("guides")
	if err != nil || !info.IsDir() {
		d.Detail = "guides/ directory does not exist — custom guides won't be found"
		d.Fix = "Create the guides directory"
		d.Fixable = true
		d.FixFn = func() error {
			return os.MkdirAll("guides", 0755)
		}
		return d
	}
	d.OK = true
	return d
}
