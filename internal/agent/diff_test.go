package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- 테스트 헬퍼 --------------------------------------------------------

// writeFile은 dir 기준 상대 경로 rel에 content를 쓰고 절대 경로를 돌려준다.
// 중간 디렉터리는 자동으로 만든다.
func writeFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// setModTime은 파일의 수정 시각을 고정해 인덱스 비교를 결정적으로 만든다.
func setModTime(t *testing.T, path string, ts time.Time) {
	t.Helper()
	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

// statOf는 인덱스 baseline을 만들 때 쓰는 크기/수정 시각을 읽어온다.
func statOf(t *testing.T, path string) (int64, int64) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.Size(), fi.ModTime().UnixNano()
}

// hashOf는 테스트용 해시 함수가 내용에 대해 만들어 내는 값과 동일한 값을
// 반환한다. 인덱스 baseline의 Hash 필드를 채울 때 쓴다.
func hashOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// countingHasher는 실제 파일 내용을 해싱하되 호출 횟수를 세는 hashFileFunc를
// 돌려준다. compare가 인덱스 빠른 경로를 탔는지 판별하는 데 쓴다.
func countingHasher(calls *int) hashFileFunc {
	return func(p string) (string, error) {
		*calls++
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		sum := sha256.Sum256(b)
		return hex.EncodeToString(sum[:]), nil
	}
}

// newIndexEntry는 FileIndexEntry의 내부 필드를 직접 채운다. 중첩 구조체 타입
// 이름에 의존하지 않도록 리터럴 대신 대입을 쓴다.
func newIndexEntry(agentSize, agentMod int64, agentHash string, wtSize, wtMod int64, wtHash string) FileIndexEntry {
	var e FileIndexEntry
	e.Agent.Size = agentSize
	e.Agent.ModTimeNano = agentMod
	e.Agent.Hash = agentHash
	e.Worktree.Size = wtSize
	e.Worktree.ModTimeNano = wtMod
	e.Worktree.Hash = wtHash
	return e
}

// changeMap은 결과 비교를 쉽게 하도록 경로 → 변경 타입 맵으로 바꾼다.
func changeMap(changes []Change) map[string]ChangeType {
	m := make(map[string]ChangeType, len(changes))
	for _, c := range changes {
		m[c.Path] = c.Type
	}
	return m
}

// findChange는 주어진 경로의 변경을 찾는다. 없으면 nil.
func findChange(changes []Change, path string) *Change {
	for i := range changes {
		if changes[i].Path == path {
			return &changes[i]
		}
	}
	return nil
}

// --- scanFiles ---------------------------------------------------------

// root가 없으면 에러 없이 빈 맵을 돌려준다.
func TestScanFilesMissingRootReturnsEmpty(t *testing.T) {
	got, err := scanFiles(filepath.Join(t.TempDir(), "nope"), NewIgnoreMatcher(nil), false, nil)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

// 무시 대상 파일과 디렉터리는 결과에서 빠지고, 경로는 슬래시로 정규화된다.
func TestScanFilesSkipsIgnoredAndNormalizesPaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "keep.txt", "a")
	writeFile(t, root, "nested/deep/keep.txt", "b")
	writeFile(t, root, "debug.log", "c")
	writeFile(t, root, "skipdir/inside.txt", "d")

	got, err := scanFiles(root, NewIgnoreMatcher([]string{"*.log", "skipdir"}), false, nil)
	if err != nil {
		t.Fatalf("scanFiles: %v", err)
	}

	want := []string{"keep.txt", "nested/deep/keep.txt"}
	if len(got) != len(want) {
		t.Fatalf("got %d entries (%v), want %d", len(got), got, len(want))
	}
	for _, rel := range want {
		if _, ok := got[rel]; !ok {
			t.Errorf("missing %q in %v", rel, got)
		}
	}
}

// withHashes가 true일 때만 해시를 채우고, false면 해시 함수를 호출하지 않는다.
func TestScanFilesHashesOnlyWhenRequested(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.txt", "hello")

	calls := 0
	withHash, err := scanFiles(root, NewIgnoreMatcher(nil), true, countingHasher(&calls))
	if err != nil {
		t.Fatalf("scanFiles(withHashes): %v", err)
	}
	if got := withHash["a.txt"].hash; got != hashOf("hello") {
		t.Errorf("hash = %q, want %q", got, hashOf("hello"))
	}
	if calls != 1 {
		t.Errorf("hash calls = %d, want 1", calls)
	}
	if got := withHash["a.txt"].size; got != int64(len("hello")) {
		t.Errorf("size = %d, want %d", got, len("hello"))
	}

	calls = 0
	noHash, err := scanFiles(root, NewIgnoreMatcher(nil), false, countingHasher(&calls))
	if err != nil {
		t.Fatalf("scanFiles(noHashes): %v", err)
	}
	if got := noHash["a.txt"].hash; got != "" {
		t.Errorf("hash = %q, want empty", got)
	}
	if calls != 0 {
		t.Errorf("hash calls = %d, want 0", calls)
	}
}

// 해시 함수 에러는 그대로 위로 전파된다.
func TestScanFilesPropagatesHashError(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.txt", "x")

	boom := errors.New("boom")
	_, err := scanFiles(root, NewIgnoreMatcher(nil), true, func(string) (string, error) {
		return "", boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

// --- compare: 인덱스 없는 폴백 경로 -------------------------------------

// source에만 있으면 ADDED, target에만 있으면 DELETED, 내용이 다르면 MODIFIED.
func TestCompareDetectsAddedDeletedModified(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeFile(t, source, "added.txt", "new")
	writeFile(t, source, "same.txt", "same")
	writeFile(t, target, "same.txt", "same")
	writeFile(t, source, "changed.txt", "v2")
	writeFile(t, target, "changed.txt", "v1")
	writeFile(t, target, "deleted.txt", "gone")

	calls := 0
	changes, hashed, err := compare("repo", source, target, NewIgnoreMatcher(nil), nil, nil, countingHasher(&calls))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	got := changeMap(changes)
	want := map[string]ChangeType{
		"added.txt":   Added,
		"changed.txt": Modified,
		"deleted.txt": Deleted,
	}
	if len(got) != len(want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	for path, typ := range want {
		if got[path] != typ {
			t.Errorf("%s = %q, want %q", path, got[path], typ)
		}
	}
	// 인덱스가 없으므로 양쪽 트리의 모든 파일(3 + 3)을 해싱한다.
	if hashed != 6 {
		t.Errorf("hashed = %d, want 6", hashed)
	}
	if hashed != calls {
		t.Errorf("hashed = %d, but hasher was called %d times", hashed, calls)
	}
}

// 변경 결과는 경로 사전순으로 정렬되고 Repo 이름이 채워진다.
func TestCompareSortsChangesAndSetsRepo(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	for _, rel := range []string{"c.txt", "a.txt", "b.txt"} {
		writeFile(t, source, rel, "x")
	}

	calls := 0
	changes, _, err := compare("myrepo", source, target, NewIgnoreMatcher(nil), nil, nil, countingHasher(&calls))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(changes) != 3 {
		t.Fatalf("len(changes) = %d, want 3", len(changes))
	}
	for i, want := range []string{"a.txt", "b.txt", "c.txt"} {
		if changes[i].Path != want {
			t.Errorf("changes[%d].Path = %q, want %q", i, changes[i].Path, want)
		}
		if changes[i].Repo != "myrepo" {
			t.Errorf("changes[%d].Repo = %q, want %q", i, changes[i].Repo, "myrepo")
		}
	}
}

// 민감 파일과 마스킹 대상은 각각 Risky/Masked 플래그로 표시된다.
func TestCompareMarksRiskyAndMasked(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeFile(t, source, ".env", "SECRET=1")
	writeFile(t, source, "config.yml", "masked")

	calls := 0
	changes, _, err := compare("repo", source, target, NewIgnoreMatcher(nil), map[string]bool{"config.yml": true}, nil, countingHasher(&calls))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}

	env := findChange(changes, ".env")
	if env == nil {
		t.Fatal("missing change for .env")
	}
	if !env.Risky {
		t.Error(".env Risky = false, want true")
	}
	if env.Masked {
		t.Error(".env Masked = true, want false")
	}

	cfg := findChange(changes, "config.yml")
	if cfg == nil {
		t.Fatal("missing change for config.yml")
	}
	if !cfg.Masked {
		t.Error("config.yml Masked = false, want true")
	}
	if cfg.Risky {
		t.Error("config.yml Risky = true, want false")
	}
}

// 인덱스가 없으면 마스킹 예외 규칙을 적용할 baseline이 없으므로, 마스킹된
// 파일이라도 변경으로 보고한다.
func TestCompareWithoutIndexReportsMaskedChange(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeFile(t, source, "config.yml", "agent")
	writeFile(t, target, "config.yml", "worktree")

	calls := 0
	changes, _, err := compare("repo", source, target, NewIgnoreMatcher(nil), map[string]bool{"config.yml": true}, nil, countingHasher(&calls))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(changes) != 1 || changes[0].Type != Modified {
		t.Fatalf("changes = %v, want one MODIFIED", changes)
	}
}

// --- compare: 인덱스 빠른 경로 -----------------------------------------

// 크기와 수정 시각이 baseline과 모두 일치하면 내용을 전혀 읽지 않는다.
func TestCompareIndexedFastPathSkipsHashing(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	sp := writeFile(t, source, "a.txt", "same")
	tp := writeFile(t, target, "a.txt", "same")
	ts := time.Unix(1700000000, 0)
	setModTime(t, sp, ts)
	setModTime(t, tp, ts)

	sSize, sMod := statOf(t, sp)
	tSize, tMod := statOf(t, tp)
	index := map[string]FileIndexEntry{
		"a.txt": newIndexEntry(sSize, sMod, hashOf("same"), tSize, tMod, hashOf("same")),
	}

	calls := 0
	changes, hashed, err := compare("repo", source, target, NewIgnoreMatcher(nil), nil, index, countingHasher(&calls))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("changes = %v, want none", changes)
	}
	if hashed != 0 {
		t.Errorf("hashed = %d, want 0 (fast path should read no content)", hashed)
	}
}

// 크기가 다르면 해싱 없이 곧바로 MODIFIED로 판정한다.
func TestCompareIndexedSizeMismatchSkipsHashing(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	sp := writeFile(t, source, "a.txt", "longer content")
	tp := writeFile(t, target, "a.txt", "short")
	sSize, sMod := statOf(t, sp)
	tSize, tMod := statOf(t, tp)
	index := map[string]FileIndexEntry{
		"a.txt": newIndexEntry(sSize, sMod, hashOf("longer content"), tSize, tMod, hashOf("short")),
	}

	calls := 0
	changes, hashed, err := compare("repo", source, target, NewIgnoreMatcher(nil), nil, index, countingHasher(&calls))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(changes) != 1 || changes[0].Type != Modified {
		t.Fatalf("changes = %v, want one MODIFIED", changes)
	}
	if hashed != 0 {
		t.Errorf("hashed = %d, want 0 (size mismatch is decisive)", hashed)
	}
}

// 수정 시각이 baseline과 어긋나면 후보로 보고 확인용 해시를 계산한다.
// 내용이 실제로 같다면 변경으로 보고하지 않는다(같은 파일을 source/target
// 두 번 읽으므로 카운트는 2).
func TestCompareIndexedStaleModTimeConfirmsByHash(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	sp := writeFile(t, source, "a.txt", "same")
	tp := writeFile(t, target, "a.txt", "same")
	sSize, _ := statOf(t, sp)
	tSize, tMod := statOf(t, tp)
	index := map[string]FileIndexEntry{
		// Agent 쪽 수정 시각만 일부러 어긋나게 둔다.
		"a.txt": newIndexEntry(sSize, 1, hashOf("same"), tSize, tMod, hashOf("same")),
	}

	calls := 0
	changes, hashed, err := compare("repo", source, target, NewIgnoreMatcher(nil), nil, index, countingHasher(&calls))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("changes = %v, want none (contents are identical)", changes)
	}
	if hashed != 2 {
		t.Errorf("hashed = %d, want 2 (source + target read of one file)", hashed)
	}
}

// 크기는 같지만 내용이 다르면 해시 확인을 거쳐 MODIFIED로 판정한다.
func TestCompareIndexedSameSizeDifferentContent(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	sp := writeFile(t, source, "a.txt", "AAAA")
	tp := writeFile(t, target, "a.txt", "BBBB")
	sSize, _ := statOf(t, sp)
	tSize, tMod := statOf(t, tp)
	index := map[string]FileIndexEntry{
		"a.txt": newIndexEntry(sSize, 1, hashOf("AAAA"), tSize, tMod, hashOf("BBBB")),
	}

	calls := 0
	changes, hashed, err := compare("repo", source, target, NewIgnoreMatcher(nil), nil, index, countingHasher(&calls))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(changes) != 1 || changes[0].Type != Modified {
		t.Fatalf("changes = %v, want one MODIFIED", changes)
	}
	if hashed != 2 {
		t.Errorf("hashed = %d, want 2", hashed)
	}
}

// 마스킹된 파일이라도 baseline과 크기/시각이 맞으면 빠른 경로로 건너뛴다.
// (해시가 서로 달라도 마스킹이면 통과)
func TestCompareIndexedMaskedFastPathSkipsDifferingHashes(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	sp := writeFile(t, source, "config.yml", "AAAA")
	tp := writeFile(t, target, "config.yml", "BBBB")
	sSize, sMod := statOf(t, sp)
	tSize, tMod := statOf(t, tp)
	index := map[string]FileIndexEntry{
		"config.yml": newIndexEntry(sSize, sMod, hashOf("AAAA"), tSize, tMod, hashOf("BBBB")),
	}

	calls := 0
	changes, hashed, err := compare("repo", source, target, NewIgnoreMatcher(nil), map[string]bool{"config.yml": true}, index, countingHasher(&calls))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("changes = %v, want none (masked baseline)", changes)
	}
	if hashed != 0 {
		t.Errorf("hashed = %d, want 0", hashed)
	}
}

// 마스킹 규칙: 에이전트 사본이 baseline 그대로면 워크트리와 달라도 변경이
// 아니다. 여기서는 수정 시각을 어긋나게 해 빠른 경로를 우회시킨다.
func TestCompareIndexedMaskedUnchangedAgentCopyIsNotAChange(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	sp := writeFile(t, source, "config.yml", "AAAA")
	tp := writeFile(t, target, "config.yml", "BBBB")
	sSize, _ := statOf(t, sp)
	tSize, tMod := statOf(t, tp)
	index := map[string]FileIndexEntry{
		"config.yml": newIndexEntry(sSize, 1, hashOf("AAAA"), tSize, tMod, hashOf("BBBB")),
	}

	calls := 0
	changes, _, err := compare("repo", source, target, NewIgnoreMatcher(nil), map[string]bool{"config.yml": true}, index, countingHasher(&calls))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("changes = %v, want none (agent copy matches baseline)", changes)
	}
}

// 반대로 에이전트 사본이 baseline에서 벗어났다면 마스킹돼 있어도 변경이다.
func TestCompareIndexedMaskedEditedAgentCopyIsAChange(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	sp := writeFile(t, source, "config.yml", "EDITED")
	tp := writeFile(t, target, "config.yml", "ORIGIN")
	sSize, _ := statOf(t, sp)
	tSize, tMod := statOf(t, tp)
	index := map[string]FileIndexEntry{
		// baseline의 Agent 해시는 편집 전 내용이다.
		"config.yml": newIndexEntry(sSize, 1, hashOf("BEFORE"), tSize, tMod, hashOf("ORIGIN")),
	}

	calls := 0
	changes, _, err := compare("repo", source, target, NewIgnoreMatcher(nil), map[string]bool{"config.yml": true}, index, countingHasher(&calls))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(changes) != 1 || changes[0].Type != Modified {
		t.Fatalf("changes = %v, want one MODIFIED", changes)
	}
	if !changes[0].Masked {
		t.Error("Masked = false, want true")
	}
}

// 인덱스에 없는 파일은 빠른 경로 대상이 아니므로 정상적으로 비교된다.
func TestCompareIndexedUnknownFileStillCompared(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeFile(t, source, "known.txt", "same")
	writeFile(t, target, "known.txt", "same")
	writeFile(t, source, "unknown.txt", "AAAA")
	writeFile(t, target, "unknown.txt", "BBBB")

	sSize, sMod := statOf(t, filepath.Join(source, "known.txt"))
	tSize, tMod := statOf(t, filepath.Join(target, "known.txt"))
	index := map[string]FileIndexEntry{
		"known.txt": newIndexEntry(sSize, sMod, hashOf("same"), tSize, tMod, hashOf("same")),
	}

	calls := 0
	changes, hashed, err := compare("repo", source, target, NewIgnoreMatcher(nil), nil, index, countingHasher(&calls))
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(changes) != 1 || changes[0].Path != "unknown.txt" || changes[0].Type != Modified {
		t.Fatalf("changes = %v, want one MODIFIED unknown.txt", changes)
	}
	// known.txt는 0회, unknown.txt만 source/target 2회.
	if hashed != 2 {
		t.Errorf("hashed = %d, want 2", hashed)
	}
}

// --- 공개 래퍼 ----------------------------------------------------------

// Compare는 실제 SHA256 해시로 폴백 경로를 수행한다.
func TestComparePublicWrapper(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeFile(t, source, "a.txt", "v2")
	writeFile(t, target, "a.txt", "v1")

	changes, hashed, err := Compare("repo", source, target, NewIgnoreMatcher(nil), nil)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(changes) != 1 || changes[0].Type != Modified {
		t.Fatalf("changes = %v, want one MODIFIED", changes)
	}
	if hashed == 0 {
		t.Error("hashed = 0, want > 0 (no index means whole-tree hashing)")
	}
}

// 인덱스가 비어 있으면 CompareIndexed는 Compare와 동일하게 동작한다.
func TestCompareIndexedEmptyIndexFallsBack(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeFile(t, source, "a.txt", "v2")
	writeFile(t, target, "a.txt", "v1")

	changes, hashed, err := CompareIndexed("repo", source, target, NewIgnoreMatcher(nil), nil, map[string]FileIndexEntry{})
	if err != nil {
		t.Fatalf("CompareIndexed: %v", err)
	}
	if len(changes) != 1 || changes[0].Type != Modified {
		t.Fatalf("changes = %v, want one MODIFIED", changes)
	}
	if hashed == 0 {
		t.Error("hashed = 0, want > 0 (empty index falls back to hashing)")
	}
}

// --- IsRisky ------------------------------------------------------------

func TestIsRisky(t *testing.T) {
	tests := []struct {
		rel  string
		want bool
	}{
		{".env", true},
		{".env.local", true},
		{"server.pem", true},
		{"id_rsa.key", true},
		{"keystore.p12", true},
		{"keystore.jks", true},
		{"application-secret.yml", true},
		{"application-local.yml", true},
		{"agentsafe.yaml", true},
		{"mask.json", true},
		{".agentignore", true},
		{"secrets.yml", true},
		{"credentials.yml", true},
		{"main.go", false},
		{"README.md", false},
		{"application.yml", false},
		{"env.txt", false},
	}
	for _, tc := range tests {
		if got := IsRisky(tc.rel); got != tc.want {
			t.Errorf("IsRisky(%q) = %v, want %v", tc.rel, got, tc.want)
		}
	}
}

// --- PrintChanges -------------------------------------------------------

// 출력 자체를 가로채지는 않고, 빈 목록·플래그 조합에서 패닉 없이 동작하는지
// 확인한다. output 패키지에 writer 주입 훅이 생기면 문자열 단언으로 강화할 것.
func TestPrintChangesSmoke(t *testing.T) {
	PrintChanges("feature-x", map[string][]Change{
		"repo-b": {},
		"repo-a": {
			{Repo: "repo-a", Type: Added, Path: "a.txt"},
			{Repo: "repo-a", Type: Modified, Path: ".env", Risky: true},
			{Repo: "repo-a", Type: Deleted, Path: "config.yml", Masked: true},
			{Repo: "repo-a", Type: Modified, Path: "both.key", Risky: true, Masked: true},
		},
	})
}
