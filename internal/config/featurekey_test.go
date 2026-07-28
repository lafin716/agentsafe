package config

import "testing"

// TestFeatureKeyAlwaysUsesFeatHash는 영문·한글·공백·하이픈 등 어떤 형태의 이름이든
// FeatureKey가 항상 "feat-" + shortHash(name) 형식을 따르는지 검증한다.
func TestFeatureKeyAlwaysUsesFeatHash(t *testing.T) {
	for _, name := range []string{"coupon-v2", "쿠폰결제", "쿠폰-v2", "My Feature"} {
		want := "feat-" + shortHash(name)
		if got := FeatureKey(name); got != want {
			t.Errorf("FeatureKey(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestFeatureKeyDeterministicAndDistinct는 FeatureKey가 같은 입력에 대해 항상 같은 키를 내고
// 서로 다른 이름에는 서로 다른 키를 내는지 검증한다.
func TestFeatureKeyDeterministicAndDistinct(t *testing.T) {
	if FeatureKey("쿠폰") != FeatureKey("쿠폰") {
		t.Error("FeatureKey is not deterministic for the same input")
	}
	if FeatureKey("쿠폰") == FeatureKey("배송") {
		t.Error("distinct names produced the same key")
	}
}
