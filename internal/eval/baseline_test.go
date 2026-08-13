package eval

import (
	"regexp"
	"strings"
	"testing"
)

func baselineSample() []Entry {
	return []Entry{
		{Model: "mineru/mineru-pro-2605", Mode: "vision-only", Score: Score{
			HasText: true, Precision: 0.755, Recall: 0.988, F1: 0.856,
			RandzifferFound: 4, RandzifferExpected: 4,
			HeadingsFound: 8, HeadingsExpected: 8,
		}},
		{Model: "chat/olmocr-2-7b", Mode: "anchored", Score: Score{
			HasText: true, Precision: 0.758, Recall: 0.982, F1: 0.855,
			RandzifferFound: 1, RandzifferExpected: 4,
			HeadingsFound: 0, HeadingsExpected: 8,
			PageNumbers: 2, Letterheads: 3,
		}},
	}
}

// A baseline that cannot be read back is a baseline that silently stops
// reporting: the run it measured would come back as "new", and a new run is
// never a regression.
func TestBaselineRoundTrips(t *testing.T) {
	got, err := ParseBaseline(FormatBaseline(baselineSample()))
	if err != nil {
		t.Fatalf("ParseBaseline: %v", err)
	}
	if len(got) != len(baselineSample()) {
		t.Fatalf("read back %d entries, want %d", len(got), len(baselineSample()))
	}
	if regs := CompareBaseline(baselineSample(), got); len(regs) != 0 {
		t.Errorf("a baseline compared against itself reported %v", regs)
	}
}

// Sorted by key, so adding a model does not reorder the file and turn one
// measurement into a diff of every line.
func TestFormatBaselineIsSorted(t *testing.T) {
	out := FormatBaseline(baselineSample())
	chat := strings.Index(out, "chat/olmocr-2-7b")
	mineru := strings.Index(out, "mineru/mineru-pro-2605")
	if chat < 0 || mineru < 0 {
		t.Fatalf("both runs should appear:\n%s", out)
	}
	if chat > mineru {
		t.Errorf("entries are not sorted by key:\n%s", out)
	}
}

// Wall time is not recorded. It changes on every run for reasons unrelated to
// the change under test, and a diff that is never empty is a diff nobody reads.
func TestFormatBaselineOmitsTiming(t *testing.T) {
	duration := regexp.MustCompile(`\b\d+(\.\d+)?(ms|s)\b`)
	out := FormatBaseline(baselineSample())
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "#") {
			continue // the header explains why, which means naming it
		}
		if duration.MatchString(line) {
			t.Errorf("scored line carries a duration: %q", line)
		}
	}
	for _, unwanted := range []string{"elapsed", "took", "seconds"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("baseline mentions timing (%q):\n%s", unwanted, out)
		}
	}
}

func TestCompareBaselineFindsRateDrops(t *testing.T) {
	now := baselineSample()
	now[0].Score.Recall = 0.900

	regs := CompareBaseline(baselineSample(), now)
	if len(regs) != 1 {
		t.Fatalf("got %d regressions, want 1: %v", len(regs), regs)
	}
	if regs[0].Metric != "recall" || !strings.Contains(regs[0].Key, "mineru") {
		t.Errorf("regression = %v, want the mineru run's recall", regs[0])
	}
}

// A third decimal is the same transcription measured on different hardware,
// not a worse one.
func TestCompareBaselineToleratesDrift(t *testing.T) {
	now := baselineSample()
	now[0].Score.Recall -= 0.002
	now[0].Score.Precision -= 0.004

	if regs := CompareBaseline(baselineSample(), now); len(regs) != 0 {
		t.Errorf("drift inside the tolerance reported %v", regs)
	}
}

// More is not better. Thirteen headings on a document with eight is
// over-marking, and fifteen is worse — the word scores cannot see either,
// which is why these counts exist.
func TestCompareBaselineCountsDistanceNotMagnitude(t *testing.T) {
	was := []Entry{{Model: "m", Mode: "vision-only", Score: Score{HeadingsFound: 13, HeadingsExpected: 8}}}

	closer := []Entry{{Model: "m", Mode: "vision-only", Score: Score{HeadingsFound: 9, HeadingsExpected: 8}}}
	if regs := CompareBaseline(was, closer); len(regs) != 0 {
		t.Errorf("moving 13 -> 9 towards 8 reported %v", regs)
	}

	further := []Entry{{Model: "m", Mode: "vision-only", Score: Score{HeadingsFound: 15, HeadingsExpected: 8}}}
	if regs := CompareBaseline(was, further); len(regs) != 1 {
		t.Errorf("moving 13 -> 15 away from 8 reported %d regressions, want 1: %v", len(regs), regs)
	}
}

// A sequence break means a page was lost and leaked furniture is text the
// document does not have. Both should be zero, so any increase is a regression.
func TestCompareBaselineFlagsLeaksAndBreaks(t *testing.T) {
	now := baselineSample()
	now[0].Score.SequenceBreaks = 1
	now[1].Score.Letterheads = 5

	regs := CompareBaseline(baselineSample(), now)
	if len(regs) != 2 {
		t.Fatalf("got %d regressions, want 2: %v", len(regs), regs)
	}
}

// Scoring one model is what -models is for, and adding one is not a
// regression in the models that were already there.
func TestCompareBaselineIgnoresRunsWithNoCounterpart(t *testing.T) {
	subset := []Entry{baselineSample()[0]}
	if regs := CompareBaseline(baselineSample(), subset); len(regs) != 0 {
		t.Errorf("scoring a subset reported %v", regs)
	}

	added := append(baselineSample(), Entry{Model: "chat/new-model", Mode: "anchored", Score: Score{Recall: 0.1}})
	if regs := CompareBaseline(baselineSample(), added); len(regs) != 0 {
		t.Errorf("adding a model reported %v", regs)
	}
}

func TestParseBaselineRejectsAMalformedLine(t *testing.T) {
	for _, bad := range []string{
		"model vision-only precision",      // not name=value
		"model vision-only precision=high", // not a number
		"model vision-only headings=8",     // missing the expected half
		"model vision-only wordcount=12",   // unknown field
		"model",                            // no scores at all
	} {
		if _, err := ParseBaseline(bad); err == nil {
			t.Errorf("ParseBaseline(%q) succeeded, want an error", bad)
		}
	}
}
