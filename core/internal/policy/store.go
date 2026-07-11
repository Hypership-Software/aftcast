package policy

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"

	"github.com/Hypership-Software/atlas/assets"
	"github.com/cedar-policy/cedar-go"
)

// Set is a compiled, hashable collection of policies, yielding an Engine plus a
// stable policy_hash for the append-only log.
type Set struct {
	ps    *cedar.PolicySet
	texts []string // MarshalCedar text of each policy, sorted — the hash input
}

func (s *Set) Engine() *Engine { return NewEngine(s.ps) }

func (s *Set) PolicySet() *cedar.PolicySet { return s.ps }

func (s *Set) Len() int { return len(s.texts) }

// Hash is the SHA-256 over the canonical (MarshalCedar) text of every policy,
// sorted. Sorting survives file reordering and MarshalCedar normalizes
// whitespace/comments, so the hash changes only when a rule's meaning changes.
func (s *Set) Hash() string {
	h := sha256.New()
	for _, t := range s.texts {
		h.Write([]byte(t))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Load compiles every *.cedar file from dirs into one Set. Pass org directories
// LAST so an @id collision resolves in the org's favour. (Cedar is
// deny-overrides, so an org forbid is never weakened by any permit regardless of
// order — load order only settles same-@id collisions.)
func Load(dirs ...string) (*Set, error) {
	acc := newAccumulator()
	for _, dir := range dirs {
		if err := acc.addFS(os.DirFS(dir), ".", dir); err != nil {
			return nil, err
		}
	}
	return acc.set(), nil
}

// StarterSet compiles the embedded starter pack — what a fresh install evaluates
// until the user or org adds policies.
func StarterSet() (*Set, error) {
	acc := newAccumulator()
	if err := acc.addFS(assets.StarterPack, assets.StarterPackDir, "starter-pack"); err != nil {
		return nil, err
	}
	return acc.set(), nil
}

// LoadWithStarter compiles the embedded starter pack FIRST, then layers every
// *.cedar file from dirs on top (org dirs last). A nonexistent dir is skipped —
// a fresh install has no user policy dir yet, and that must not fail the daemon.
// (Cedar is deny-overrides, so layering can only tighten the baseline.)
func LoadWithStarter(dirs ...string) (*Set, error) {
	acc := newAccumulator()
	if err := acc.addFS(assets.StarterPack, assets.StarterPackDir, "starter-pack"); err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := acc.addFS(os.DirFS(dir), ".", dir); err != nil {
			return nil, err
		}
	}
	return acc.set(), nil
}

type accumulator struct {
	ps    *cedar.PolicySet
	texts map[cedar.PolicyID]string
}

func newAccumulator() *accumulator {
	return &accumulator{ps: cedar.NewPolicySet(), texts: map[cedar.PolicyID]string{}}
}

// addFS merges the policies in dir (within fsys) into the accumulator, in
// filename order. label is used only for error messages.
func (a *accumulator) addFS(fsys fs.FS, dir, label string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("read policy dir %s: %w", label, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && path.Ext(e.Name()) == ".cedar" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		doc, err := fs.ReadFile(fsys, path.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read %s/%s: %w", label, name, err)
		}
		list, err := cedar.NewPolicyListFromBytes(name, doc)
		if err != nil {
			return fmt.Errorf("parse %s/%s: %w", label, name, err)
		}
		for _, p := range list {
			id, err := ruleID(p)
			if err != nil {
				return fmt.Errorf("%s/%s: %w", label, name, err)
			}
			a.ps.Remove(id) // later source wins on @id collision
			a.ps.Add(id, p)
			a.texts[id] = string(p.MarshalCedar())
		}
	}
	return nil
}

func (a *accumulator) set() *Set {
	texts := make([]string, 0, len(a.texts))
	for _, t := range a.texts {
		texts = append(texts, t)
	}
	sort.Strings(texts)
	return &Set{ps: a.ps, texts: texts}
}

// ruleID reads the stable @id annotation. A policy without one is rejected:
// positional IDs would break the append-only rule_id contract the moment a file
// is reordered.
func ruleID(p *cedar.Policy) (cedar.PolicyID, error) {
	id, ok := p.Annotations()["id"]
	if !ok || id == "" {
		return "", errors.New("policy is missing a stable @id annotation")
	}
	return cedar.PolicyID(id), nil
}

// signedBundle is a signed org policy bundle: Cedar policy text plus a detached
// Ed25519 signature over exactly those bytes.
type signedBundle struct {
	Policies string `json:"policies"`
	Sig      string `json:"sig"` // base64 Ed25519 signature over Policies
}

// VerifyBundle checks the *.orgbundle carries a valid Ed25519 signature over its
// policy text. Org bundles are the only policy source that can tighten a machine
// remotely, so an unsigned or tampered bundle is rejected before it is trusted.
func VerifyBundle(path string, pubkey []byte) error {
	if len(pubkey) != ed25519.PublicKeySize {
		return fmt.Errorf("public key must be %d bytes, got %d", ed25519.PublicKeySize, len(pubkey))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var b signedBundle
	if err := json.Unmarshal(raw, &b); err != nil {
		return fmt.Errorf("bundle %s is not valid JSON: %w", path, err)
	}
	sig, err := base64.StdEncoding.DecodeString(b.Sig)
	if err != nil {
		return fmt.Errorf("bundle %s has a malformed signature: %w", path, err)
	}
	if !ed25519.Verify(ed25519.PublicKey(pubkey), []byte(b.Policies), sig) {
		return errors.New("bundle signature verification failed")
	}
	return nil
}
