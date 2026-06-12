package config

import (
	"strings"
	"testing"
)

func TestFeatureKeyCleanASCIIUnchanged(t *testing.T) {
	for _, name := range []string{"coupon-v2", "backend", "Feature_1", "a.b.c"} {
		if got := FeatureKey(name); got != name {
			t.Errorf("FeatureKey(%q) = %q, want unchanged", name, got)
		}
	}
}

func TestFeatureKeyPureHangulUsesHashFallback(t *testing.T) {
	got := FeatureKey("쿠폰결제")
	want := "feature-" + shortHash("쿠폰결제")
	if got != want {
		t.Errorf("FeatureKey(쿠폰결제) = %q, want %q", got, want)
	}
	if strings.ContainsAny(got, "쿠폰결제") {
		t.Errorf("key %q still contains Hangul", got)
	}
}

func TestFeatureKeyMixedKeepsASCIIPartWithHash(t *testing.T) {
	got := FeatureKey("쿠폰-v2")
	want := "v2-" + shortHash("쿠폰-v2")
	if got != want {
		t.Errorf("FeatureKey(쿠폰-v2) = %q, want %q", got, want)
	}
}

func TestFeatureKeySpacesAndCaseSlugified(t *testing.T) {
	got := FeatureKey("My Feature")
	want := "my-feature-" + shortHash("My Feature")
	if got != want {
		t.Errorf("FeatureKey(My Feature) = %q, want %q", got, want)
	}
}

func TestFeatureKeyDeterministicAndDistinct(t *testing.T) {
	if FeatureKey("쿠폰") != FeatureKey("쿠폰") {
		t.Error("FeatureKey is not deterministic for the same input")
	}
	if FeatureKey("쿠폰") == FeatureKey("배송") {
		t.Error("distinct Hangul names produced the same key")
	}
}
