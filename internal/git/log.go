package git

import (
	"strconv"
	"strings"
)

// Commit Graph reading. Every Repo Worktree of a repository shares its Main
// Clone's object database, so one `git log` run in main/<repo> already sees
// every Feature branch — there is no need to walk the worktrees. That matters
// because a git subprocess costs on the order of a second on Windows machines
// running antivirus, and this is a hot path.

// RefKind distinguishes the three ref namespaces a decoration can name.
type RefKind string

const (
	RefHead   RefKind = "head"
	RefRemote RefKind = "remote"
	RefTag    RefKind = "tag"
)

// Ref is one decoration pointing at a commit.
type Ref struct {
	Name string  `json:"name" yaml:"name"`
	Kind RefKind `json:"kind" yaml:"kind"`
}

// RefTip is a ref and the commit it currently points at. Used to notice refs
// whose tip falls outside the commit window so the graph can say so instead of
// pretending the branch does not exist.
type RefTip struct {
	Name string  `json:"name" yaml:"name"`
	Kind RefKind `json:"kind" yaml:"kind"`
	SHA  string  `json:"sha"  yaml:"sha"`
}

// Commit is one node of a Commit Graph. Parents is ordered as git reports it,
// so Parents[0] is the first parent.
type Commit struct {
	SHA         string   `json:"sha"         yaml:"sha"`
	Parents     []string `json:"parents"     yaml:"parents"`
	AuthorName  string   `json:"authorName"  yaml:"authorName"`
	AuthorEmail string   `json:"authorEmail" yaml:"authorEmail"`
	AuthorDate  string   `json:"authorDate"  yaml:"authorDate"`
	Subject     string   `json:"subject"     yaml:"subject"`
	Refs        []Ref    `json:"refs"        yaml:"refs"`
	IsHead      bool     `json:"isHead"      yaml:"isHead"`
}

const (
	logFieldSep  = "\x00"
	logRecordSep = "\x1e"
	// The same two bytes as git's %xNN escapes. They have to be spelled this way
	// in a --format argument: an argv entry containing a NUL is rejected before
	// the process starts on Windows (UTF16PtrFromString answers EINVAL, which
	// surfaces as "fork/exec ... git.exe: invalid argument"), so the escape is
	// what puts the separator in the output instead of in the argument.
	logFieldSepEsc  = "%x00"
	logRecordSepEsc = "%x1e"
	// logFieldCount is how many fields logFormat emits per commit. A record with
	// any other count is treated as truncated output and dropped.
	logFieldCount = 7
)

// logFormat asks for NUL-separated fields and a record separator that cannot
// appear in a commit subject, so subjects containing commas, newlines-escaped
// text or "->" survive parsing intact.
const logFormat = "--format=%H" + logFieldSepEsc + "%P" + logFieldSepEsc + "%an" +
	logFieldSepEsc + "%ae" + logFieldSepEsc + "%aI" + logFieldSepEsc + "%D" +
	logFieldSepEsc + "%s" + logRecordSepEsc

// refListFormat pairs a ref with its tip. for-each-ref spells a literal byte
// "%00" rather than "%x00", and needs the escape for the same reason logFormat
// does.
const refListFormat = "--format=%(refname)%00%(objectname)"

// LogRefArgs turns the caller's ref selection into git rev arguments. The
// default is the managed set — a repository's Base Branches plus the Feature
// branches that have a Repo Worktree here — because a real repository can carry
// hundreds of origin refs that would bury them.
func LogRefArgs(managed []string, allBranches bool) []string {
	if allBranches {
		return []string{"--all"}
	}
	args := make([]string, 0, len(managed))
	for _, r := range managed {
		if r = strings.TrimSpace(r); r != "" {
			args = append(args, r)
		}
	}
	return args
}

// Log reads up to limit commits reachable from refs, newest first in
// topological order. Missing refs are ignored rather than failing the whole
// read, so a Base Branch that was never fetched does not blank the graph.
func Log(repoPath string, refs []string, limit int) ([]Commit, error) {
	if limit <= 0 {
		limit = 300
	}
	args := []string{
		"log", "--topo-order", "--decorate=full", logFormat,
		"-n", strconv.Itoa(limit),
		// Drops revs that do not resolve instead of aborting the whole read, so a
		// Base Branch that was never fetched does not blank the graph. It has to
		// precede the revs: placed after them, git has already rejected the
		// unknown name ("ambiguous argument") by the time it is parsed.
		"--ignore-missing",
	}
	args = append(args, refs...)
	res, err := Run(repoPath, args...)
	if err != nil {
		return nil, err
	}
	return ParseLog(res.Stdout), nil
}

// ParseLog parses the output of Log's format string.
func ParseLog(raw string) []Commit {
	records := strings.Split(raw, logRecordSep)
	commits := make([]Commit, 0, len(records))
	for _, record := range records {
		// git separates records with logRecordSep; any newline around one is
		// incidental whitespace from the terminal, not part of a field.
		record = strings.Trim(record, "\r\n")
		if record == "" {
			continue
		}
		fields := strings.Split(record, logFieldSep)
		if len(fields) != logFieldCount {
			continue
		}
		isHead, refs := parseDecorations(fields[5])
		commits = append(commits, Commit{
			SHA:         fields[0],
			Parents:     splitSHAs(fields[1]),
			AuthorName:  fields[2],
			AuthorEmail: fields[3],
			AuthorDate:  fields[4],
			Subject:     fields[6],
			Refs:        refs,
			IsHead:      isHead,
		})
	}
	return commits
}

// splitSHAs splits git's space-separated parent list, always returning a
// non-nil slice so a root commit serializes as [] rather than null — which
// strings.Fields alone does not guarantee.
func splitSHAs(raw string) []string {
	if fields := strings.Fields(raw); len(fields) > 0 {
		return fields
	}
	return []string{}
}

// ShortSHA abbreviates a commit id for display. Seven characters is what git
// itself uses by default.
func ShortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// parseDecorations reads a --decorate=full %D value such as
// "HEAD -> refs/heads/main, refs/remotes/origin/main, refs/tags/v1".
// Full refnames are unambiguous, which is why Log asks for them rather than
// guessing a namespace from a short name like "origin/main".
func parseDecorations(raw string) (bool, []Ref) {
	refs := []Ref{}
	isHead := false
	for _, part := range strings.Split(raw, ", ") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// "HEAD -> refs/heads/x" marks the checked-out branch; a bare "HEAD"
		// marks a detached head, which is what an Interrupted Integration looks
		// like mid-rebase.
		if part == "HEAD" {
			isHead = true
			continue
		}
		if rest, ok := strings.CutPrefix(part, "HEAD -> "); ok {
			isHead = true
			part = rest
		}
		if ref, ok := refFromFullName(part); ok {
			refs = append(refs, ref)
		}
	}
	return isHead, refs
}

func refFromFullName(full string) (Ref, bool) {
	switch {
	case strings.HasPrefix(full, "refs/heads/"):
		return Ref{Name: strings.TrimPrefix(full, "refs/heads/"), Kind: RefHead}, true
	case strings.HasPrefix(full, "refs/remotes/"):
		return Ref{Name: strings.TrimPrefix(full, "refs/remotes/"), Kind: RefRemote}, true
	case strings.HasPrefix(full, "refs/tags/"):
		return Ref{Name: strings.TrimPrefix(full, "refs/tags/"), Kind: RefTag}, true
	}
	return Ref{}, false
}

// ListRefTips returns every local branch, origin-side branch and tag with the
// commit it points at. One call, so the graph can report which refs fall
// outside the commit window without probing them individually.
func ListRefTips(repoPath string) ([]RefTip, error) {
	out, err := Output(repoPath,
		"for-each-ref", refListFormat,
		"refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return nil, err
	}
	return ParseRefList(out), nil
}

// ParseRefList parses ListRefTips output. The symbolic origin/HEAD ref is
// dropped: it duplicates whichever branch it follows and is not something a
// user acts on.
func ParseRefList(raw string) []RefTip {
	tips := []RefTip{}
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		name, sha, ok := strings.Cut(line, logFieldSep)
		if !ok {
			continue
		}
		ref, ok := refFromFullName(name)
		if !ok {
			continue
		}
		if ref.Kind == RefRemote && strings.HasSuffix(ref.Name, "/HEAD") {
			continue
		}
		tips = append(tips, RefTip{Name: ref.Name, Kind: ref.Kind, SHA: sha})
	}
	return tips
}

// ShowFileAtRev reads one path's content as of a revision. Reports exists=false
// for a path that is not in that revision — a newly added file has no HEAD side,
// and that is a normal thing to render as an empty panel rather than an error.
//
// The path is passed with forward slashes because git's <rev>:<path> syntax uses
// them on every platform; a Windows-style path would simply not be found.
func ShowFileAtRev(worktreePath, rev, relPath string) (string, bool, error) {
	res, err := Run(worktreePath, "show", rev+":"+strings.ReplaceAll(relPath, "\\", "/"))
	if err != nil {
		// git says "path 'x' does not exist in 'HEAD'", or "exists on disk, but
		// not in 'HEAD'" — both mean the same thing here.
		if strings.Contains(res.Stderr, "does not exist in") ||
			strings.Contains(res.Stderr, "exists on disk, but not in") {
			return "", false, nil
		}
		return "", false, err
	}
	return res.Stdout, true, nil
}

// CommitFileChange is one path touched by a commit.
type CommitFileChange struct {
	// Status is git's name-status letter: A, M, D, R, C or T.
	Status string `json:"status" yaml:"status"`
	Path   string `json:"path"   yaml:"path"`
	// OldPath is set for renames and copies.
	OldPath string `json:"oldPath,omitempty" yaml:"oldPath,omitempty"`
}

// CommitFiles lists the paths a commit changed against its first parent. Called
// lazily when a user selects a commit, since it costs a git subprocess.
func CommitFiles(repoPath, sha string) ([]CommitFileChange, error) {
	res, err := Run(repoPath, "show", "--name-status", "--find-renames",
		"--format=", "-z", "--no-color", sha)
	if err != nil {
		return nil, err
	}
	return ParseNameStatusZ(res.Stdout), nil
}

// ParseNameStatusZ parses NUL-separated `--name-status -z` output. Renames and
// copies emit three records (status, old path, new path); everything else emits
// two, which is why this cannot be a plain pairwise split.
func ParseNameStatusZ(raw string) []CommitFileChange {
	fields := strings.Split(strings.Trim(raw, "\x00"), "\x00")
	changes := []CommitFileChange{}
	for i := 0; i < len(fields); {
		status := strings.TrimSpace(fields[i])
		if status == "" {
			i++
			continue
		}
		letter := status[:1]
		if letter == "R" || letter == "C" {
			if i+2 >= len(fields) {
				break
			}
			changes = append(changes, CommitFileChange{
				Status:  letter,
				OldPath: fields[i+1],
				Path:    fields[i+2],
			})
			i += 3
			continue
		}
		if i+1 >= len(fields) {
			break
		}
		changes = append(changes, CommitFileChange{Status: letter, Path: fields[i+1]})
		i += 2
	}
	return changes
}
