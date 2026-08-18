package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kevinzehnder/docc/internal/profile"
	"github.com/kevinzehnder/docc/internal/project"
)

const (
	profileInstallHelp = `docc profile install [--ref REF] [--default] REPOSITORY

Clone REPOSITORY at REF (or its default HEAD), validate it as a profile pack,
and store the immutable revision under the XDG data directory. --default makes
that revision available outside a project binding.

flags:
`
	profileUseHelp = `docc profile use [--ref REF] [--project DIR] REPOSITORY

Install REPOSITORY and bind DIR (default: .) to its exact revision. The binding
and lockfile are committed with the document project; schemas and themes remain
in the user profile store.

flags:
`
	profileUpdateHelp = `docc profile update [--project DIR]

Resolve and install the configured source/ref again. With --project, update
that project's lockfile. Without it, update the configured user default.

flags:
`
	profileStatusHelp = `docc profile status [--project DIR] [--check-remote]

Report the installed profile revision resolved for DIR (default: .). The command
is offline unless --check-remote asks Git whether the selected ref has advanced.

flags:
`
)

func cmdProfile(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, profileHelp)
		return exitUsage
	}
	switch args[0] {
	case "install":
		return cmdProfileInstall(args[1:])
	case "use":
		return cmdProfileUse(args[1:])
	case "update":
		return cmdProfileUpdate(args[1:])
	case "status":
		return cmdProfileStatus(args[1:])
	case "-h", "--help", "help":
		fmt.Print(profileHelp)
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "docc profile: unknown command %q\n\n%s", args[0], profileHelp)
		return exitUsage
	}
}

func cmdProfileInstall(args []string) int {
	fs := flag.NewFlagSet("profile install", flag.ContinueOnError)
	ref := fs.String("ref", "", "Git branch, tag, or revision (default: repository HEAD)")
	setDefault := fs.Bool("default", false, "set this installed revision as the user default")
	var trust trustFlags
	trust.bind(fs)
	if code, stop := parseFlags(fs, profileInstallHelp, args); stop {
		return code
	}
	if fs.NArg() != 1 {
		return failf(commonFlags{}, exitUsage, "usage: docc profile install [--ref REF] [--default] REPOSITORY")
	}
	paths, err := profile.XDGPaths()
	if err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	policy, err := trust.policy(paths)
	if err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	installed, err := profile.Install(context.Background(), paths, fs.Arg(0), *ref, policy)
	if err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	if *setDefault {
		if err := profile.SetDefault(paths, installed); err != nil {
			return fail(commonFlags{}, exitConfig, err)
		}
	}
	printProfileReference(installed, *setDefault)
	return exitOK
}

func cmdProfileUse(args []string) int {
	fs := flag.NewFlagSet("profile use", flag.ContinueOnError)
	ref := fs.String("ref", "", "Git branch, tag, or revision (default: repository HEAD)")
	projectDir := fs.String("project", ".", "project directory to bind")
	var trust trustFlags
	trust.bind(fs)
	if code, stop := parseFlags(fs, profileUseHelp, args); stop {
		return code
	}
	if fs.NArg() != 1 {
		return failf(commonFlags{}, exitUsage, "usage: docc profile use [--ref REF] [--project DIR] REPOSITORY")
	}
	paths, err := profile.XDGPaths()
	if err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	policy, err := trust.policy(paths)
	if err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	installed, err := profile.Install(context.Background(), paths, fs.Arg(0), *ref, policy)
	if err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	root, err := filepath.Abs(*projectDir)
	if err != nil {
		return fail(commonFlags{}, exitUsage, err)
	}
	if err := profile.WriteProject(root, installed); err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	fmt.Printf("bound %s to profile %s at %s\n", root, installed.ID, installed.Commit)
	return exitOK
}

func cmdProfileUpdate(args []string) int {
	fs := flag.NewFlagSet("profile update", flag.ContinueOnError)
	projectDir := fs.String("project", "", "project directory whose lockfile to update")
	var trust trustFlags
	trust.bind(fs)
	if code, stop := parseFlags(fs, profileUpdateHelp, args); stop {
		return code
	}
	if fs.NArg() != 0 {
		return failf(commonFlags{}, exitUsage, "usage: docc profile update [--project DIR]")
	}
	paths, err := profile.XDGPaths()
	if err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	policy, err := trust.policy(paths)
	if err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	if *projectDir == "" {
		return updateDefault(paths, policy)
	}
	proj, err := project.Resolve(*projectDir)
	if err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	binding, err := profile.ReadBinding(proj.Dir)
	if err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	installed, err := profile.Install(context.Background(), paths, binding.Source, binding.Ref, policy)
	if err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	if err := profile.UpdateProject(proj.Root, installed); err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	fmt.Printf("updated %s to profile %s at %s\n", proj.Root, installed.ID, installed.Commit)
	return exitOK
}

func updateDefault(paths profile.Paths, policy profile.Policy) int {
	current, err := profile.Default(paths)
	if err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	installed, err := profile.Install(context.Background(), paths, current.Source, current.Ref, policy)
	if err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	if installed.ID != current.ID {
		return failf(commonFlags{}, exitConfig, "updated profile id %q does not match default id %q", installed.ID, current.ID)
	}
	if err := profile.SetDefault(paths, installed); err != nil {
		return fail(commonFlags{}, exitConfig, err)
	}
	fmt.Printf("updated default profile %s to %s\n", installed.ID, installed.Commit)
	return exitOK
}

func cmdProfileStatus(args []string) int {
	fs := flag.NewFlagSet("profile status", flag.ContinueOnError)
	projectDir := fs.String("project", ".", "path from which to resolve the profile")
	checkRemote := fs.Bool("check-remote", false, "query Git to see whether the selected ref has advanced")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if code, stop := parseFlags(fs, profileStatusHelp, args); stop {
		return code
	}
	if fs.NArg() != 0 {
		return failf(commonFlags{jsonOut: *jsonOut}, exitUsage, "usage: docc profile status [--project DIR]")
	}
	paths, err := profile.XDGPaths()
	if err != nil {
		return fail(commonFlags{jsonOut: *jsonOut}, exitConfig, err)
	}
	resolved, err := profile.Resolve(*projectDir, paths)
	if err != nil {
		return fail(commonFlags{jsonOut: *jsonOut}, exitConfig, err)
	}
	var remote string
	if *checkRemote {
		if resolved.Reference == nil {
			return failf(commonFlags{jsonOut: *jsonOut}, exitConfig, "the resolved %s configuration has no Git source", resolved.Source)
		}
		remote, err = profile.RemoteCommit(context.Background(), resolved.Reference.Source, resolved.Reference.Ref)
		if err != nil {
			return fail(commonFlags{jsonOut: *jsonOut}, exitConfig, err)
		}
	}
	if *jsonOut {
		out := struct {
			*profile.Resolved
			RemoteCommit string `json:"remote_commit,omitempty"`
			Stale        bool   `json:"stale"`
		}{Resolved: resolved, RemoteCommit: remote, Stale: remote != "" && resolved.Reference != nil && remote != resolved.Reference.Commit}
		if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
			return fail(commonFlags{jsonOut: true}, exitDiag, err)
		}
		return exitOK
	}
	fmt.Printf("source:  %s\n", resolved.Source)
	fmt.Printf("schemas: %s\n", resolved.SchemaDir)
	fmt.Printf("themes:  %s\n", resolved.ThemeDir)
	if resolved.Reference != nil {
		fmt.Printf("profile: %s\nsource:  %s\nref:     %s\ncommit:  %s\n", resolved.Reference.ID, resolved.Reference.Source, resolved.Reference.Ref, resolved.Reference.Commit)
	}
	if remote != "" {
		state := "current"
		if resolved.Reference != nil && remote != resolved.Reference.Commit {
			state = "stale"
		}
		fmt.Printf("remote:  %s (%s)\n", remote, state)
	}
	return exitOK
}

// trustFlags bind the signature policy a single command may add on top of the
// machine's configuration.
type trustFlags struct {
	requireSignature bool
	signers          string
}

func (t *trustFlags) bind(fs *flag.FlagSet) {
	fs.BoolVar(&t.requireSignature, "require-signature", false, "refuse a revision without a good signature")
	fs.StringVar(&t.signers, "signer", "", "comma-separated key fingerprints permitted to sign the profile")
}

// policy combines the machine's configured trust policy with what this command
// asked for. A flag can only tighten: an operator's configuration is not
// something a single invocation gets to relax.
func (t *trustFlags) policy(paths profile.Paths) (profile.Policy, error) {
	policy, err := profile.TrustPolicy(paths)
	if err != nil {
		return policy, err
	}
	if t.requireSignature {
		policy.RequireSignature = true
	}
	for _, s := range strings.Split(t.signers, ",") {
		if s = strings.TrimSpace(s); s != "" {
			policy.AllowedSigners = append(policy.AllowedSigners, s)
		}
	}
	return policy, nil
}

func printProfileReference(ref profile.Reference, isDefault bool) {
	fmt.Printf("installed profile %s at %s\n", ref.ID, ref.Commit)
	if isDefault {
		fmt.Println("set as user default")
	}
}
