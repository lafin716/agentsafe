package agent

import (
	"strings"
	"testing"
	"time"
)

func fixedValues() CommitMessageValues {
	return CommitMessageValues{
		Feature: "coupon-v2",
		Branch:  "feature/coupon-v2",
		Base:    "develop",
		Now:     time.Date(2026, 8, 3, 14, 12, 5, 0, time.FixedZone("KST", 9*3600)),
	}
}

func TestRenderCommitMessageSubstitutesEveryVariable(t *testing.T) {
	got := RenderCommitMessage(
		"{{feature}} | {{branch}} | {{base}} | {{timestamp}} | {{date}} | {{time}}",
		fixedValues())

	want := "coupon-v2 | feature/coupon-v2 | develop | " +
		"2026-08-03T14:12:05+09:00 | 2026-08-03 | 14:12:05"
	if got != want {
		t.Errorf("rendered = %q, want %q", got, want)
	}
}

func TestRenderCommitMessageToleratesSpacesInsideBraces(t *testing.T) {
	got := RenderCommitMessage("chore({{ feature }}): sync", fixedValues())

	if got != "chore(coupon-v2): sync" {
		t.Errorf("rendered = %q", got)
	}
}

func TestRenderCommitMessageLeavesTextWithoutPlaceholdersAlone(t *testing.T) {
	got := RenderCommitMessage("wip", fixedValues())

	if got != "wip" {
		t.Errorf("rendered = %q, want wip", got)
	}
}

func TestValidateCommitMessageTemplateAcceptsSupportedVariables(t *testing.T) {
	for _, tmpl := range []string{
		"",
		"wip",
		"agent({{feature}}): sync at {{timestamp}}",
		"{{branch}} onto {{base}} on {{date}} {{time}}",
		"{{ feature }}",
		// A brace that opens nothing is literal text, not a placeholder.
		"100% {{feature}} {not a var}",
	} {
		if err := ValidateCommitMessageTemplate(tmpl); err != nil {
			t.Errorf("ValidateCommitMessageTemplate(%q) = %v, want nil", tmpl, err)
		}
	}
}

func TestValidateCommitMessageTemplateRejectsUnknownVariables(t *testing.T) {
	err := ValidateCommitMessageTemplate("agent({{repo}}): {{changeCount}} files")
	if err == nil {
		t.Fatal("want an error naming the unsupported variables")
	}
	// repo and changeCount are deliberately unsupported: rendering them would
	// need a per-repository message, which feature.Commit does not take.
	for _, want := range []string{"repo", "changeCount"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
	// The message has to say what the user may use instead.
	if !strings.Contains(err.Error(), "feature") {
		t.Errorf("error %q does not list the supported variables", err)
	}
}

func TestCommitMessageForFallsBackWhenTheTemplateIsEmpty(t *testing.T) {
	got := CommitMessageFor("", fixedValues())

	if !strings.Contains(got, "coupon-v2") {
		t.Errorf("fallback = %q, want it to name the feature", got)
	}
	if got != DefaultCommitMessage("coupon-v2", fixedValues().Now) {
		t.Errorf("fallback = %q, want the hardcoded default", got)
	}
}

func TestCommitMessageForFallsBackWhenTheTemplateIsInvalid(t *testing.T) {
	// A config edited by hand can hold anything; a bad template must not produce
	// a commit message with a literal {{repo}} in it.
	got := CommitMessageFor("agent({{repo}}): sync", fixedValues())

	if strings.Contains(got, "{{") {
		t.Errorf("message = %q, want no unrendered placeholder", got)
	}
	if got != DefaultCommitMessage("coupon-v2", fixedValues().Now) {
		t.Errorf("message = %q, want the hardcoded default", got)
	}
}

func TestCommitMessageForRendersAValidTemplate(t *testing.T) {
	got := CommitMessageFor("agent({{feature}}): auto-sync {{date}}", fixedValues())

	if got != "agent(coupon-v2): auto-sync 2026-08-03" {
		t.Errorf("message = %q", got)
	}
}

func TestCommitMessageForRejectsATemplateThatRendersToNothing(t *testing.T) {
	// git refuses an empty commit message, so a template of only whitespace has
	// to fall back rather than fail the whole sync at the commit step.
	got := CommitMessageFor("   ", fixedValues())

	if strings.TrimSpace(got) == "" {
		t.Error("message is blank; git would reject the commit")
	}
}
