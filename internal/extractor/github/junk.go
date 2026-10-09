package github

import (
	"path"
	"regexp"
	"sort"
	"strings"
)

var strippedExtensions = map[string]bool{
	// Images
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".svg":  true,
	".ico":  true,
	".bmp":  true,
	".tiff": true,
	".tif":  true,
	".avif": true,
	".heic": true,
	".heif": true,

	// Fonts
	".woff":  true,
	".woff2": true,
	".ttf":   true,
	".otf":   true,
	".eot":   true,

	// Audio
	".mp3":  true,
	".wav":  true,
	".ogg":  true,
	".flac": true,
	".aac":  true,
	".m4a":  true,

	// Video
	".mp4":  true,
	".mov":  true,
	".avi":  true,
	".webm": true,
	".mkv":  true,
	".wmv":  true,
	".m4v":  true,

	// Archives
	".zip": true,
	".tar": true,
	".gz":  true,
	".tgz": true,
	".bz2": true,
	".xz":  true,
	".rar": true,
	".7z":  true,

	// Documents
	".pdf":  true,
	".doc":  true,
	".docx": true,
	".ppt":  true,
	".pptx": true,
	".xls":  true,
	".xlsx": true,

	// Native binaries
	".exe":   true,
	".dll":   true,
	".so":    true,
	".dylib": true,
	".o":     true,
	".a":     true,
	".obj":   true,
	".bin":   true,

	// JVM artifacts
	".class": true,
	".jar":   true,
	".war":   true,
	".ear":   true,

	// Mobile artifacts
	".apk": true,
	".aab": true,
	".ipa": true,

	// Disk images
	".iso": true,
	".img": true,

	// Databases
	".sqlite":  true,
	".sqlite3": true,
	".db":      true,

	// Certificates / keys
	".pem": true,
	".crt": true,
	".cer": true,
	".key": true,

	// Data blobs
	".parquet": true,
	".feather": true,

	// Source maps
	".map": true,

	// Minified assets
	".min.js":  true,
	".min.css": true,
}

var strippedFilenames = map[string]bool{
	// JavaScript
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
	"bun.lockb":         true,

	// PHP
	"composer.lock": true,

	// Ruby
	"Gemfile.lock": true,

	// Rust
	"Cargo.lock": true,

	// Elixir
	"mix.lock": true,

	// Python
	"poetry.lock":  true,
	"Pipfile.lock": true,

	// OS generated
	".DS_Store": true,
	"Thumbs.db": true,
}

var envFilePattern = regexp.MustCompile(`(^|/)\.env($|\.)`)

// envTemplatePattern matches committed template files such as .env.example.
// Those are meant to be public and are not a credential leak.
var envTemplatePattern = regexp.MustCompile(`\.(example|sample|template|dist|defaults)$`)

const (
	minClusterFiles     = 30
	minClusterPercent   = 10.0
	minDuplicationRatio = 0.5
)

var knownJunkDirnames = map[string]bool{
	// Node.js
	"node_modules":  true,
	".next":         true,
	".nuxt":         true,
	".svelte-kit":   true,
	".turbo":        true,
	".parcel-cache": true,
	".pnpm-store":   true,
	".yarn":         true,

	// Python
	"__pycache__":   true,
	".venv":         true,
	".mypy_cache":   true,
	".pytest_cache": true,
	".ruff_cache":   true,
	".tox":          true,
	".nox":          true,

	// PHP
	"vendor": true,

	// Ruby
	".bundle": true,

	// Java / Kotlin
	".gradle": true,

	// Rust
	"target": true,

	// CMake
	"cmake-build-debug":   true,
	"cmake-build-release": true,

	// Coverage
	"coverage": true,

	// Build outputs
	"dist":  true,
	"build": true,
}

type JunkSignals struct {
	JunkDirs []string
	EnvFiles []string
}

func (j JunkSignals) HasIssue() bool {
	return len(j.JunkDirs) > 0 || len(j.EnvFiles) > 0
}

func CountNonJunkFiles(paths []string, junkDirs []string) int {
	if len(junkDirs) == 0 {
		return len(paths)
	}

	count := 0

	for _, p := range paths {
		if !underAnyPrefix(p, junkDirs) {
			count++
		}
	}

	return count
}

func underAnyPrefix(p string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}

	return false
}

func DetectJunk(allPaths, blobPaths []string) JunkSignals {
	var signals JunkSignals

	for _, p := range allPaths {
		if envFilePattern.MatchString(p) && !envTemplatePattern.MatchString(p) {
			signals.EnvFiles = append(signals.EnvFiles, p)
		}
	}

	stripped := make([]string, 0, len(blobPaths))

	for _, p := range blobPaths {
		if !shouldStrip(p) {
			stripped = append(stripped, p)
		}
	}

	junkDirs := knownNameMatches(blobPaths)

	prefixCounts := map[string][]string{}

	for _, p := range stripped {
		for _, prefix := range ancestorPrefixes(p) {
			prefixCounts[prefix] = append(prefixCounts[prefix], p)
		}
	}

	total := len(stripped)

	for d, paths := range prefixCounts {
		if len(paths) < minClusterFiles {
			continue
		}

		pct := 0.0
		if total > 0 {
			pct = float64(len(paths)) / float64(total) * 100
		}

		dup := basenameDuplicationRatio(d, paths)

		if len(paths) >= minClusterFiles &&
			pct >= minClusterPercent &&
			dup >= minDuplicationRatio {
			junkDirs = append(junkDirs, d)
		}
	}

	signals.JunkDirs = dedupeTopLevelJunk(junkDirs)

	return signals
}

func knownNameMatches(blobPaths []string) []string {
	seen := map[string]bool{}
	var matches []string

	for _, p := range blobPaths {
		dir := path.Dir(p)

		for dir != "." && dir != "/" && dir != "" {
			if knownJunkDirnames[path.Base(dir)] && !seen[dir] {
				seen[dir] = true
				matches = append(matches, dir)
			}

			dir = path.Dir(dir)
		}
	}

	return matches
}

func shouldStrip(p string) bool {
	base := path.Base(p)

	if strippedFilenames[base] {
		return true
	}

	// Handle multi-part suffixes like:
	// app.min.js
	// styles.min.css
	for ext := range strippedExtensions {
		if strings.HasSuffix(strings.ToLower(base), ext) {
			return true
		}
	}

	return false
}

func ancestorPrefixes(p string) []string {
	var prefixes []string

	dir := path.Dir(p)

	for dir != "." && dir != "/" && dir != "" {
		prefixes = append(prefixes, dir)
		dir = path.Dir(dir)
	}

	return prefixes
}

func basenameDuplicationRatio(prefix string, paths []string) float64 {
	if len(paths) == 0 {
		return 0
	}

	counts := map[string]int{}

	for _, p := range paths {
		rel := strings.TrimPrefix(p, prefix+"/")
		counts[path.Base(rel)]++
	}

	dup := 0

	for _, p := range paths {
		rel := strings.TrimPrefix(p, prefix+"/")
		if counts[path.Base(rel)] > 1 {
			dup++
		}
	}

	return float64(dup) / float64(len(paths))
}

func dedupeTopLevelJunk(dirs []string) []string {
	if len(dirs) == 0 {
		return nil
	}

	set := make(map[string]bool, len(dirs))

	for _, d := range dirs {
		set[d] = true
	}

	unique := make([]string, 0, len(set))

	for d := range set {
		unique = append(unique, d)
	}

	var out []string

	for _, d := range unique {
		parent := path.Dir(d)
		covered := false

		for parent != "." && parent != "/" && parent != "" {
			if set[parent] {
				covered = true
				break
			}
			parent = path.Dir(parent)
		}

		if !covered {
			out = append(out, d)
		}
	}

	sort.Strings(out)

	return out
}
