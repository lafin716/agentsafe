package config

import (
	"crypto/sha1"
	"encoding/hex"
)

// FeatureKey는 feature 이름으로부터 ASCII 안전한 안정적인 폴더 키를 만든다.
// 이 키는 디스크상의 워크트리/에이전트 폴더명에 쓰이고, 원래 feature 이름은
// Git 브랜치와 메타데이터에 그대로 사용된다.
//
// FeatureKey는 기존 feature 전체에 대한 전역 유일성을 보장하지 않는다.
// 파일시스템을 다루는 호출자는 키 충돌을 별도로 처리해야 한다.
func FeatureKey(name string) string {
	return "feat-" + shortHash(name)
}

// shortHash는 s의 SHA-1 해시에서 앞 6자리 16진수 문자를 돌려준다.
func shortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:6]
}
