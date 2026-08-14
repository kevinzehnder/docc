//go:build eval

// Package eval's round trip. Guarded by a build tag because it needs a loaded
// VLM and LibreOffice, and takes minutes:
//
//	task eval
//	task eval -- -models olmocr-2-7b,ggml-org/gemma-4-E4B-it-GGUF:Q4_0
//
// It builds a document docc already validates, renders it to PDF, transcribes
// the PDF back, and scores the result against the source it started from. The
// ground truth is exact because we wrote it — no hand-labelled corpus, and no
// client document in the repository.
package eval

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kevinzehnder/docc/internal/emit"
	"github.com/kevinzehnder/docc/internal/ingest"
	"github.com/kevinzehnder/docc/internal/ir"
	"github.com/kevinzehnder/docc/internal/parse"
	"github.com/kevinzehnder/docc/internal/schema"
	"github.com/kevinzehnder/docc/internal/sema"
	"github.com/kevinzehnder/docc/internal/theme"
)

var (
	modelsFlag = flag.String("models", "", "comma-separated models to compare (default: the configured one)")
	backends   = flag.String("backends", "", "comma-separated backends to compare: chat, mineru (default: the configured one)")
	dpiFlag    = flag.Int("dpi", 0, "rasterization DPI (default: from .docc/ingest.yaml)")
	endpoint   = flag.String("endpoint", "", "VLM endpoint (default: from .docc/ingest.yaml)")
	docFlag    = flag.String("doc", "", "an external PDF to score instead of the round trip; needs a text layer")
	corpusFlag = flag.String("corpus", "", "a directory of real PDFs (e.g. scans) to transcribe and report on, one by one")
	keepFlag   = flag.Bool("keep", false, "keep the rendered PDF for inspection")
	updateFlag = flag.Bool("update", false, "rewrite testdata/baseline.txt with this run's scores")
)

// baselinePath is the committed record of the last scores, which is what makes
// "did this change help?" a diff rather than a memory.
const baselinePath = "testdata/baseline.txt"

// TestRoundTrip renders a known document, transcribes it back, and reports how
// much of it survived — per model, so two can be compared on the same page
// rather than by recollection.
func TestRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("soffice"); err != nil {
		t.Skip("soffice not on PATH")
	}
	root := filepath.Join("..", "..", "testdata")

	// legal_complex is the yardstick rather than legal_valid: a longer brief
	// whose Randziffern run across several pages is what proves the score
	// notices a lost page, and the small fixture never spanned one. Both are
	// authored by us, so the ground truth is exact either way.
	srcPath := filepath.Join(root, "good", "legal_complex.md")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	pdfPath, values := buildPDF(t, root, srcPath, src)

	// Collected across the subtests below and settled once, at the end: a
	// baseline written per run would record whichever model finished last.
	var scored []Entry
	t.Cleanup(func() { settleBaseline(t, scored) })

	cfg := loadConfig(t)
	// What the page says: the body, plus the frontmatter the theme renders
	// into the letterhead and signature block. Both are text a transcription
	// should reproduce; the running header and page numbers are not, and are
	// scored separately.
	want := PlainText(string(src)) + "\n" + FlattenValues(values)

	for _, backend := range backendList(cfg) {
		t.Run(backend, func(t *testing.T) {
			for _, model := range models(cfg) {
				t.Run(model, func(t *testing.T) {
					for _, anchor := range anchorModes(backend) {
						name := "vision-only"
						if anchor {
							// The text layer is fed into the prompt, so this score is
							// not independent evidence of transcription quality — it
							// measures what anchoring adds on top of it.
							name = "anchored"
						}
						t.Run(name, func(t *testing.T) {
							runCfg := cfg
							runCfg.Backend = backend
							runCfg.Model = model
							runCfg.Anchor = anchor

							start := time.Now()
							md, pages, err := ingest.Convert(context.Background(), pdfPath, runCfg, ingest.ConvertOptions{
								DocType: "legal",
								Outline: outlineFor(t, root, "legal"),
								// The fixture is ours, so its scheme is known
								// rather than guessed — which is exactly the
								// condition --outline-strict states.
								OutlineStrict: true,
							})
							if err != nil {
								t.Fatalf("convert: %v", err)
							}
							score := Grade(Transcription{
								Markdown:          md,
								SourceText:        want,
								SourceRandziffern: make([]int, ExpectedRandziffern(string(src), "BEGRÜNDUNG")),
								Letterhead:        "Bezirksgericht Baden",
								SourceHeadings:    CountHeadings(PlainText(string(src))),
							})
							scored = append(scored, Entry{Model: backend + "/" + model, Mode: name, Score: score})
							report(t, backend+"/"+model, name, score, len(pages), time.Since(start))
						})
					}
				})
			}
		})
	}
}

// backendList is the set of backends to score, from -backends or the project
// configuration.
func backendList(cfg ingest.Config) []string {
	if *backends != "" {
		return strings.Split(*backends, ",")
	}
	if cfg.Backend != "" {
		return []string{cfg.Backend}
	}
	return []string{ingest.BackendChat}
}

// anchorModes is the anchoring axis for a backend. The mineru protocol has
// nowhere to put a text layer — its prompt is the task name — so running it
// twice would report the same score under two labels and double the time.
func anchorModes(backend string) []bool {
	if backend == ingest.BackendMinerU {
		return []bool{false}
	}
	return []bool{false, true}
}

// TestExternalDocument scores a PDF that is not ours against its own text
// layer. Weaker evidence than the round trip — the text layer's reading order
// is approximate, and a scan has none at all — but it is the only way to see
// how a model handles a document nobody wrote for the test.
func TestExternalDocument(t *testing.T) {
	if *docFlag == "" {
		t.Skip("pass -doc <file.pdf> to score an external document")
	}
	cfg := loadConfig(t)
	want := textLayer(t, *docFlag)
	if strings.TrimSpace(want) == "" {
		t.Logf("%s has no text layer — only the structural scores mean anything", *docFlag)
	}

	for _, model := range models(cfg) {
		t.Run(model, func(t *testing.T) {
			runCfg := cfg
			runCfg.Model = model
			runCfg.Anchor = false

			start := time.Now()
			md, pages, err := ingest.Convert(context.Background(), *docFlag, runCfg, ingest.ConvertOptions{})
			if err != nil {
				t.Fatalf("convert: %v", err)
			}
			report(t, model, "vision-only", Grade(Transcription{
				Markdown:   md,
				SourceText: want,
			}), len(pages), time.Since(start))
		})
	}
}

// TestCorpus transcribes every PDF in a directory and reports on each, so a
// folder of real documents — the scans a firm actually files, which cannot live
// in the repository — can be run through the pipeline in one command:
//
//	task test:eval -- -run Corpus -corpus assets
//
// It scores, it does not gate. There is no committed baseline because there is
// no committed corpus: the documents are a third party's, they vary, and a
// number measured against a folder nobody else has is not a regression anyone
// can reproduce. The value is the readout, read by a person deciding whether the
// pipeline is good enough for the documents it will actually see.
//
// Two regimes, decided per document by whether it carries a text layer:
//
//   - Born-digital or already-OCR'd PDF: the text layer is an approximate
//     ground truth, so the word scores mean something, with the reading-order
//     caveat TestExternalDocument notes.
//   - A scan: no text layer, so the word scores are skipped and only the
//     structural signals remain. The one that needs no ground truth is the
//     Randziffer sequence — a gap, repeat or reversal is a page the pipeline
//     lost or doubled, visible without knowing what the page was supposed to
//     say. Leaked page numbers and letterheads are the other two.
func TestCorpus(t *testing.T) {
	if *corpusFlag == "" {
		t.Skip("pass -corpus <dir> to transcribe a directory of PDFs")
	}
	// `go test` runs in the package directory, so a relative -corpus is resolved
	// against the repository root the way a person naming "assets" means it,
	// rather than against internal/eval where nothing they have lives.
	dir := *corpusFlag
	if !filepath.IsAbs(dir) {
		dir = filepath.Join("..", "..", dir)
	}
	pdfs, err := filepath.Glob(filepath.Join(dir, "*.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pdfs) == 0 {
		t.Skipf("no PDFs in %s", dir)
	}
	sort.Strings(pdfs)

	cfg := loadConfig(t)
	// The convention a third party's brief is most likely to follow, so the
	// headings that can be recognized are marked. Not strict: an unfamiliar
	// document is exactly the one whose structure we would rather keep than
	// discard, so a line matching no rule is left as the model produced it.
	outline := outlineFor(t, filepath.Join("..", "..", "testdata"), "legal_reference")

	for _, model := range models(cfg) {
		t.Run(model, func(t *testing.T) {
			for _, pdf := range pdfs {
				t.Run(filepath.Base(pdf), func(t *testing.T) {
					runCfg := cfg
					runCfg.Model = model
					runCfg.Anchor = false

					want := textLayer(t, pdf)
					if strings.TrimSpace(want) == "" {
						t.Logf("%s has no text layer — a scan; only the structural scores mean anything", filepath.Base(pdf))
					}

					start := time.Now()
					md, pages, err := ingest.Convert(context.Background(), pdf, runCfg, ingest.ConvertOptions{
						Outline: outline,
					})
					if err != nil {
						t.Fatalf("convert: %v", err)
					}
					report(t, filepath.Base(pdf), "vision-only", Grade(Transcription{
						Markdown:   md,
						SourceText: want,
					}), len(pages), time.Since(start))
				})
			}
		})
	}
}

// TestReferenceRandziffernSurvive transcribes a numbered brief under
// legal_reference semantics and checks that its paragraph numbers come back
// exactly, 1..N with no gap, repeat or reversal.
//
// This is the opposite requirement to TestRoundTrip. There the Randziffer is
// ours to regenerate: a transcription that renumbers is no error, because
// building the document assigns the numbers afresh. Here the page belongs to
// somebody else and the number is the citation key — carried as body text
// rather than generated, since legal_reference declares no
// render.paragraph_numbering — so a dropped or doubled number is a citation
// pointing at the wrong paragraph. The assertion is exact equality, not a
// score.
//
// The page under test is the same one TestRoundTrip renders: legal_complex
// carries a continuous margin Randziffer 1..20, which is exactly what a filed
// brief a firm would cite looks like. A reference is never itself rendered —
// it is transcribed from a client PDF that cannot live in the repository — so
// the fixture stands in for that PDF, and the only thing that changes from the
// round trip is that ingest is told to treat the numbers as the source's rather
// than its own.
func TestReferenceRandziffernSurvive(t *testing.T) {
	if _, err := exec.LookPath("soffice"); err != nil {
		t.Skip("soffice not on PATH")
	}
	root := filepath.Join("..", "..", "testdata")

	srcPath := filepath.Join(root, "good", "legal_complex.md")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}

	// The rendered page numbers every prose paragraph after RECHTSBEGEHREN
	// continuously from 1, so the sequence that has to survive is 1..N.
	n := ExpectedRandziffern(string(src), "BEGRÜNDUNG")
	want := make([]int, n)
	for i := range want {
		want[i] = i + 1
	}

	pdfPath, _ := buildPDF(t, root, srcPath, src)
	cfg := loadConfig(t)
	outline := outlineFor(t, root, "legal_reference")

	for _, model := range models(cfg) {
		t.Run(model, func(t *testing.T) {
			runCfg := cfg
			runCfg.Model = model
			// Anchoring feeds the text layer into the prompt, which would let a
			// model copy the numbers rather than read them off the page. The
			// point is whether the page survives transcription, so it is off.
			runCfg.Anchor = false

			md, _, err := ingest.Convert(context.Background(), pdfPath, runCfg, ingest.ConvertOptions{
				DocType:       "legal_reference",
				Outline:       outline,
				OutlineStrict: true,
			})
			if err != nil {
				t.Fatalf("convert: %v", err)
			}

			got := markedNumbers(md)
			if !slices.Equal(got, want) {
				t.Errorf("Randziffern did not survive verbatim\n  want %v\n  got  %v\n"+
					"A reference document's numbers are citation keys and must come back exactly; "+
					"a gap or repeat above is a page the transcription lost or doubled.", want, got)
			}
		})
	}
}

// settleBaseline writes this run's scores, or reports what got worse since the
// last one.
//
// It fails on a regression rather than only logging it, because a score that is
// merely printed is a score nobody compares: this project once recorded
// "Randziffern 1 of 4" in every condition for a day, read it as a stable
// property of the models, and only later found it was a normalizer discarding
// all four. Under -update it writes instead, which is the same discipline the
// golden fixtures use — read the diff, then accept it.
func settleBaseline(t *testing.T, scored []Entry) {
	t.Helper()
	if len(scored) == 0 {
		return // every subtest skipped, or the run died before scoring anything
	}

	stored, err := os.ReadFile(baselinePath)
	switch {
	case os.IsNotExist(err):
		if !*updateFlag {
			t.Logf("no %s yet — run with -update to record this run as the baseline\n%s", baselinePath, FormatBaseline(scored))
			return
		}
	case err != nil:
		t.Fatal(err)
	}
	was, err := ParseBaseline(string(stored))
	if err != nil {
		t.Fatalf("%s: %v", baselinePath, err)
	}

	if *updateFlag {
		// Merged, not replaced: a run scoping itself with -backends or -models
		// measured a subset, and writing only what it measured would drop every
		// other row — after which the next comparison reports no regression
		// because there is nothing left to compare against.
		if err := os.MkdirAll(filepath.Dir(baselinePath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(baselinePath, []byte(FormatBaseline(MergeBaseline(was, scored))), 0o644); err != nil { //nolint:gosec // a committed fixture, not a secret
			t.Fatal(err)
		}
		t.Logf("wrote %s — read the diff before committing it", baselinePath)
		return
	}

	regressions := CompareBaseline(was, scored)
	if len(regressions) == 0 {
		t.Logf("no regression against %s", baselinePath)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d score(s) worse than %s:\n", len(regressions), baselinePath)
	for _, r := range regressions {
		fmt.Fprintf(&b, "  %s\n", r)
	}
	b.WriteString("\nIf the change is intended, re-run with -update and commit the diff.")
	t.Error(b.String())
}

func report(t *testing.T, model, mode string, s Score, pages int, took time.Duration) {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s / %s — %d page(s) in %s\n", model, mode, pages, took.Round(time.Second))
	if s.HasText {
		// Recall is the metric to trust: the ground truth is every word the
		// document is known to contain, so a drop is a real loss. Precision has
		// a floor below 1 that is not the model's fault — a theme prints its own
		// boilerplate (a letterhead strapline, an attachment-list caption) which
		// is on the page, correctly transcribed, and in no source we can compare
		// against. Read it as a number that should not fall, not as an error rate.
		fmt.Fprintf(&b, "  words     precision %.3f  recall %.3f  F1 %.3f\n", s.Precision, s.Recall, s.F1)
		if len(s.Missing) > 0 {
			fmt.Fprintf(&b, "  dropped   %s\n", strings.Join(s.Missing, " "))
		}
		if len(s.Spurious) > 0 {
			fmt.Fprintf(&b, "  invented  %s\n", strings.Join(s.Spurious, " "))
		}
	} else {
		b.WriteString("  words     (no source text to compare against)\n")
	}
	fmt.Fprintf(&b, "  Randziffern %d found / %d expected, %d sequence break(s)\n",
		s.RandzifferFound, s.RandzifferExpected, s.SequenceBreaks)
	fmt.Fprintf(&b, "  headings  %d found / %d expected\n", s.HeadingsFound, s.HeadingsExpected)
	fmt.Fprintf(&b, "  leaked    %d page number(s), %d letterhead(s)\n", s.PageNumbers, s.Letterheads)
	t.Log(b.String())
}

// buildPDF renders the fixture the same way docc build does.
func buildPDF(t *testing.T, root, srcPath string, src []byte) (string, map[string]any) {
	t.Helper()

	schemas, err := schema.Load(filepath.Join(root, "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	themeDir := filepath.Join(root, "themes")
	themes, err := theme.Load(themeDir)
	if err != nil {
		t.Fatal(err)
	}

	f, parseDiags := parse.Parse(filepath.Base(srcPath), src)
	res := sema.Check(f, schemas, parseDiags, "")
	if res.Diagnostics.HasErrors() {
		t.Fatalf("%s does not validate; the fixture must be clean before it is a yardstick", srcPath)
	}
	th, err := themes.Get(res.Schema.Theme)
	if err != nil {
		t.Fatal(err)
	}
	built, err := emit.Build(ir.Build(f, res.DocType, res.Meta.Values), res.Schema, th, emit.Options{ThemeDir: themeDir})
	if err != nil {
		t.Fatal(err)
	}
	data, err := built.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if *keepFlag {
		dir, err = os.MkdirTemp("", "docc-eval-")
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("keeping rendered output in %s", dir)
	}
	base := strings.TrimSuffix(filepath.Base(srcPath), ".md")
	docxPath := filepath.Join(dir, base+".docx")
	if err := os.WriteFile(docxPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(dir, base+".pdf")
	if err := emit.ToPDF(docxPath, pdfPath, emit.PDFOptions{Retries: 1}); err != nil {
		t.Fatalf("render to pdf: %v", err)
	}
	return pdfPath, res.Meta.Values
}

// outlineFor compiles a document type's default section-title scheme, the way
// cmd/docc does for --type.
//
// Without it the heading count measures the model alone, which is a fair number
// about the model and a misleading one about the pipeline: what an author gets
// is `docc ingest --type`, and the schema knows that this theme has Word draw an
// I./A./1. outline the source markdown does not contain.
func outlineFor(t *testing.T, root, docType string) []ingest.OutlineRule {
	t.Helper()
	set, err := schema.Load(filepath.Join(root, "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	sc, err := set.Get(docType)
	if err != nil {
		t.Fatal(err)
	}
	rules, ok := sc.Outline.Schemes[sc.Outline.Default]
	if !ok {
		t.Fatalf("%s declares no default outline scheme", docType)
	}
	patterns := make([]ingest.OutlinePattern, 0, len(rules))
	for _, r := range rules {
		patterns = append(patterns, ingest.OutlinePattern{Pattern: r.Pattern, Level: r.Level})
	}
	compiled, err := ingest.CompileOutline(patterns)
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func loadConfig(t *testing.T) ingest.Config {
	t.Helper()
	cfg, err := ingest.LoadConfig(filepath.Join("..", "..", ".docc", "ingest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if *dpiFlag != 0 {
		cfg.DPI = *dpiFlag
	}
	if *endpoint != "" {
		cfg.Endpoint = *endpoint
	}
	if cfg.Model == "" && *modelsFlag == "" {
		t.Skip("no model configured — set one in .docc/ingest.yaml or pass -models")
	}
	return cfg
}

func models(cfg ingest.Config) []string {
	if *modelsFlag == "" {
		return []string{cfg.Model}
	}
	return strings.Split(*modelsFlag, ",")
}

func textLayer(t *testing.T, pdfPath string) string {
	t.Helper()
	out, err := exec.Command("pdftotext", pdfPath, "-").Output()
	if err != nil {
		t.Logf("pdftotext: %v", err)
		return ""
	}
	return string(out)
}
