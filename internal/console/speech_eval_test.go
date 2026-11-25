package console

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// The speech recognition evaluation's Go side (scripts/speech-eval/run.sh
// runs it on a Mac): the words to listen for, and the score of what Apple's
// recognizer heard in each mode, as heard and after correcting known words.

const speechEvalDir = "../../scripts/speech-eval"

type evalSample struct {
	id, lang, said string
	terms          []string
}

func readEvalSamples(t *testing.T) []evalSample {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(speechEvalDir, "samples.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	var out []evalSample
	for _, l := range strings.Split(string(b), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Split(l, "\t")
		if len(f) < 4 {
			t.Fatalf("samples.tsv: %q", l)
		}
		out = append(out, evalSample{id: f[0], lang: f[1], said: f[2], terms: strings.Split(f[3], ",")})
	}
	return out
}

// evalWords: the workspace stand-ins (words.txt) and the common words, as
// the App would listen for them.
func evalWords(t *testing.T) ([]string, map[string][]string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(speechEvalDir, "words.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, l := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			lines = append(lines, l)
		}
	}
	mine, aliases := parseSpeechWords(lines)
	seen := map[string]bool{}
	var words []string
	for _, w := range append(mine, commonSpeechWords...) {
		if k := strings.ToLower(w); !seen[k] && len(words) < maxSpeechWords {
			seen[k] = true
			words = append(words, w)
		}
	}
	return words, aliases
}

func TestSpeechEvalHints(t *testing.T) {
	path := os.Getenv("SPEECH_EVAL_HINTS")
	if path == "" {
		t.Skip("scripts/speech-eval/run.sh sets SPEECH_EVAL_HINTS")
	}
	words, _ := evalWords(t)
	if err := os.WriteFile(path, []byte(strings.Join(words, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

type evalResult struct {
	ID    string `json:"id"`
	Mode  string `json:"mode"`
	Text  string `json:"text"`
	Error string `json:"error"`
}

// evalScore is one mode's score: how many key terms came out exactly, and
// the character error rate (spaces and punctuation aside).
type evalScore struct {
	terms, hit  int
	chars, errs int
	failed      int
	misses      []string
}

func (s evalScore) String() string {
	if s.terms == 0 {
		return "—"
	}
	return fmt.Sprintf("terms %d/%d (%.0f%%), CER %.1f%%", s.hit, s.terms, 100*float64(s.hit)/float64(s.terms), 100*float64(s.errs)/float64(max(1, s.chars)))
}

func (s *evalScore) add(sm evalSample, heard string) {
	s.terms += len(sm.terms)
	for _, term := range sm.terms {
		if strings.Contains(heard, term) {
			s.hit++
		} else {
			s.misses = append(s.misses, sm.id+":"+term)
		}
	}
	want, got := []rune(normSpeech(sm.said)), []rune(normSpeech(heard))
	s.chars += len(want)
	s.errs += editDistance(want, got)
}

// scoreSpeech scores results by mode: as heard, and as corrected.
func scoreSpeech(samples []evalSample, results []evalResult, words []string, aliases map[string][]string) (modes []string, raw, fixed map[string]*evalScore) {
	by := map[string]evalSample{}
	for _, s := range samples {
		by[s.id] = s
	}
	raw, fixed = map[string]*evalScore{}, map[string]*evalScore{}
	fixers := map[string]*speechFixer{}
	for _, r := range results {
		sm, ok := by[r.ID]
		if !ok {
			continue
		}
		key := r.Mode + " " + strings.SplitN(sm.lang, "-", 2)[0]
		if raw[key] == nil {
			raw[key], fixed[key] = &evalScore{}, &evalScore{}
			modes = append(modes, key)
		}
		if r.Error != "" && r.Text == "" {
			raw[key].failed++
			fixed[key].failed++
			continue
		}
		if fixers[sm.lang] == nil {
			fixers[sm.lang] = newSpeechFixer(sm.lang, words, aliases)
		}
		raw[key].add(sm, r.Text)
		fixed[key].add(sm, fixers[sm.lang].Fix(r.Text))
	}
	sort.Strings(modes)
	return modes, raw, fixed
}

func TestSpeechEvalScore(t *testing.T) {
	path := os.Getenv("SPEECH_EVAL_RESULTS")
	if path == "" {
		t.Skip("scripts/speech-eval/run.sh sets SPEECH_EVAL_RESULTS")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var results []evalResult
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r evalResult
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			results = append(results, r)
		}
	}
	samples := readEvalSamples(t)
	words, aliases := evalWords(t)
	modes, raw, fixed := scoreSpeech(samples, results, words, aliases)
	t.Logf("%-22s %-36s %s", "mode", "as heard", "corrected (myco)")
	for _, m := range modes {
		note := ""
		if raw[m].failed > 0 {
			note = fmt.Sprintf("  (%d not recognized)", raw[m].failed)
		}
		t.Logf("%-22s %-36s %s%s", m, raw[m], fixed[m], note)
	}
	for _, m := range modes {
		if len(fixed[m].misses) > 0 {
			t.Logf("still missed in %s: %s", m, strings.Join(fixed[m].misses, " "))
		}
	}
	for _, r := range results {
		if r.Text != "" {
			t.Logf("heard %s %s: %s", r.Mode, r.ID, r.Text)
		} else if r.Error != "" {
			t.Logf("error %s %s: %s", r.Mode, r.ID, r.Error)
		}
	}
}

// The scoring itself, on made-up results.
func TestSpeechEvalScoring(t *testing.T) {
	samples := readEvalSamples(t)
	if len(samples) < 10 {
		t.Fatalf("%d samples", len(samples))
	}
	words, aliases := evalWords(t)
	for _, w := range []string{"Kettu", "前端小王", "Claude", "GitHub"} {
		if !contains(words, w) {
			t.Fatalf("the words to listen for lack %s: %v", w, words)
		}
	}
	results := []evalResult{
		{ID: "zh01", Mode: "base-ondevice", Text: "让克劳德看一下这个 pr"},
		{ID: "zh02", Mode: "base-ondevice", Text: "把代码 push 到 GitHub 上"},
		{ID: "zh03", Mode: "base-ondevice", Error: "no speech"},
	}
	modes, raw, fixed := scoreSpeech(samples, results, words, aliases)
	if len(modes) != 1 || modes[0] != "base-ondevice zh" {
		t.Fatalf("modes: %v", modes)
	}
	r, f := raw[modes[0]], fixed[modes[0]]
	if r.terms != 4 || r.hit != 2 || f.hit != 4 || r.failed != 1 {
		t.Fatalf("raw %+v fixed %+v", r, f)
	}
	if f.errs >= r.errs {
		t.Fatalf("correcting should lower the errors: %d → %d", r.errs, f.errs)
	}
}

// normSpeech: lower case, without spaces and punctuation.
func normSpeech(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func editDistance(a, b []rune) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
