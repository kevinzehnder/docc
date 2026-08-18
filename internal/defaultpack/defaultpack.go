// Package defaultpack carries the starter profile pack embedded in the docc
// binary. It is content only: a manifest, schemas and themes in exactly the
// layout of any other profile pack. The profile layer decides where the pack
// materializes on disk; docc init copies it out as an editable checkout.
package defaultpack

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"sort"
	"sync"
)

//go:embed all:files
var files embed.FS

// ID is the pack id declared in the embedded manifest.
const ID = "starter"

// FS returns the embedded pack rooted at its manifest.
func FS() fs.FS {
	sub, err := fs.Sub(files, "files")
	if err != nil {
		// The subtree is compiled in; failure here is a build defect.
		panic(err)
	}
	return sub
}

var hashOnce = sync.OnceValue(func() string {
	sum := sha256.New()
	var paths []string
	err := fs.WalkDir(FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		data, err := fs.ReadFile(FS(), path)
		if err != nil {
			panic(err)
		}
		sum.Write([]byte(path))
		sum.Write([]byte{0})
		sum.Write(data)
		sum.Write([]byte{0})
	}
	return hex.EncodeToString(sum.Sum(nil))
})

// Hash returns a content hash over the embedded pack. It is 64 hex characters,
// so the profile store can address the materialized pack exactly like an
// installed Git revision — a changed pack in a new docc release lands in a new
// directory of its own accord.
func Hash() string { return hashOnce() }
