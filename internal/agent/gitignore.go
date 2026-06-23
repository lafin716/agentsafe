package agent

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// gitignore support, implemented entirely in-process (no `git` subprocess). It
// reads the feature worktree's `.gitignore` files and reports which paths they
// would ignore, so agent build output (e.g. a freshly built, possibly nested
// build/ directory) is excluded from prepare and from the sync diff.

// gitignoreRule is one compiled `.gitignore` line, scoped to the directory that
// contains the `.gitignore` it came from.
type gitignoreRule struct {
	re      *regexp.Regexp // matches a path relative to the rule's scope dir
	dirOnly bool           // pattern ended with "/", so it matches directories only
	negate  bool           // pattern began with "!", re-including a previously ignored path
}

// scopedRules is the compiled rule set of a single `.gitignore`, together with
// the slash-form repo-relative directory it applies to ("" for the repo root).
type scopedRules struct {
	scope string
	rules []gitignoreRule
}

// gitIgnoredPatterns walks each scan root and returns the slash-form relative
// paths that the worktree's `.gitignore` rules would ignore. Rules are always
// sourced from ruleRoot (the worktree) — even when walking a sanitized agent
// copy — so an agent editing its own `.gitignore` cannot change what is filtered.
// Ignored directories are recorded and not descended into, so large
// build/dependency trees are never enumerated. The returned paths are appended to
// the agent ignore patterns as literals; the existing matcher treats a literal
// "a/b" as also covering "a/b/...", so an ignored directory excludes everything
// beneath it.
func gitIgnoredPatterns(ruleRoot string, scanRoots []string) ([]string, error) {
	cache := map[string][]gitignoreRule{}
	seen := map[string]bool{}
	var out []string
	for _, root := range scanRoots {
		if err := walkGitignore(ruleRoot, root, "", nil, cache, seen, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func walkGitignore(ruleRoot, scanRoot, rel string, inherited []scopedRules, cache map[string][]gitignoreRule, seen map[string]bool, out *[]string) error {
	dir := scanRoot
	if rel != "" {
		dir = filepath.Join(scanRoot, filepath.FromSlash(rel))
	}
	scopes := inherited
	if rules := loadGitignoreRules(ruleRoot, rel, cache); len(rules) > 0 {
		scopes = append(append([]scopedRules{}, inherited...), scopedRules{scope: rel, rules: rules})
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if rel == "" && name == ".git" {
			continue // never report or descend into the Git directory itself
		}
		cr := name
		if rel != "" {
			cr = path.Join(rel, name)
		}
		if gitignoreMatch(scopes, cr, e.IsDir()) {
			if !seen[cr] {
				seen[cr] = true
				*out = append(*out, cr)
			}
			continue // do not descend into an ignored directory
		}
		if e.IsDir() {
			if err := walkGitignore(ruleRoot, scanRoot, cr, scopes, cache, seen, out); err != nil {
				return err
			}
		}
	}
	return nil
}

// loadGitignoreRules reads and compiles the `.gitignore` at ruleRoot/<rel> (the
// worktree copy), caching the result by rel. A missing file caches an empty set.
func loadGitignoreRules(ruleRoot, rel string, cache map[string][]gitignoreRule) []gitignoreRule {
	if cached, ok := cache[rel]; ok {
		return cached
	}
	p := filepath.Join(ruleRoot, filepath.FromSlash(rel), ".gitignore")
	b, err := os.ReadFile(p)
	var rules []gitignoreRule
	if err == nil {
		rules = compileGitignore(strings.Split(string(b), "\n"))
	}
	cache[rel] = rules
	return rules
}

// gitignoreMatch reports whether rel (slash-form, relative to the repo root) is
// ignored by the applicable scoped rule sets. Scopes are ordered shallow→deep and
// rules within a scope are in file order; the last matching rule wins, so deeper
// `.gitignore` files and later lines override earlier ones (gitignore semantics).
func gitignoreMatch(scopes []scopedRules, rel string, isDir bool) bool {
	ignored := false
	for _, s := range scopes {
		sub := rel
		if s.scope != "" {
			sub = strings.TrimPrefix(rel, s.scope+"/")
		}
		for _, r := range s.rules {
			if r.dirOnly && !isDir {
				continue
			}
			if r.re.MatchString(sub) {
				ignored = !r.negate
			}
		}
	}
	return ignored
}

func compileGitignore(lines []string) []gitignoreRule {
	var rules []gitignoreRule
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		line = strings.TrimRight(line, " ") // trailing spaces are insignificant
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = line[1:]
		}
		// An escaped leading "#" or "!" is a literal first character.
		if strings.HasPrefix(line, "\\#") || strings.HasPrefix(line, "\\!") {
			line = line[1:]
		}
		dirOnly := false
		if strings.HasSuffix(line, "/") {
			dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		if line == "" {
			continue
		}
		anchored := false
		if strings.HasPrefix(line, "/") {
			anchored = true
			line = line[1:]
		}
		if strings.Contains(line, "/") {
			anchored = true // a slash anywhere anchors to the .gitignore's directory
		}
		re, err := regexp.Compile(gitignoreToRegex(line, anchored))
		if err != nil {
			continue // skip a malformed pattern rather than failing the whole scan
		}
		rules = append(rules, gitignoreRule{re: re, dirOnly: dirOnly, negate: negate})
	}
	return rules
}

// gitignoreToRegex translates a single gitignore pattern body (negation,
// trailing slash, and a leading slash already stripped) into a regexp that
// matches a path relative to the rule's scope. An unanchored pattern matches the
// basename at any depth; an anchored one matches from the scope root.
func gitignoreToRegex(p string, anchored bool) string {
	var b strings.Builder
	if anchored {
		b.WriteString("^")
	} else {
		b.WriteString("^(?:.*/)?")
	}
	runes := []rune(p)
	n := len(runes)
	for i := 0; i < n; {
		c := runes[i]
		switch c {
		case '*':
			if i+1 < n && runes[i+1] == '*' {
				j := i
				for j < n && runes[j] == '*' {
					j++
				}
				prevSlash := i == 0 || runes[i-1] == '/'
				nextSlash := j < n && runes[j] == '/'
				if prevSlash && nextSlash {
					b.WriteString("(?:.*/)?") // "**/" → zero or more leading segments
					i = j + 1
					continue
				}
				b.WriteString(".*") // any other "**" → cross segments
				i = j
				continue
			}
			b.WriteString("[^/]*")
			i++
		case '?':
			b.WriteString("[^/]")
			i++
		case '[':
			i = writeCharClass(&b, runes, i)
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
			i++
		}
	}
	b.WriteString("$")
	return b.String()
}

// writeCharClass copies a gitignore "[...]" class starting at runes[i] (a "[")
// into b as a regexp class, translating a leading "!" negation to "^". It returns
// the index just past the closing "]". A class with no closing bracket is treated
// as a literal "[".
func writeCharClass(b *strings.Builder, runes []rune, i int) int {
	n := len(runes)
	// Find the closing ']' (a ']' immediately after '[' or '[!' is a literal).
	k := i + 1
	if k < n && runes[k] == '!' {
		k++
	}
	if k < n && runes[k] == ']' {
		k++
	}
	for k < n && runes[k] != ']' {
		k++
	}
	if k >= n {
		b.WriteString(regexp.QuoteMeta("["))
		return i + 1
	}
	b.WriteString("[")
	j := i + 1
	if runes[j] == '!' {
		b.WriteString("^")
		j++
	}
	for ; j < k; j++ {
		if runes[j] == '\\' && j+1 < k {
			b.WriteByte('\\')
			b.WriteRune(runes[j+1])
			j++
			continue
		}
		b.WriteRune(runes[j])
	}
	b.WriteString("]")
	return k + 1
}
