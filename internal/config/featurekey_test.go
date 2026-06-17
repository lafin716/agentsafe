package config

import "testing"

func TestFeatureKeyAlwaysUsesFeatHash(t *testing.T) {
	for _, name := range []string{"coupon-v2", "쿠폰결제", "쿠폰-v2", "My Feature"} {
		want := "feat-" + shortHash(name)
		if got := FeatureKey(name); got != want {
			t.Errorf("FeatureKey(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestFeatureKeyDeterministicAndDistinct(t *testing.T) {
	if FeatureKey("쿠폰") != FeatureKey("쿠폰") {
		t.Error("FeatureKey is not deterministic for the same input")
	}
	if FeatureKey("쿠폰") == FeatureKey("배송") {
		t.Error("distinct names produced the same key")
	}
}
