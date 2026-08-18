// Package profile manages versioned docc profile packs and resolves the
// schemas and themes a document uses.
package profile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/kevinzehnder/docc/internal/defaultpack"
	"github.com/kevinzehnder/docc/internal/docx"
	"github.com/kevinzehnder/docc/internal/emit"
	"github.com/kevinzehnder/docc/internal/project"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/starter"
	"github.com/kevinzehnder/docc/internal/theme"
)

const (
	manifestName = "docc-profile.yaml"
	bindingName  = "profile.yaml"
	lockName     = "profile.lock"
	configName   = "config.yaml"
	format       = 1
)

var (
	// ErrNotConfigured means neither a project configuration nor a user default
	// could resolve a profile pack.
	ErrNotConfigured = errors.New("profile not configured")
	idPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	commitPattern    = regexp.MustCompile(`^[0-9a-f]{40,64}$`)
)

// Paths contains docc's XDG base directories. Profile repositories are data,
// not configuration, and consequently live under Data.
type Paths struct {
	Config string
	Data   string
	Cache  string
}

// XDGPaths returns the user-specific directories docc uses. XDG values must be
// absolute; a relative value is ignored as required by the specification.
func XDGPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("find home directory: %w", err)
	}
	return xdgPaths(home, os.Getenv), nil
}

func xdgPaths(home string, getenv func(string) string) Paths {
	return Paths{
		Config: xdgBase(getenv("XDG_CONFIG_HOME"), filepath.Join(home, ".config")),
		Data:   xdgBase(getenv("XDG_DATA_HOME"), filepath.Join(home, ".local", "share")),
		Cache:  xdgBase(getenv("XDG_CACHE_HOME"), filepath.Join(home, ".cache")),
	}
}

func xdgBase(value, fallback string) string {
	if filepath.IsAbs(value) {
		return value
	}
	return fallback
}

// PackDir returns the immutable local directory for one installed revision.
func (p Paths) PackDir(id, commit string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("invalid profile id %q", id)
	}
	if !commitPattern.MatchString(commit) {
		return "", fmt.Errorf("invalid profile commit %q", commit)
	}
	return filepath.Join(p.Data, "docc", "profiles", id, commit), nil
}

func (p Paths) configPath() string { return filepath.Join(p.Config, "docc", configName) }

// Manifest identifies a profile pack and its schema/theme directories.
type Manifest struct {
	Format  int    `yaml:"format"`
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Schemas string `yaml:"schemas"`
	Themes  string `yaml:"themes"`
}

// Pack is a validated local profile revision.
type Pack struct {
	Root     string
	Manifest Manifest
}

// LoadPack reads and validates a profile-pack manifest and its declared paths.
func LoadPack(root string) (*Pack, error) {
	path := filepath.Join(root, manifestName)
	data, err := os.ReadFile(path) //nolint:gosec // root is a selected local pack directory
	if err != nil {
		return nil, fmt.Errorf("read profile manifest %s: %w", path, err)
	}
	var manifest Manifest
	if err := yaml.UnmarshalWithOptions(data, &manifest, yaml.Strict()); err != nil {
		return nil, fmt.Errorf("parse profile manifest %s: %w", path, err)
	}
	if manifest.Format != format {
		return nil, fmt.Errorf("%s: unsupported format %d (supported: %d)", path, manifest.Format, format)
	}
	if !idPattern.MatchString(manifest.ID) {
		return nil, fmt.Errorf("%s: id must match %s", path, idPattern)
	}
	for _, entry := range []struct {
		name  string
		value string
	}{{"schemas", manifest.Schemas}, {"themes", manifest.Themes}} {
		name, value := entry.name, entry.value
		if value == "" {
			return nil, fmt.Errorf("%s: %s is required", path, name)
		}
		if filepath.IsAbs(value) || value == "." || strings.HasPrefix(filepath.Clean(value), ".."+string(filepath.Separator)) || filepath.Clean(value) == ".." {
			return nil, fmt.Errorf("%s: %s must be a relative path inside the pack", path, name)
		}
		// The path is checked above: relative, and inside a pack root the
		// caller chose.
		info, err := os.Stat(filepath.Join(root, value)) //nolint:gosec // validated relative path inside the pack
		if err != nil {
			return nil, fmt.Errorf("%s: %s directory: %w", path, name, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s: %s path is not a directory", path, name)
		}
	}
	return &Pack{Root: root, Manifest: manifest}, nil
}

// SchemaDir returns this pack's declared schema directory.
func (p *Pack) SchemaDir() string { return filepath.Join(p.Root, p.Manifest.Schemas) }

// ThemeDir returns this pack's declared theme directory.
func (p *Pack) ThemeDir() string { return filepath.Join(p.Root, p.Manifest.Themes) }

// Validate loads the pack's schemas and themes and checks every renderable
// schema/theme pair before a revision becomes available to a project.
func (p *Pack) Validate() error {
	schemas, err := schema.Load(p.SchemaDir())
	if err != nil {
		return err
	}
	themes, err := theme.Load(p.ThemeDir())
	if err != nil {
		return err
	}
	_, err = checkPairs(schemas, themes)
	return err
}

// checkPairs validates every renderable schema/theme pair and returns the
// sorted renderable type names.
func checkPairs(schemas *schema.Set, themes *theme.Set) ([]string, error) {
	var renderable []string
	for _, typ := range schemas.Types() {
		sc, err := schemas.Get(typ)
		if err != nil {
			return nil, err
		}
		if sc.Theme == "" {
			continue
		}
		th, err := themes.Get(sc.Theme)
		if err != nil {
			return nil, fmt.Errorf("schema %q: %w", sc.Type, err)
		}
		if err := emit.Validate(sc, th); err != nil {
			return nil, fmt.Errorf("schema %q and theme %q: %w", sc.Type, th.Name, err)
		}
		renderable = append(renderable, typ)
	}
	sort.Strings(renderable)
	return renderable, nil
}

// Reference is a Git source plus the exact revision currently selected.
type Reference struct {
	ID     string `yaml:"id" json:"id"`
	Source string `yaml:"source" json:"source"`
	Ref    string `yaml:"ref,omitempty" json:"ref,omitempty"`
	Commit string `yaml:"commit" json:"commit"`
}

func (r Reference) validate(requireCommit bool) error {
	if !idPattern.MatchString(r.ID) {
		return fmt.Errorf("invalid profile id %q", r.ID)
	}
	if strings.TrimSpace(r.Source) == "" {
		return errors.New("profile source is required")
	}
	if strings.HasPrefix(r.Ref, "-") {
		return fmt.Errorf("invalid profile ref %q", r.Ref)
	}
	if requireCommit && !commitPattern.MatchString(r.Commit) {
		return fmt.Errorf("invalid profile commit %q", r.Commit)
	}
	return nil
}

// Binding is the tracked part of a project profile selection.
type Binding struct {
	Format int    `yaml:"format"`
	ID     string `yaml:"id"`
	Source string `yaml:"source"`
	Ref    string `yaml:"ref,omitempty"`
}

// Lock is the immutable part of a project profile selection.
type Lock struct {
	Format int    `yaml:"format"`
	Commit string `yaml:"commit"`
}

// ReadBinding reads a project's tracked profile selection.
func ReadBinding(dir string) (Binding, error) {
	var binding Binding
	path := filepath.Join(dir, bindingName)
	data, err := os.ReadFile(path) //nolint:gosec // dir was found as the selected project .docc directory
	if errors.Is(err, os.ErrNotExist) {
		return binding, ErrNotConfigured
	}
	if err != nil {
		return binding, fmt.Errorf("read profile binding %s: %w", path, err)
	}
	if err := yaml.UnmarshalWithOptions(data, &binding, yaml.Strict()); err != nil {
		return binding, fmt.Errorf("parse profile binding %s: %w", path, err)
	}
	if binding.Format != format {
		return binding, fmt.Errorf("%s: unsupported format %d (supported: %d)", path, binding.Format, format)
	}
	if err := (Reference{ID: binding.ID, Source: binding.Source, Ref: binding.Ref}).validate(false); err != nil {
		return binding, fmt.Errorf("%s: %w", path, err)
	}
	return binding, nil
}

// ReadLock reads a project's exact profile revision.
func ReadLock(dir string) (Lock, error) {
	var lock Lock
	path := filepath.Join(dir, lockName)
	data, err := os.ReadFile(path) //nolint:gosec // dir was found as the selected project .docc directory
	if err != nil {
		return lock, fmt.Errorf("read profile lock %s: %w", path, err)
	}
	if err := yaml.UnmarshalWithOptions(data, &lock, yaml.Strict()); err != nil {
		return lock, fmt.Errorf("parse profile lock %s: %w", path, err)
	}
	if lock.Format != format || !commitPattern.MatchString(lock.Commit) {
		return lock, fmt.Errorf("%s: expected format %d and a Git commit", path, format)
	}
	return lock, nil
}

// WriteProject creates a managed profile binding. It refuses to sit beside the
// legacy directories: merging two packs would make type and theme names
// ambiguous.
func WriteProject(root string, ref Reference) error {
	if err := ref.validate(true); err != nil {
		return err
	}
	dir := filepath.Join(root, project.DirName)
	for _, name := range []string{"schemas", "themes"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			return fmt.Errorf("refusing to add a profile binding beside existing %s/%s", project.DirName, name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect legacy configuration: %w", err)
		}
	}
	for _, name := range []string{bindingName, lockName} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			return fmt.Errorf("refusing to overwrite existing %s/%s", project.DirName, name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect profile binding: %w", err)
		}
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create project configuration: %w", err)
	}
	// A lock without a binding is inert; write it first so a failure never
	// activates a binding with no exact revision.
	lockPath := filepath.Join(dir, lockName)
	if err := writeYAML(lockPath, Lock{Format: format, Commit: ref.Commit}, 0o644); err != nil {
		return err
	}
	if err := writeYAML(filepath.Join(dir, bindingName), Binding{Format: format, ID: ref.ID, Source: ref.Source, Ref: ref.Ref}, 0o644); err != nil {
		_ = os.Remove(lockPath)
		return err
	}
	return nil
}

func writeLock(dir string, commit string) error {
	if !commitPattern.MatchString(commit) {
		return fmt.Errorf("invalid profile commit %q", commit)
	}
	return writeYAML(filepath.Join(dir, lockName), Lock{Format: format, Commit: commit}, 0o644)
}

// UserConfig stores the optional profile used outside a bound project, and the
// machine's trust policy for installing revisions.
type UserConfig struct {
	Format  int        `yaml:"format"`
	Default *Reference `yaml:"default,omitempty"`
	// Policy applies to every installation on this machine. It belongs here
	// rather than in a project binding because it is an operator's decision,
	// deployed with the rest of a workstation's configuration, and a repository
	// must not be able to lower the bar it is checked against.
	Policy *Policy `yaml:"policy,omitempty"`
}

func readUserConfig(paths Paths) (UserConfig, error) {
	var cfg UserConfig
	data, err := os.ReadFile(paths.configPath())
	if errors.Is(err, os.ErrNotExist) {
		return cfg, ErrNotConfigured
	}
	if err != nil {
		return cfg, fmt.Errorf("read user profile configuration: %w", err)
	}
	if err := yaml.UnmarshalWithOptions(data, &cfg, yaml.Strict()); err != nil {
		return cfg, fmt.Errorf("parse user profile configuration: %w", err)
	}
	if cfg.Format != format {
		return cfg, fmt.Errorf("%s: expected format %d", paths.configPath(), format)
	}
	// A configuration may carry a policy and no default profile: a managed
	// workstation states its trust requirement before anyone selects a pack.
	if cfg.Default != nil {
		if err := cfg.Default.validate(true); err != nil {
			return cfg, fmt.Errorf("%s: %w", paths.configPath(), err)
		}
	}
	return cfg, nil
}

// requireDefault reads the configuration and insists it names a profile.
func requireDefault(paths Paths) (UserConfig, error) {
	cfg, err := readUserConfig(paths)
	if err != nil {
		return cfg, err
	}
	if cfg.Default == nil {
		return cfg, fmt.Errorf("%w: %s names no default profile", ErrNotConfigured, paths.configPath())
	}
	return cfg, nil
}

// TrustPolicy reports the machine's configured trust policy. A machine with no
// configuration has no policy, which is the documented default and not a
// silent one: nothing is verified unless someone asked for it.
func TrustPolicy(paths Paths) (Policy, error) {
	cfg, err := readUserConfig(paths)
	if errors.Is(err, ErrNotConfigured) {
		return Policy{}, nil
	}
	if err != nil {
		return Policy{}, err
	}
	if cfg.Policy == nil {
		return Policy{}, nil
	}
	return *cfg.Policy, nil
}

// writeUserConfig records a default profile without disturbing the policy
// beside it. Selecting a profile must never quietly relax the machine's trust
// requirement.
func writeUserConfig(paths Paths, ref Reference) error {
	if err := ref.validate(true); err != nil {
		return err
	}
	cfg, err := readUserConfig(paths)
	if err != nil && !errors.Is(err, ErrNotConfigured) {
		return err
	}
	cfg.Format = format
	cfg.Default = &ref
	return writeYAML(paths.configPath(), cfg, 0o600)
}

func writeYAML(path string, value any, mode os.FileMode) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	committed = true
	return nil
}

// Resolved is the schema/theme source selected for an invocation.
type Resolved struct {
	Root      string `json:"root,omitempty"`
	SchemaDir string `json:"schema_dir"`
	ThemeDir  string `json:"theme_dir"`
	// Source is env-profile, project-profile, pack-checkout, user-default or
	// builtin.
	Source    string     `json:"source"`
	Reference *Reference `json:"reference,omitempty"`
	// PackRoot is the pack directory the schemas and themes came from.
	PackRoot string `json:"pack_root,omitempty"`
	// PackID is that pack's declared id. It names what was resolved even when
	// nothing pinned it — a checkout has no Reference, but it still has an
	// identity.
	PackID string `json:"pack_id,omitempty"`
}

// fromPack describes a resolution that came from a pack manifest.
func fromPack(pack *Pack, source string, ref *Reference) *Resolved {
	return &Resolved{
		Root:      pack.Root,
		SchemaDir: pack.SchemaDir(),
		ThemeDir:  pack.ThemeDir(),
		Source:    source,
		Reference: ref,
		PackRoot:  pack.Root,
		PackID:    pack.Manifest.ID,
	}
}

// Provenance describes the configuration that produced a document, for the
// output's custom properties. It records what the build was pinned to, never
// a filesystem path: the answer must mean the same thing on another machine,
// and a home directory in a filed document is nobody's business.
func (r *Resolved) Provenance() []docx.CustomProperty {
	if r == nil {
		return nil
	}
	props := []docx.CustomProperty{{Name: "docc-config", Value: r.Source}}
	if r.Reference == nil {
		return props
	}
	props = append(props,
		docx.CustomProperty{Name: "docc-profile", Value: r.Reference.ID},
		docx.CustomProperty{Name: "docc-profile-source", Value: r.Reference.Source},
	)
	if r.Reference.Ref != "" {
		props = append(props, docx.CustomProperty{Name: "docc-profile-ref", Value: r.Reference.Ref})
	}
	return append(props, docx.CustomProperty{Name: "docc-profile-commit", Value: r.Reference.Commit})
}

// FindPack walks up from start looking for a pack manifest, the way
// project.Resolve looks for .docc. start may be a file or a directory.
//
// It is what makes a pack repository usable from inside itself. A pack has no
// .docc — its schemas and themes are the product, not one project's local
// configuration — so authoring or checking anything in a pack checkout meant
// passing --schema-dir and --theme-dir to every command. The manifest is
// already there, already names both directories, and is already validated, so
// nothing new has to be written down for this to work.
//
// A manifest that fails to load is an error rather than a miss: the file says
// this is a pack, and walking past a broken one would silently resolve some
// other configuration.
func FindPack(start string) (*Pack, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}

	for dir := abs; ; {
		if _, err := os.Stat(filepath.Join(dir, manifestName)); err == nil {
			return LoadPack(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil, fmt.Errorf("no %s found in %s or any parent: %w", manifestName, abs, ErrNotFound)
}

// ErrNotFound signals that no pack manifest exists above the start path.
var ErrNotFound = errors.New("profile pack not found")

// EnvProfile is the environment variable naming a pack directory to use. It is
// what a host sets when the documents it compiles live nowhere near the pack,
// so neither walking up from the document nor the working directory can find
// it.
const EnvProfile = "DOCC_PROFILE"

// Resolve finds a pack named by the environment, a project binding, a pack
// checkout, the user default, or — when nothing is configured — the starter
// pack embedded in the binary, in that order. It never fetches or updates a
// profile.
func Resolve(start string, paths Paths) (*Resolved, error) {
	// The environment comes first because setting it is a deliberate act by
	// whoever runs docc, second only to passing the directories outright.
	if dir := os.Getenv(EnvProfile); dir != "" {
		pack, err := LoadPack(dir)
		if err != nil {
			return nil, fmt.Errorf("%s=%s: %w", EnvProfile, dir, err)
		}
		return fromPack(pack, "env-profile", nil), nil
	}

	if proj, err := project.Resolve(start); err == nil {
		binding, bindErr := ReadBinding(proj.Dir)
		switch {
		case bindErr == nil:
			lock, err := ReadLock(proj.Dir)
			if err != nil {
				return nil, err
			}
			ref := Reference{ID: binding.ID, Source: binding.Source, Ref: binding.Ref, Commit: lock.Commit}
			pack, err := loadInstalled(paths, ref)
			if err != nil {
				return nil, err
			}
			resolved := fromPack(pack, "project-profile", &ref)
			// The project is the root a person recognises; the pack lives in
			// the immutable install directory.
			resolved.Root = proj.Root
			return resolved, nil
		case !errors.Is(bindErr, ErrNotConfigured):
			return nil, bindErr
		}
		// The legacy layout — bare schemas/themes under .docc, no manifest —
		// is no longer resolved. Failing loudly beats silently compiling the
		// document against the builtin starter pack instead of the schemas
		// sitting right there.
		for _, dir := range []string{proj.SchemaDir(), proj.ThemeDir()} {
			if _, err := os.Stat(dir); err == nil {
				return nil, fmt.Errorf("%s uses the removed legacy layout (.docc/schemas): move the schemas and themes up beside a docc-profile.yaml, or bind a pack with `docc profile use`", proj.Dir)
			}
		}
	} else if !errors.Is(err, project.ErrNotFound) {
		return nil, err
	}

	// Standing inside a pack checkout. It comes after the project forms —
	// a .docc binding is an explicit statement about which profile applies,
	// and a pack that happens to sit above it does not override that — and
	// before the user default, because the pack you are working in is a more
	// specific answer than the one you installed globally.
	if pack, err := FindPack(start); err == nil {
		return fromPack(pack, "pack-checkout", nil), nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	cfg, err := requireDefault(paths)
	switch {
	case err == nil:
		pack, err := loadInstalled(paths, *cfg.Default)
		if err != nil {
			return nil, err
		}
		resolved := fromPack(pack, "user-default", cfg.Default)
		resolved.Root = "" // an installed pack is nobody's project root
		return resolved, nil
	case errors.Is(err, ErrNotConfigured):
		// Nothing is configured anywhere: fall back to the starter pack
		// embedded in the binary, so docc works out of the box.
		pack, err := builtinPack(paths)
		if err != nil {
			return nil, err
		}
		resolved := fromPack(pack, "builtin", nil)
		resolved.Root = "" // the embedded pack is nobody's project root
		return resolved, nil
	default:
		return nil, err
	}
}

// builtinPack materializes the embedded starter pack into the profile store
// and loads it like any installed revision. The directory is content-addressed
// by the pack's hash, so a new docc release with changed starter content lands
// in a fresh directory and an unchanged one reuses the old extraction.
func builtinPack(paths Paths) (*Pack, error) {
	dir, err := paths.PackDir(defaultpack.ID, defaultpack.Hash())
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		if err := extractBuiltin(dir); err != nil {
			return nil, fmt.Errorf("materialize builtin profile: %w", err)
		}
	} else if err != nil {
		return nil, err
	}
	return LoadPack(dir)
}

// extractBuiltin writes the embedded pack to dir via a temporary sibling and a
// rename, the same idempotent pattern Install uses: a torn extraction never
// becomes resolvable, and two racing processes both end with a complete pack.
func extractBuiltin(dir string) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, "builtin-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := starter.CopyTree(defaultpack.FS(), ".", tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, dir); err != nil {
		// Another process finished first; its extraction is just as good.
		if _, statErr := os.Stat(dir); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}

func loadInstalled(paths Paths, ref Reference) (*Pack, error) {
	if err := ref.validate(true); err != nil {
		return nil, err
	}
	dir, err := paths.PackDir(ref.ID, ref.Commit)
	if err != nil {
		return nil, err
	}
	pack, err := LoadPack(dir)
	if err != nil {
		return nil, fmt.Errorf("profile %q at %s is not installed: %w\n  run `docc profile install %s`", ref.ID, dir, err, ref.Source)
	}
	if pack.Manifest.ID != ref.ID {
		return nil, fmt.Errorf("installed profile id %q does not match selected id %q", pack.Manifest.ID, ref.ID)
	}
	return pack, nil
}

// Install clones one Git revision into an immutable XDG data directory. A
// checkout is discarded after validation, so profile files cannot become dirty
// and different projects can use different commits concurrently.
func Install(ctx context.Context, paths Paths, source, ref string, policy Policy) (Reference, error) {
	if strings.TrimSpace(source) == "" {
		return Reference{}, errors.New("profile repository is required")
	}
	if strings.HasPrefix(ref, "-") {
		return Reference{}, fmt.Errorf("invalid profile ref %q", ref)
	}
	base := filepath.Join(paths.Data, "docc", "profiles")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return Reference{}, fmt.Errorf("create profile store: %w", err)
	}
	tmp, err := os.MkdirTemp(base, ".install-*")
	if err != nil {
		return Reference{}, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if err := git(ctx, "clone", "--quiet", "--", source, tmp); err != nil {
		return Reference{}, err
	}
	if ref != "" {
		if err := git(ctx, "-C", tmp, "checkout", "--quiet", "--detach", ref); err != nil {
			return Reference{}, err
		}
	}
	commit, err := gitOutput(ctx, "-C", tmp, "rev-parse", "HEAD")
	if err != nil {
		return Reference{}, err
	}
	commit = strings.TrimSpace(commit)
	if !commitPattern.MatchString(commit) {
		return Reference{}, fmt.Errorf("git returned invalid commit %q", commit)
	}
	// Verification happens here or nowhere: the installed copy has its Git
	// metadata removed, so nothing downstream could check a signature.
	if _, err := verify(ctx, tmp, ref, commit, policy); err != nil {
		return Reference{}, err
	}
	if err := git(ctx, "-C", tmp, "checkout", "--quiet", "--detach", commit); err != nil {
		return Reference{}, err
	}
	pack, err := LoadPack(tmp)
	if err != nil {
		return Reference{}, err
	}
	if err := pack.Validate(); err != nil {
		return Reference{}, fmt.Errorf("validate profile %q: %w", pack.Manifest.ID, err)
	}
	result := Reference{ID: pack.Manifest.ID, Source: source, Ref: ref, Commit: commit}
	dest, err := paths.PackDir(result.ID, result.Commit)
	if err != nil {
		return Reference{}, err
	}
	if _, err := os.Stat(dest); err == nil {
		return result, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Reference{}, err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return Reference{}, err
	}
	if err := os.RemoveAll(filepath.Join(tmp, ".git")); err != nil {
		return Reference{}, fmt.Errorf("remove Git metadata: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return Reference{}, fmt.Errorf("install profile revision: %w", err)
	}
	return result, nil
}

// SetDefault installs the reference separately; this function only records a
// known-good installed revision as the user's fallback profile.
func SetDefault(paths Paths, ref Reference) error { return writeUserConfig(paths, ref) }

// UpdateProject replaces only a project's lockfile after a new immutable
// revision was installed and checked to belong to the same profile id.
func UpdateProject(root string, ref Reference) error {
	binding, err := ReadBinding(filepath.Join(root, project.DirName))
	if err != nil {
		return err
	}
	if binding.ID != ref.ID {
		return fmt.Errorf("updated profile id %q does not match project id %q", ref.ID, binding.ID)
	}
	return writeLock(filepath.Join(root, project.DirName), ref.Commit)
}

// Default returns the configured default profile, if any.
func Default(paths Paths) (Reference, error) {
	cfg, err := requireDefault(paths)
	if err != nil {
		return Reference{}, err
	}
	return *cfg.Default, nil
}

// RemoteCommit asks Git which commit a source/ref currently resolves to. It is
// deliberately separate from Resolve so ordinary compilation remains offline.
func RemoteCommit(ctx context.Context, source, ref string) (string, error) {
	if strings.TrimSpace(source) == "" {
		return "", errors.New("profile repository is required")
	}
	if strings.HasPrefix(ref, "-") {
		return "", fmt.Errorf("invalid profile ref %q", ref)
	}
	wanted := ref
	if wanted == "" {
		wanted = "HEAD"
	}
	out, err := gitOutput(ctx, "ls-remote", "--exit-code", "--", source, wanted)
	if err != nil {
		return "", err
	}
	var commit string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !commitPattern.MatchString(fields[0]) {
			continue
		}
		// An annotated tag lists its tag object first and its peeled commit
		// second. Prefer the peeled commit because profile locks pin commits.
		if strings.HasSuffix(fields[1], "^{}") {
			return fields[0], nil
		}
		if commit == "" {
			commit = fields[0]
		}
	}
	if commit == "" {
		return "", fmt.Errorf("git ls-remote returned no commit for %q", wanted)
	}
	return commit, nil
}

func git(ctx context.Context, args ...string) error {
	_, err := gitOutput(ctx, args...)
	return err
}

func gitOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // arguments are fixed or validated, never passed through a shell
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), nil
	}
	text := strings.TrimSpace(string(out))
	if len(text) > 4096 {
		text = text[:4096] + "…"
	}
	if text == "" {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, text)
}
