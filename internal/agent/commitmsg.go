package agent

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Commit Message Template: the workspace-level pattern that produces a commit
// message when a delivery action is run without an explicit one.
//
// Only `동기화 → 커밋 → 푸시` (SyncCommitPush) uses it. The per-repository and
// whole-feature commit buttons still require a message the user typed, because
// a template cannot know what a particular change was for.
//
// Every variable resolves from Feature metadata or the clock, so the whole
// message is rendered once, up front, and handed to feature.Commit as a plain
// string. {{repo}} and {{changeCount}} are deliberately absent: they would need
// a different message per repository, which would mean changing what
// feature.Commit takes.

// CommitMessageValues holds everything a Commit Message Template can refer to.
type CommitMessageValues struct {
	Feature string
	Branch  string
	Base    string
	// Now is the instant the delivery ran. Injected rather than read from the
	// clock so the same values render the same message in tests and in previews.
	Now time.Time
}

// substitutions maps each supported variable to what it renders as. Keeping the
// list in one place is what lets ValidateCommitMessageTemplate report the
// supported set without it drifting from what RenderCommitMessage substitutes.
func (v CommitMessageValues) substitutions() map[string]string {
	return map[string]string{
		"feature":   v.Feature,
		"branch":    v.Branch,
		"base":      v.Base,
		"timestamp": v.Now.Format(time.RFC3339),
		"date":      v.Now.Format("2006-01-02"),
		"time":      v.Now.Format("15:04:05"),
	}
}

// CommitMessageVariables lists the supported variable names, sorted, for error
// messages and the settings UI.
func CommitMessageVariables() []string {
	names := make([]string, 0, 6)
	for name := range (CommitMessageValues{}).substitutions() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// placeholderRE matches one {{name}}, tolerating spaces inside the braces. A
// lone "{" or a "{single}" is literal text and does not match, so a template
// can contain braces that are not placeholders.
var placeholderRE = regexp.MustCompile(`\{\{\s*([A-Za-z][A-Za-z0-9]*)\s*\}\}`)

// ValidateCommitMessageTemplate reports any {{...}} the template uses that
// cannot be rendered. Called when the template is saved, so an unusable
// template is refused while the user is looking at it rather than discovered at
// commit time. An empty template is valid and means "use the default".
func ValidateCommitMessageTemplate(tmpl string) error {
	known := (CommitMessageValues{}).substitutions()
	var unknown []string
	seen := map[string]bool{}
	for _, match := range placeholderRE.FindAllStringSubmatch(tmpl, -1) {
		name := match[1]
		if _, ok := known[name]; ok || seen[name] {
			continue
		}
		seen[name] = true
		unknown = append(unknown, name)
	}
	if len(unknown) == 0 {
		return nil
	}
	return fmt.Errorf("unsupported commit message variable(s): %s; available: %s",
		strings.Join(unknown, ", "), strings.Join(CommitMessageVariables(), ", "))
}

// RenderCommitMessage substitutes the supported variables. Unknown ones are
// left as written — validation is what rejects them; rendering stays total so a
// preview can show the user exactly what a template produces.
func RenderCommitMessage(tmpl string, v CommitMessageValues) string {
	subs := v.substitutions()
	return placeholderRE.ReplaceAllStringFunc(tmpl, func(match string) string {
		name := placeholderRE.FindStringSubmatch(match)[1]
		if value, ok := subs[name]; ok {
			return value
		}
		return match
	})
}

// CommitMessageFor is what a delivery action calls: render the template, or
// fall back to the built-in default when there is nothing usable. A template
// that is empty, invalid, or renders to whitespace falls back, because git
// refuses an empty commit message and failing the commit after the sync has
// already written files would leave the worktree half-delivered.
func CommitMessageFor(tmpl string, v CommitMessageValues) string {
	if strings.TrimSpace(tmpl) == "" || ValidateCommitMessageTemplate(tmpl) != nil {
		return DefaultCommitMessage(v.Feature, v.Now)
	}
	rendered := RenderCommitMessage(tmpl, v)
	if strings.TrimSpace(rendered) == "" {
		return DefaultCommitMessage(v.Feature, v.Now)
	}
	return rendered
}
