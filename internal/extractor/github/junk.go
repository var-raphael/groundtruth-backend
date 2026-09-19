package github

import (
	"path"
	"regexp"
	"sort"
	"strings"
)

var strippedExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".svg": true, ".ico": true, ".bmp": true, ".tiff": true, ".avif": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".mp4": true, ".mp3": true, ".wav": true, ".mov": true, ".avi": true,
	".webm": true, ".ogg": true,
	".pdf": true, ".zip": true, ".tar": true, ".gz": true, ".rar": true, ".7z": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".class": true,
	".jar": true, ".apk": true, ".ipa": true, ".o": true, ".a": true,
}

var strippedFilenames = map[string]bool{
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
	"Gemfile.lock":      true,
	"composer.lock":     true,
}

var envFilePattern = regexp.MustCompile(`(^|/)\.env($|\.)`)

const (
	minClusterFiles     = 30
	minClusterPercent   = 10.0
	minDuplicationRatio = 0.5
)

var knownJunkDirnames = map[string]bool{
	"node_modules":     true,
	".next":            true,
	".nuxt":            true,
	".svelte-kit":      true,
	".turbo":           true,
	".parcel-cache":    true,
	"bower_components": true,

	"__pycache__":   true,
	".venv":         true,
	".mypy_cache":   true,
	".pytest_cache": true,
	".tox":          true,

	"vendor": true,

	".bundle": true,

	".gradle": true,

	"cmake-build-debug":   true,
	"cmake-build-release": true,
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
		if envFilePattern.MatchString(p) {
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

		if len(paths) >= minClusterFiles && pct >= minClusterPercent && dup >= minDuplicationRatio {
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
	ext := strings.ToLower(path.Ext(p))
	return strippedExtensions[ext]
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
