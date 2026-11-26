package console

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// Speech recognition regression: what myco does after a recognizer (the
// rules in speech_words.go, the model's corrections filtered by
// speech_polish.go) on real recognizer output, scored the way the team's
// evaluation scores it:
//
//   - Chinese: character error rate (lower case, no spaces or punctuation;
//     Latin letters count as characters); English: word error rate;
//     micro-averaged per language;
//   - key words: exact spelling and spacing, not touching an ASCII letter,
//     digit or underscore ("PR" is not in "PRICE", "P R" is not "PR");
//   - negative controls (testdata/speech-negatives.json): any change at all
//     is a false edit.
//
// The recordings' texts are private: SPEECH_REGRESSION_DIR points at a
// folder with postprocessing-candidates.tsv (method, id, …, language,
// reference, recognized), keywords.json and vocabulary.json. Without it
// only the negative controls run.

type speechNegative struct {
	ID, Language, Input, Expected, Reason string
}

func loadSpeechNegatives(t *testing.T) []speechNegative {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "speech-negatives.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out []speechNegative
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// The six controls are never changed: not by the rules, and not by a model
// that replaces them with the nearest name (as models have).
func TestSpeechNegativeControls(t *testing.T) {
	words := append([]string{"Kettu", "Mac", "main", "App"}, commonSpeechWords...)
	// What a model has written, or might, for each.
	tempted := map[string][]string{
		"这里加一个句号":                           {"这里加一个GitHub"},
		"The cloud server is running":       {"The Claude server is running", "The Claude Cursor is running"},
		"Open Michael and check the report": {"Open myco and check the report", "Open Mac and check the report"},
		"先准备一下妹妹的作业":                        {"先准备一下mainmain的作业", "先准备一下main的作业"},
		"The cat sat on the mat":            {"The Kettu sat on the mat"},
		"Give me half of the result":        {"Give me Hub of the result", "Give me half of the React"},
	}
	for _, n := range loadSpeechNegatives(t) {
		f := newSpeechFixer(n.Language, words, nil)
		if got := f.Fix(n.Input); got != n.Expected {
			t.Errorf("%s: the rules changed %q to %q (%s)", n.ID, n.Input, got, n.Reason)
		}
		if len(tempted[n.Input]) == 0 {
			t.Errorf("%s: no tempting answer to try", n.ID)
		}
		for _, model := range tempted[n.Input] {
			if got := onlyNameFixes(n.Input, model, words); got != n.Expected {
				t.Errorf("%s: a model's %q got through as %q (%s)", n.ID, model, got, n.Reason)
			}
		}
	}
}

type regressionRow struct {
	id, lang, reference, recognized string
}

func TestSpeechRegression(t *testing.T) {
	dir := os.Getenv("SPEECH_REGRESSION_DIR")
	if dir == "" {
		t.Skip("set SPEECH_REGRESSION_DIR to the evaluation's text files")
	}
	var words []string
	readJSON(t, filepath.Join(dir, "vocabulary.json"), &words)
	var keywords map[string][]string
	readJSON(t, filepath.Join(dir, "keywords.json"), &keywords)
	b, err := os.ReadFile(filepath.Join(dir, "postprocessing-candidates.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]map[string]regressionRow{} // method → id → row
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n")[1:] {
		f := strings.Split(l, "\t")
		if len(f) < 6 {
			continue
		}
		if by[f[0]] == nil {
			by[f[0]] = map[string]regressionRow{}
		}
		by[f[0]][f[1]] = regressionRow{id: f[1], lang: f[3], reference: f[4], recognized: f[5]}
	}
	raw, llm := by["raw"], by["llm"]
	if len(raw) == 0 {
		t.Fatal("no raw rows")
	}
	ids := make([]string, 0, len(raw))
	for id := range raw {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	fixers := map[string]*speechFixer{}
	rules := func(lang, s string) string {
		if fixers[lang] == nil {
			fixers[lang] = newSpeechFixer(lang, words, nil)
		}
		return fixers[lang].Fix(s)
	}
	methods := []struct {
		name string
		out  func(r regressionRow) string
	}{
		{"as heard", func(r regressionRow) string { return r.recognized }},
		{"rules (speechFixer)", func(r regressionRow) string { return rules(r.lang, r.recognized) }},
		{"model, unfiltered", func(r regressionRow) string { return llm[r.id].recognized }},
		{"rules + model names only", func(r regressionRow) string {
			fixed := rules(r.lang, r.recognized)
			return rules(r.lang, onlyNameFixes(fixed, rules(r.lang, llm[r.id].recognized), words))
		}},
	}
	t.Logf("%-26s %10s %10s %14s", "", "zh CER", "en WER", "key words")
	for _, m := range methods {
		var zhE, zhN, enE, enN, hit, total int
		var missed []string
		for _, id := range ids {
			r := raw[id]
			out := m.out(r)
			if strings.HasPrefix(r.lang, "en") {
				ref, got := enWords(r.reference), enWords(out)
				enE += wordDistance(ref, got)
				enN += len(ref)
			} else {
				ref, got := []rune(normSpeech(r.reference)), []rune(normSpeech(out))
				zhE += editDistance(ref, got)
				zhN += len(ref)
			}
			for _, k := range keywords[id] {
				total++
				if keywordIn(out, k) {
					hit++
				} else {
					missed = append(missed, id+":"+k)
				}
			}
		}
		t.Logf("%-26s %9.1f%% %9.1f%% %8d/%d", m.name, pct(zhE, zhN), pct(enE, enN), hit, total)
		if m.name == "rules + model names only" {
			t.Logf("  still missed: %s", strings.Join(missed, " "))
		}
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

// keywordIn: k appears exactly, not touching an ASCII letter, digit or "_".
func keywordIn(s, k string) bool {
	edge := func(r rune) bool {
		return r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')
	}
	for from := 0; ; {
		i := strings.Index(s[from:], k)
		if i < 0 {
			return false
		}
		i += from
		before, after := []rune(s[:i]), []rune(s[i+len(k):])
		if (len(before) == 0 || !edge(before[len(before)-1])) && (len(after) == 0 || !edge(after[0])) {
			return true
		}
		from = i + 1
	}
}

// enWords: lower-case runs of letters and digits.
func enWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func wordDistance(a, b []string) int {
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

func TestKeywordBoundaries(t *testing.T) {
	for _, tc := range []struct {
		s, k string
		want bool
	}{
		{"看一下这个 PR", "PR", true}, {"这个PR好了", "PR", true}, {"PRICE", "PR", false}, {"P R", "PR", false},
		{"Claude Code 装好了", "Claude Code", true}, {"ClaudeCode", "Claude Code", false}, {"my_PR", "PR", false},
	} {
		if got := keywordIn(tc.s, tc.k); got != tc.want {
			t.Errorf("%q in %q: %v", tc.k, tc.s, got)
		}
	}
}
