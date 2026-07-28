package agent

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/agentsafe/agentsafe/internal/fsutil"
	"github.com/agentsafe/agentsafe/internal/output"
)

type ChangeType string

const (
	Added    ChangeType = "ADDED"
	Modified ChangeType = "MODIFIED"
	Deleted  ChangeType = "DELETED"
)

type Change struct {
	Repo   string     `json:"repo"   yaml:"repo"`
	Type   ChangeType `json:"type"   yaml:"type"`
	Path   string     `json:"path"   yaml:"path"`
	Risky  bool       `json:"risky"  yaml:"risky"`
	Masked bool       `json:"masked" yaml:"masked"`
}

type fileInfo struct {
	size        int64
	modTimeNano int64
	hash        string
}

type hashFileFunc func(string) (string, error)

// scanFiles는 root 하위를 순회하며 무시 대상이 아닌 파일의 메타데이터를
// 상대 경로 기준으로 수집한다. withHashes가 true면 각 파일의 내용 해시까지
// 함께 계산한다. root가 존재하지 않으면 빈 맵을 반환한다.
func scanFiles(root string, matcher IgnoreMatcher, withHashes bool, hashFile hashFileFunc) (map[string]fileInfo, error) {
	out := map[string]fileInfo{}
	if _, err := os.Stat(root); err != nil {
		return out, nil
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if matcher.Match(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h := ""
		if withHashes {
			h, err = hashFile(path)
			if err != nil {
				return err
			}
		}
		out[rel] = fileInfo{size: info.Size(), modTimeNano: info.ModTime().UnixNano(), hash: h}
		return nil
	})
	return out, err
}

// Compare는 감지된 변경 목록과 수행한 내용 해시 연산 횟수를 반환한다
// (자세한 내용은 compare 참고).
func Compare(repoName, source, target string, matcher IgnoreMatcher, masked map[string]bool) ([]Change, int, error) {
	return compare(repoName, source, target, matcher, masked, nil, fsutil.SHA256File)
}

// CompareIndexed는 prepare 시점에 수집한 stat 메타데이터를 Git의 인덱스처럼
// 활용한다. 크기와 수정 시각이 두 스냅샷 모두와 일치하는 파일은 내용을 읽지
// 않고 건너뛰며, 변경 가능성이 있는 파일만 해시로 확인한다. 수행한 내용 해시
// 연산 횟수도 함께 반환하므로, 호출자는 인덱스 기반 빠른 경로(거의 0회)와
// 전체 트리 해시 폴백을 구분할 수 있다.
func CompareIndexed(repoName, source, target string, matcher IgnoreMatcher, masked map[string]bool, index map[string]FileIndexEntry) ([]Change, int, error) {
	if len(index) == 0 {
		return Compare(repoName, source, target, matcher, masked)
	}
	return compare(repoName, source, target, matcher, masked, index, fsutil.SHA256File)
}

// compare는 감지된 변경 목록과 내용 해시 연산 횟수를 반환한다. 이 횟수는
// hashFile 호출을 모두 합산한 값이며(인덱스가 없는 폴백에서는 스캔한 모든
// 파일, 인덱스 후보에 대해서는 source/target 읽기), 같은 파일이라도 source와
// target을 읽으면 2회로 센다. compare가 느린 원인을 진단할 때 중요한 것은
// 파일 식별이 아니라 연산 횟수의 규모이기 때문이다.
func compare(repoName, source, target string, matcher IgnoreMatcher, masked map[string]bool, index map[string]FileIndexEntry, hashFile hashFileFunc) ([]Change, int, error) {
	hashed := 0
	countHash := func(p string) (string, error) {
		hashed++
		return hashFile(p)
	}
	withHashes := len(index) == 0
	s, err := scanFiles(source, matcher, withHashes, countHash)
	if err != nil {
		return nil, hashed, err
	}
	t, err := scanFiles(target, matcher, withHashes, countHash)
	if err != nil {
		return nil, hashed, err
	}
	keys := map[string]bool{}
	for k := range s {
		keys[k] = true
	}
	for k := range t {
		keys[k] = true
	}
	var sorted []string
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	var changes []Change
	for _, k := range sorted {
		si, sok := s[k]
		ti, tok := t[k]
		var change *Change
		switch {
		case sok && !tok:
			c := Change{Repo: repoName, Type: Added, Path: k, Risky: IsRisky(k), Masked: masked[k]}
			change = &c
		case !sok && tok:
			c := Change{Repo: repoName, Type: Deleted, Path: k, Risky: IsRisky(k), Masked: masked[k]}
			change = &c
		case withHashes:
			if si.size != ti.size || si.hash != ti.hash {
				c := Change{Repo: repoName, Type: Modified, Path: k, Risky: IsRisky(k), Masked: masked[k]}
				change = &c
			}
		default:
			baseline, indexed := index[k]
			if indexed &&
				si.size == baseline.Agent.Size &&
				si.modTimeNano == baseline.Agent.ModTimeNano &&
				ti.size == baseline.Worktree.Size &&
				ti.modTimeNano == baseline.Worktree.ModTimeNano &&
				(baseline.Agent.Hash == baseline.Worktree.Hash || masked[k]) {
				continue
			}
			if si.size != ti.size {
				c := Change{Repo: repoName, Type: Modified, Path: k, Risky: IsRisky(k), Masked: masked[k]}
				change = &c
				break
			}
			sourceHash, err := countHash(filepath.Join(source, filepath.FromSlash(k)))
			if err != nil {
				return nil, hashed, err
			}
			targetHash, err := countHash(filepath.Join(target, filepath.FromSlash(k)))
			if err != nil {
				return nil, hashed, err
			}
			si.hash, ti.hash = sourceHash, targetHash
			if sourceHash != targetHash {
				c := Change{Repo: repoName, Type: Modified, Path: k, Risky: IsRisky(k), Masked: masked[k]}
				change = &c
			}
		}
		if change == nil {
			continue
		}
		// 기존 마스킹 규칙 유지: prepare된 에이전트 사본이 그대로라면 실제
		// 워크트리와 내용이 달라도 사용자가 수정한 것으로 보지 않는다.
		if change.Masked && sok {
			currentHash := si.hash
			if currentHash == "" {
				currentHash, err = countHash(filepath.Join(source, filepath.FromSlash(k)))
				if err != nil {
					return nil, hashed, err
				}
			}
			if baseline, ok := index[k]; ok && baseline.Agent.Hash == currentHash {
				continue
			}
		}
		changes = append(changes, *change)
	}
	return changes, hashed, nil
}

// PrintChanges는 저장소별 변경 목록을 저장소 이름 순으로 정렬해 출력한다.
// 변경이 없는 저장소는 NO CHANGES로 표시하고, 각 변경에는 RISKY/MASKED
// 플래그를 덧붙인다.
func PrintChanges(feature string, byRepo map[string][]Change) {
	output.Printf("Feature: %s\n\n", feature)
	repos := make([]string, 0, len(byRepo))
	for r := range byRepo {
		repos = append(repos, r)
	}
	sort.Strings(repos)
	for _, r := range repos {
		output.Printf("[%s]\n", r)
		if len(byRepo[r]) == 0 {
			output.Println("NO CHANGES")
			output.Println()
			continue
		}
		for _, c := range byRepo[r] {
			flags := ""
			if c.Risky {
				flags += " RISKY"
			}
			if c.Masked {
				flags += " MASKED"
			}
			output.Printf("%-8s %s%s\n", c.Type, c.Path, flags)
		}
		output.Println()
	}
}

// IsRisky는 상대 경로가 자격 증명·비밀 설정 파일 같은 민감 파일 패턴에
// 해당하는지 판단한다.
func IsRisky(rel string) bool {
	return NewIgnoreMatcher([]string{".env", ".env.*", "*.pem", "*.key", "*.p12", "*.jks", "application-secret.yml", "application-local.yml", "agentsafe.yaml", "mask.json", ".agentignore", "secrets.yml", "credentials.yml"}).Match(rel, false)
}
