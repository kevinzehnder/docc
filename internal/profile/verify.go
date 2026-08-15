package profile

import (
	"context"
	"fmt"
	"strings"
)

// Policy is an organisation's trust requirement for the profile revisions it
// installs. It is always configured explicitly and never inferred: silently
// deciding that an unsigned pack is acceptable, or that a signed one is
// trusted, are both decisions only the operator can make.
type Policy struct {
	// RequireSignature refuses to install a revision that does not carry a
	// good signature.
	RequireSignature bool `yaml:"require_signature" json:"require_signature"`
	// AllowedSigners lists the key fingerprints permitted to sign a profile.
	// Empty means any key the local keyring trusts, which is a weaker and
	// deliberately separate statement from naming the firm's own keys.
	AllowedSigners []string `yaml:"allowed_signers,omitempty" json:"allowed_signers,omitempty"`
}

// enabled reports whether the policy asks for anything at all.
func (p Policy) enabled() bool { return p.RequireSignature || len(p.AllowedSigners) > 0 }

// Signature is the outcome of verifying one revision.
type Signature struct {
	// Object is "tag" or "commit": what actually carried the signature.
	Object string `yaml:"object" json:"object"`
	// Status is Git's one-letter verdict: G good, U good with an untrusted
	// user id, B bad, E unverifiable, N absent.
	Status string `yaml:"status" json:"status"`
	// Fingerprint is the signing key, when Git could report one.
	Fingerprint string `yaml:"fingerprint,omitempty" json:"fingerprint,omitempty"`
	Signer      string `yaml:"signer,omitempty" json:"signer,omitempty"`
}

// Good reports whether the signature verified. An untrusted user id still
// counts: the key is what an allowed-signers list pins, and the local
// keyring's opinion of the owner's name is a separate question.
func (s Signature) Good() bool { return s.Status == "G" || s.Status == "U" }

// verify checks a revision against a policy inside a checkout that still has
// its Git metadata. It is the only moment verification is possible: an
// installed pack has had .git removed, so a later check would have nothing to
// verify against.
func verify(ctx context.Context, dir, ref, commit string, policy Policy) (*Signature, error) {
	if !policy.enabled() {
		return nil, nil
	}
	sig, err := readSignature(ctx, dir, ref, commit)
	if err != nil {
		return nil, err
	}
	switch {
	case sig.Status == "N":
		return nil, fmt.Errorf("profile revision %s carries no signature, and the configured policy requires one\n"+
			"  sign the %s, or clear require_signature for this installation", short(commit), sig.Object)
	case sig.Status == "B":
		return nil, fmt.Errorf("profile revision %s has a BAD signature — the %s does not match its key", short(commit), sig.Object)
	case !sig.Good():
		return nil, fmt.Errorf("profile revision %s could not be verified (git reported %q)\n"+
			"  the signing key is probably not in the local keyring", short(commit), sig.Status)
	}
	if len(policy.AllowedSigners) > 0 && !allowed(sig.Fingerprint, policy.AllowedSigners) {
		return nil, fmt.Errorf("profile revision %s is signed by %s, which is not an allowed signer\n"+
			"  allowed: %s", short(commit), fingerprintOrUnknown(sig.Fingerprint), strings.Join(policy.AllowedSigners, ", "))
	}
	return sig, nil
}

// readSignature asks Git what signs a revision. An annotated tag is checked in
// preference to the commit it points at, because signing the tag is how a
// release is normally marked and the commit beneath it is often unsigned.
func readSignature(ctx context.Context, dir, ref, commit string) (*Signature, error) {
	if ref != "" {
		typ, err := gitOutput(ctx, "-C", dir, "cat-file", "-t", ref)
		if err == nil && strings.TrimSpace(typ) == "tag" {
			out, err := gitOutput(ctx, "-C", dir, "for-each-ref", "--format=%(signature:grade)%n%(signature:key)%n%(signature:signer)",
				"--points-at", commit, "refs/tags/"+ref)
			if err != nil {
				return nil, err
			}
			if sig := parseSignature("tag", out); sig.Status != "" {
				return sig, nil
			}
		}
	}
	out, err := gitOutput(ctx, "-C", dir, "log", "-1", "--format=%G?%n%GF%n%GS", commit)
	if err != nil {
		return nil, err
	}
	sig := parseSignature("commit", out)
	if sig.Status == "" {
		sig.Status = "N"
	}
	return sig, nil
}

func parseSignature(object, out string) *Signature {
	fields := strings.Split(strings.TrimRight(out, "\n"), "\n")
	sig := &Signature{Object: object}
	for i, v := range fields {
		v = strings.TrimSpace(v)
		switch i {
		case 0:
			sig.Status = v
		case 1:
			sig.Fingerprint = v
		case 2:
			sig.Signer = v
		}
	}
	return sig
}

// allowed reports whether the signing key is one the policy names.
func allowed(fingerprint string, signers []string) bool {
	got := strings.TrimSpace(fingerprint)
	if got == "" {
		return false
	}
	for _, s := range signers {
		if want := strings.TrimSpace(s); want != "" && matchKey(got, want) {
			return true
		}
	}
	return false
}

// matchKey compares two key identifiers.
//
// A GPG fingerprint is hex, so case and the spacing key managers display it
// with are noise, and a short key id is the tail of the long one — an operator
// will paste whichever form was in front of them. An SSH key fingerprint is
// base64, where case carries meaning and a suffix means nothing, so anything
// that is not hex must match exactly.
func matchKey(got, want string) bool {
	if isHexKey(got) && isHexKey(want) {
		g, w := normaliseHex(got), normaliseHex(want)
		return g == w || strings.HasSuffix(g, w)
	}
	return got == want
}

func isHexKey(s string) bool {
	s = normaliseHex(s)
	if len(s) < 8 {
		return false
	}
	return strings.IndexFunc(s, func(r rune) bool {
		return !strings.ContainsRune("0123456789ABCDEF", r)
	}) < 0
}

func normaliseHex(s string) string {
	s = strings.ToUpper(strings.NewReplacer(" ", "", "\t", "").Replace(strings.TrimSpace(s)))
	return strings.TrimPrefix(s, "0X")
}

func fingerprintOrUnknown(f string) string {
	if f == "" {
		return "an unreported key"
	}
	return f
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
