package profile

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// packRepo builds a minimal, valid profile-pack Git repository.
func packRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	write(t, filepath.Join(repo, manifestName), "format: 1\nid: firm\nschemas: schemas\nthemes: themes\n")
	write(t, filepath.Join(repo, "schemas", "memo.yaml"), "type: memo\ndescription: memo\ntheme: t\n")
	write(t, filepath.Join(repo, "themes", "t.yaml"), "name: t\ndescription: theme\n")
	gitTest(t, repo, "init")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
	return repo
}

// An unsigned revision installs happily until someone asks for a signature.
// Nothing is verified unless it was configured, and that has to be true or the
// default install would break for everyone.
func TestInstallWithoutPolicyIgnoresSignatures(t *testing.T) {
	if _, err := Install(context.Background(), testPaths(t), packRepo(t), "", Policy{}); err != nil {
		t.Fatalf("Install with no policy: %v", err)
	}
}

func TestInstallRequiringSignatureRejectsUnsigned(t *testing.T) {
	_, err := Install(context.Background(), testPaths(t), packRepo(t), "", Policy{RequireSignature: true})
	if err == nil {
		t.Fatal("expected an unsigned revision to be refused")
	}
	if !strings.Contains(err.Error(), "no signature") {
		t.Errorf("error = %q, want it to say the revision is unsigned", err)
	}
}

// An allowed-signers list alone is a policy: naming who may sign implies that
// something must be signed.
func TestInstallWithAllowedSignersRejectsUnsigned(t *testing.T) {
	_, err := Install(context.Background(), testPaths(t), packRepo(t), "", Policy{AllowedSigners: []string{"DEADBEEF"}})
	if err == nil {
		t.Fatal("expected an unsigned revision to be refused when signers are named")
	}
}

// A refused revision must not be left behind: a later install with no policy
// would otherwise find it already present and skip verification entirely.
func TestRefusedInstallLeavesNothingInstalled(t *testing.T) {
	paths := testPaths(t)
	if _, err := Install(context.Background(), paths, packRepo(t), "", Policy{RequireSignature: true}); err == nil {
		t.Fatal("expected the install to be refused")
	}
	store := filepath.Join(paths.Data, "docc", "profiles")
	entries, err := os.ReadDir(store)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".install-") {
			t.Errorf("refused install left %s in the profile store", e.Name())
		}
	}
}

// signedPackRepo builds a pack whose commit carries a real SSH signature, and
// points Git's global configuration at the key that made it. SSH signing is
// used because it needs only ssh-keygen — a GPG keyring in a test is a
// different test, of GPG.
//
// It returns the repository and the signing key's fingerprint.
func signedPackRepo(t *testing.T) (repo, fingerprint string) {
	t.Helper()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen is not available; skipping signature verification")
	}
	keys := t.TempDir()
	key := filepath.Join(keys, "id")
	if out, err := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-f", key, "-C", "test@example.invalid").CombinedOutput(); err != nil {
		t.Skipf("ssh-keygen failed: %v: %s", err, out)
	}
	pub, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	allowedSigners := filepath.Join(keys, "allowed_signers")
	write(t, allowedSigners, "test@example.invalid "+string(pub))

	gitConfig := filepath.Join(keys, "gitconfig")
	write(t, gitConfig, strings.Join([]string{
		"[user]",
		"\tname = Test",
		"\temail = test@example.invalid",
		"\tsigningkey = " + key + ".pub",
		"[gpg]",
		"\tformat = ssh",
		"[gpg \"ssh\"]",
		"\tallowedSignersFile = " + allowedSigners,
		"[commit]",
		"\tgpgsign = true",
		"",
	}, "\n"))
	// Install shells out to git, which inherits this process's environment.
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	// Sign with the key file and nothing else. A developer's ssh-agent is tried
	// first otherwise, and an agent that asks its owner to approve each
	// signature — a hardware key, a password manager — blocks the test forever.
	t.Setenv("SSH_AUTH_SOCK", "")
	t.Setenv("SSH_ASKPASS_REQUIRE", "never")
	t.Setenv("DISPLAY", "")

	repo = packRepo(t)
	gitTest(t, repo, "commit", "--amend", "--no-edit", "-S")

	out, err := exec.Command("ssh-keygen", "-lf", key+".pub").Output()
	if err != nil {
		t.Fatal(err)
	}
	// "256 SHA256:… comment (ED25519)" — the fingerprint is the second field.
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		t.Fatalf("unexpected ssh-keygen -lf output: %q", out)
	}
	return repo, fields[1]
}

func TestInstallAcceptsASignedRevision(t *testing.T) {
	repo, _ := signedPackRepo(t)
	if _, err := Install(context.Background(), testPaths(t), repo, "", Policy{RequireSignature: true}); err != nil {
		t.Fatalf("Install of a signed revision: %v", err)
	}
}

func TestInstallAcceptsAnAllowedSigner(t *testing.T) {
	repo, fingerprint := signedPackRepo(t)
	policy := Policy{RequireSignature: true, AllowedSigners: []string{fingerprint}}
	if _, err := Install(context.Background(), testPaths(t), repo, "", policy); err != nil {
		t.Fatalf("Install signed by an allowed signer: %v", err)
	}
}

// A good signature from the wrong key is the case an allowed-signers list
// exists for: the pack is authentic, but not the firm's.
func TestInstallRejectsASignerNotOnTheList(t *testing.T) {
	repo, fingerprint := signedPackRepo(t)
	policy := Policy{RequireSignature: true, AllowedSigners: []string{"SHA256:AAAAthisIsNotTheKeyThatSignedAnything00000000"}}
	_, err := Install(context.Background(), testPaths(t), repo, "", policy)
	if err == nil {
		t.Fatal("expected a signature from an unlisted key to be refused")
	}
	if !strings.Contains(err.Error(), "not an allowed signer") {
		t.Errorf("error = %q, want it to name the unlisted signer", err)
	}
	if !strings.Contains(err.Error(), fingerprint) {
		t.Errorf("error = %q, want it to report the key that actually signed", err)
	}
}

func TestPolicyEnabled(t *testing.T) {
	for _, tt := range []struct {
		name   string
		policy Policy
		want   bool
	}{
		{"empty", Policy{}, false},
		{"require", Policy{RequireSignature: true}, true},
		{"signers", Policy{AllowedSigners: []string{"ABC"}}, true},
	} {
		if got := tt.policy.enabled(); got != tt.want {
			t.Errorf("%s: enabled() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// Operators paste fingerprints from whatever showed them, so the comparison
// has to survive spacing, case and the short forms Git and GPG print.
func TestAllowedMatchesFingerprintForms(t *testing.T) {
	const full = "ABCD1234ABCD1234ABCD1234ABCD1234ABCD1234"
	for _, tt := range []struct {
		name    string
		signers []string
		want    bool
	}{
		{"exact", []string{full}, true},
		{"lowercase", []string{strings.ToLower(full)}, true},
		{"spaced", []string{"ABCD 1234 ABCD 1234 ABCD 1234 ABCD 1234 ABCD 1234"}, true},
		{"short key id", []string{"ABCD1234ABCD1234"}, true},
		{"0x prefixed", []string{"0x" + full}, true},
		{"other key", []string{"FFFF0000FFFF0000"}, false},
		{"empty entry", []string{""}, false},
	} {
		if got := allowed(full, tt.signers); got != tt.want {
			t.Errorf("%s: allowed = %v, want %v", tt.name, got, tt.want)
		}
	}
	// A key Git could not report must never satisfy an allow-list.
	if allowed("", []string{full}) {
		t.Error("an unreported fingerprint satisfied an allowed-signers list")
	}
}

// An SSH fingerprint is base64. Folding its case — which is right for a hex
// GPG fingerprint — turns one key into another, so it must not happen.
func TestAllowedTreatsSSHFingerprintsAsCaseSensitive(t *testing.T) {
	const key = "SHA256:7ZTCVMwgbsozLB4URSN3HW5rHfbSU521iYifH06FTC4"
	if !allowed(key, []string{key}) {
		t.Error("an SSH fingerprint did not match itself")
	}
	if allowed(key, []string{strings.ToUpper(key)}) {
		t.Error("an SSH fingerprint matched a differently-cased key, which is a different key")
	}
	// Nor may a suffix match: base64 has no short form.
	if allowed(key, []string{"H06FTC4"}) {
		t.Error("an SSH fingerprint matched a suffix, which identifies nothing")
	}
}

func TestSignatureGood(t *testing.T) {
	for status, want := range map[string]bool{"G": true, "U": true, "B": false, "E": false, "N": false, "": false} {
		if got := (Signature{Status: status}).Good(); got != want {
			t.Errorf("status %q: Good() = %v, want %v", status, got, want)
		}
	}
}

// The machine's policy is not something selecting a profile may quietly drop.
func TestSetDefaultPreservesPolicy(t *testing.T) {
	paths := testPaths(t)
	if err := writeYAML(paths.configPath(), UserConfig{
		Format: format,
		Policy: &Policy{RequireSignature: true, AllowedSigners: []string{"ABCD1234"}},
	}, 0o600); err != nil {
		t.Fatal(err)
	}
	ref := Reference{ID: "firm", Source: "ssh://git.example.invalid/p.git", Commit: strings.Repeat("a", 40)}
	if err := SetDefault(paths, ref); err != nil {
		t.Fatal(err)
	}
	policy, err := TrustPolicy(paths)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.RequireSignature || len(policy.AllowedSigners) != 1 {
		t.Errorf("policy after SetDefault = %+v, want the configured policy intact", policy)
	}
	got, err := Default(paths)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != ref.ID {
		t.Errorf("Default = %+v, want the profile just set", got)
	}
}

// A machine with no configuration verifies nothing, and says so by reporting
// an empty policy rather than an error.
func TestTrustPolicyWithoutConfiguration(t *testing.T) {
	policy, err := TrustPolicy(testPaths(t))
	if err != nil {
		t.Fatalf("TrustPolicy on an unconfigured machine: %v", err)
	}
	if policy.enabled() {
		t.Errorf("policy = %+v, want nothing required by default", policy)
	}
}
