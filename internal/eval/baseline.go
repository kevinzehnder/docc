package eval

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// A baseline is the last scores a run produced, committed so that the next run
// can be read as a diff rather than against somebody's recollection.
//
// It exists because every decision behind ingest — which model, what DPI,
// whether a rule belongs in the prompt or in code — was settled by reading
// numbers off a terminal and remembering them. That is how this project once
// spent a day believing Randziffern were a detection problem: the score said
// "1 of 4" in every condition, which looked like a stable property of the
// models and was in fact a normalizer throwing away all four. A stored number
// nobody can compare against is a number that can be wrong for a long time.
//
// Wall time is deliberately not recorded. It is the field that changes on every
// run for reasons that have nothing to do with the change under test, and a
// diff that is never empty is a diff nobody reads.

// Entry is one scored run: which backend and model produced it, in which
// anchoring mode, and what it scored.
type Entry struct {
	// Model is the run's key, "backend/model" as the report prints it.
	Model string
	// Mode is "vision-only" or "anchored".
	Mode  string
	Score Score
}

// key identifies a run across baselines. Two runs with the same key are the
// same measurement taken twice.
func (e Entry) key() string { return e.Model + " " + e.Mode }

// FormatBaseline renders entries as the committed file: one line per run,
// sorted by key so that adding a model does not reorder the rest.
func FormatBaseline(entries []Entry) string {
	sorted := append([]Entry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].key() < sorted[j].key() })

	var b strings.Builder
	b.WriteString("# docc ingest evaluation baseline — `task test:eval -- -update`\n")
	b.WriteString("#\n")
	b.WriteString("# Recall is the number to trust: the ground truth is every word the document\n")
	b.WriteString("# is known to contain. Precision has a floor below 1 that is not the model's\n")
	b.WriteString("# fault — the theme prints boilerplate that is on the page and in no source.\n")
	b.WriteString("# Wall time is not recorded; it changes for reasons unrelated to any change\n")
	b.WriteString("# under test.\n")
	for _, e := range sorted {
		s := e.Score
		fmt.Fprintf(&b, "%s %s precision=%.3f recall=%.3f f1=%.3f rz=%d/%d breaks=%d headings=%d/%d pagenumbers=%d letterheads=%d\n",
			e.Model, e.Mode,
			s.Precision, s.Recall, s.F1,
			s.RandzifferFound, s.RandzifferExpected, s.SequenceBreaks,
			s.HeadingsFound, s.HeadingsExpected,
			s.PageNumbers, s.Letterheads)
	}
	return b.String()
}

// ParseBaseline reads a committed baseline back. An unreadable line is an
// error rather than a line to skip: a baseline that silently loses a row
// reports no regression for the run that row measured.
func ParseBaseline(text string) ([]Entry, error) {
	var out []Entry
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, fmt.Errorf("baseline line %d: expected a model, a mode and at least one score in %q", i+1, line)
		}
		e := Entry{Model: fields[0], Mode: fields[1]}
		for _, f := range fields[2:] {
			name, value, ok := strings.Cut(f, "=")
			if !ok {
				return nil, fmt.Errorf("baseline line %d: %q is not name=value", i+1, f)
			}
			if err := assign(&e.Score, name, value); err != nil {
				return nil, fmt.Errorf("baseline line %d: %w", i+1, err)
			}
		}
		e.Score.HasText = e.Score.Precision > 0 || e.Score.Recall > 0
		out = append(out, e)
	}
	return out, nil
}

// assign writes one name=value field onto a Score.
func assign(s *Score, name, value string) error {
	// The paired fields carry both halves in one token, because a count is
	// meaningless without what it was counted against: "headings=8" invites the
	// reader to work out whether that is all of them.
	pair := func() (int, int, error) {
		found, expected, ok := strings.Cut(value, "/")
		if !ok {
			return 0, 0, fmt.Errorf("field %s=%s: expected found/expected", name, value)
		}
		f, err := strconv.Atoi(found)
		if err != nil {
			return 0, 0, fmt.Errorf("field %s=%s: %w", name, value, err)
		}
		e, err := strconv.Atoi(expected)
		if err != nil {
			return 0, 0, fmt.Errorf("field %s=%s: %w", name, value, err)
		}
		return f, e, nil
	}
	num := func() (int, error) {
		n, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("field %s=%s: %w", name, value, err)
		}
		return n, nil
	}
	rate := func() (float64, error) {
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("field %s=%s: %w", name, value, err)
		}
		return v, nil
	}

	var err error
	switch name {
	case "precision":
		s.Precision, err = rate()
	case "recall":
		s.Recall, err = rate()
	case "f1":
		s.F1, err = rate()
	case "rz":
		s.RandzifferFound, s.RandzifferExpected, err = pair()
	case "headings":
		s.HeadingsFound, s.HeadingsExpected, err = pair()
	case "breaks":
		s.SequenceBreaks, err = num()
	case "pagenumbers":
		s.PageNumbers, err = num()
	case "letterheads":
		s.Letterheads, err = num()
	default:
		return fmt.Errorf("unknown field %q", name)
	}
	return err
}

// rateTolerance is how far a word score may drift before it counts as a
// regression.
//
// Not zero, even though a fixed seed makes a run reproducible on one machine:
// a different quantization or a different llama.cpp build moves the third
// decimal without anything being wrong. It is small enough that the 0.116 of
// precision anchoring is worth, or the 0.05 between two checkpoints, is never
// inside it.
const rateTolerance = 0.005

// Regression is one measurement that got worse.
type Regression struct {
	Key    string
	Metric string
	Was    string
	Now    string
}

func (r Regression) String() string {
	return fmt.Sprintf("%s: %s was %s, now %s", r.Key, r.Metric, r.Was, r.Now)
}

// CompareBaseline reports what got worse between a stored baseline and a run.
//
// The integer scores are compared exactly and the word scores within
// rateTolerance, because they are different kinds of claim. Recall 0.982
// against 0.983 is the same transcription measured twice; eight headings
// against seven is a section the next stage will fail to validate. A new run
// with no stored counterpart is not a regression — it is a model somebody just
// added — and a stored run missing from the new one is not either, because
// scoring a subset is what `-models` is for.
func CompareBaseline(was, now []Entry) []Regression {
	old := make(map[string]Score, len(was))
	for _, e := range was {
		old[e.key()] = e.Score
	}

	var out []Regression
	for _, e := range now {
		prev, ok := old[e.key()]
		if !ok {
			continue
		}
		out = append(out, compareScores(e.key(), prev, e.Score)...)
	}
	return out
}

func compareScores(key string, was, now Score) []Regression {
	var out []Regression
	add := func(metric, oldV, newV string) {
		out = append(out, Regression{Key: key, Metric: metric, Was: oldV, Now: newV})
	}

	for _, r := range []struct {
		name     string
		was, now float64
	}{
		{"precision", was.Precision, now.Precision},
		{"recall", was.Recall, now.Recall},
		{"f1", was.F1, now.F1},
	} {
		if r.now < r.was-rateTolerance {
			add(r.name, fmt.Sprintf("%.3f", r.was), fmt.Sprintf("%.3f", r.now))
		}
	}

	// Found counts are compared by distance from what the document has, not by
	// magnitude. Thirteen headings on a page with eight is over-marking, and
	// "more" is not "better" — moving from thirteen to fifteen is a step away
	// from the document.
	if missBy(now.HeadingsFound, now.HeadingsExpected) > missBy(was.HeadingsFound, was.HeadingsExpected) {
		add("headings", fmt.Sprintf("%d/%d", was.HeadingsFound, was.HeadingsExpected),
			fmt.Sprintf("%d/%d", now.HeadingsFound, now.HeadingsExpected))
	}
	if missBy(now.RandzifferFound, now.RandzifferExpected) > missBy(was.RandzifferFound, was.RandzifferExpected) {
		add("rz", fmt.Sprintf("%d/%d", was.RandzifferFound, was.RandzifferExpected),
			fmt.Sprintf("%d/%d", now.RandzifferFound, now.RandzifferExpected))
	}

	// A sequence break means a page was lost, and leaked furniture means the
	// draft carries text the document does not. Both should be zero, so any
	// increase is a regression.
	for _, c := range []struct {
		name     string
		was, now int
	}{
		{"breaks", was.SequenceBreaks, now.SequenceBreaks},
		{"pagenumbers", was.PageNumbers, now.PageNumbers},
		{"letterheads", was.Letterheads, now.Letterheads},
	} {
		if c.now > c.was {
			add(c.name, strconv.Itoa(c.was), strconv.Itoa(c.now))
		}
	}
	return out
}

// missBy is how far a count is from what the document actually has, in either
// direction.
func missBy(found, expected int) int {
	if found > expected {
		return found - expected
	}
	return expected - found
}
