package github

import (
	"path"
	"strings"
)

var sourceAndDocExtensions = map[string]bool{
	// Go
	".go": true,

	// JavaScript / TypeScript
	".js":  true,
	".jsx": true,
	".mjs": true,
	".cjs": true,
	".ts":  true,
	".tsx": true,

	// Python
	".py":  true,
	".pyi": true,
	".pyw": true,

	// Java / JVM
	".java":   true,
	".kt":     true,
	".kts":    true,
	".scala":  true,
	".groovy": true,

	// PHP
	".php":   true,
	".phtml": true,

	// Ruby
	".rb":   true,
	".rake": true,

	// Rust
	".rs": true,

	// C / C++
	".c":   true,
	".h":   true,
	".cpp": true,
	".cc":  true,
	".cxx": true,
	".hpp": true,
	".hh":  true,
	".hxx": true,

	// C#
	".cs": true,

	// Swift / Objective-C
	".swift": true,
	".m":     true,
	".mm":    true,

	// Dart
	".dart": true,

	// Elixir / Erlang
	".ex":  true,
	".exs": true,
	".erl": true,
	".hrl": true,

	// Haskell
	".hs": true,

	// Clojure
	".clj":  true,
	".cljs": true,
	".cljc": true,

	// F#
	".fs":  true,
	".fsi": true,
	".fsx": true,

	// OCaml
	".ml":  true,
	".mli": true,

	// Lua
	".lua": true,

	// Perl
	".pl": true,
	".pm": true,

	// R
	".r": true,

	// Julia
	".jl": true,

	// Zig
	".zig": true,

	// Nim
	".nim": true,

	// Crystal
	".cr": true,

	// Solidity
	".sol": true,

	// V
	".v": true,

	// COBOL
	".cob": true,
	".cbl": true,

	// Fortran
	".f":   true,
	".f90": true,
	".f95": true,

	// Assembly
	".asm": true,
	".s":   true,

	// Shell
	".sh":   true,
	".bash": true,
	".zsh":  true,
	".fish": true,
	".ksh":  true,

	// PowerShell
	".ps1":  true,
	".psm1": true,

	// SQL
	".sql": true,

	// Web
	".html": true,
	".htm":  true,
	".css":  true,
	".scss": true,
	".sass": true,
	".less": true,

	// Components
	".vue":    true,
	".svelte": true,

	// Templates
	".hbs":        true,
	".handlebars": true,
	".twig":       true,
	".jinja":      true,
	".jinja2":     true,
	".ejs":        true,

	// GraphQL
	".graphql": true,
	".gql":     true,

	// Protocol Buffers
	".proto": true,

	// Docs
	".md":   true,
	".mdx":  true,
	".rst":  true,
	".txt":  true,
	".adoc": true,

	// Test suffixes
	".test.js":  true,
	".test.jsx": true,
	".test.ts":  true,
	".test.tsx": true,
	".spec.js":  true,
	".spec.jsx": true,
	".spec.ts":  true,
	".spec.tsx": true,
}

var wideNetNoiseDirnames = map[string]bool{
	// Git
	".git": true,

	// Node.js dependencies & caches
	"node_modules":  true,
	".pnpm-store":   true,
	".yarn":         true,
	".parcel-cache": true,

	// Framework build outputs
	".next":       true,
	".nuxt":       true,
	".svelte-kit": true,
	".turbo":      true,

	// PHP
	"vendor": true,

	// Python
	"__pycache__":   true,
	".venv":         true,
	".mypy_cache":   true,
	".pytest_cache": true,
	".ruff_cache":   true,
	".tox":          true,
	".nox":          true,

	// Java / Kotlin
	".gradle": true,

	// Rust
	"target": true,

	// Generated outputs
	"dist":     true,
	"build":    true,
	"coverage": true,

	// Ruby
	".bundle": true,
}

func FindCandidatePaths(paths []string) []string {
	var found []string

	for _, p := range paths {
		if underAnyNoiseDir(p, wideNetNoiseDirnames) {
			continue
		}

		if isCIConfigPath(p) {
			continue
		}

		if isBlockedExtension(p) {
			continue
		}

		found = append(found, p)
	}

	return found
}

func underAnyNoiseDir(p string, noise map[string]bool) bool {
	for _, seg := range strings.Split(path.Dir(p), "/") {
		if noise[seg] {
			return true
		}
	}
	return false
}

func isBlockedExtension(p string) bool {
	base := strings.ToLower(path.Base(p))

	if strippedExtensions[path.Ext(base)] {
		return true
	}

	for ext := range sourceAndDocExtensions {
		if strings.HasSuffix(base, ext) {
			return true
		}
	}

	return false
}
