package console

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Correcting names in what speech recognition heard, with a fast text model,
// in the background: what was heard is shown and used at once; when the
// model answers in time (polishTimeout) with a name from the vocabulary
// where the recognizer wrote something else ("包扣" → "Codex", "CLOU" →
// "Claude"), that is put in its place, marked as corrected, and can be
// undone. Only those changes are taken: whatever else the model changed
// (punctuation, wording) is dropped. No model, a slow or failed one, or
// the setting off: what was heard stays, and nothing is said.

const polishTimeout = 2 * time.Second

// speechPolish corrects names with a model.
type speechPolish struct {
	model voiceModel
	words []string
	lang  string
}

// speechPolisher is the voice model as a corrector, or nil when there is
// none or the person turned it off.
func (b *Backend) speechPolisher(lang string, words []string) *speechPolish {
	c := b.effectiveVoiceConfig()
	if !c.polishes() || len(words) == 0 {
		return nil
	}
	llm, _ := b.voiceKeys(c)
	if c.LLMBaseURL == "" || c.LLMModel == "" || llm == "" {
		return nil
	}
	return &speechPolish{model: &openAIModel{base: c.LLMBaseURL, model: c.LLMModel, key: llm, cold: true, session: "myco-polish-" + requestID()}, words: words, lang: lang}
}

func polishPrompt(lang string, words []string) string {
	list := strings.Join(words, "、")
	if lang != "" && !strings.HasPrefix(strings.ToLower(lang), "zh") {
		return "You correct speech recognition. The user's message is what a recognizer heard; it may have written some names wrong (\"Claude\" as \"cloud\" or \"clod\", \"GitHub\" as \"get hub\"). Change only names it got wrong into how they are written in this list, and nothing else: no rewording, no punctuation. If nothing is wrong, give it back unchanged. Answer with the sentence only.\nNames: " + list
	}
	return "你是语音识别的纠错器。用户发来的是语音识别的结果，里面可能把一些专有名词听错了（比如把 Claude 识别成「包扣」「CLOU」，把 GitHub 识别成「句号」，把 DeepSeek 识别成「DIP SIP」）。只把听错的专有名词改成下面词表里的写法，其他一个字都不要动：不润色、不改标点、不加字。没有要改的就原样返回。只输出这句话本身。\n词表：" + list
}

// Fix asks the model, within polishTimeout; ok when a name was corrected.
func (p *speechPolish) Fix(ctx context.Context, heard string) (string, bool) {
	if p == nil || utf8.RuneCountInString(strings.TrimSpace(heard)) < 2 {
		return heard, false
	}
	ctx, cancel := context.WithTimeout(ctx, polishTimeout)
	defer cancel()
	reply, err := p.model.Stream(ctx, []chatMsg{{Role: "system", Content: polishPrompt(p.lang, p.words)}, {Role: "user", Content: heard}}, nil, nil, nil)
	if err != nil || ctx.Err() != nil {
		return heard, false
	}
	fixed := onlyNameFixes(heard, strings.TrimSpace(reply.Content), p.words)
	return fixed, fixed != heard
}

// onlyNameFixes keeps, of the model's changes to heard, those that put a
// vocabulary word where something else was written, and drops the rest.
func onlyNameFixes(heard, fixed string, words []string) string {
	if fixed == "" || fixed == heard {
		return heard
	}
	vocab := map[string]bool{}
	for _, w := range words {
		vocab[w] = true
	}
	a, b := speechTokens(heard), speechTokens(fixed)
	n, m := len(a), len(b)
	if n*m > 400*400 {
		return heard
	}
	// Longest common subsequence of tokens; spaces do not anchor it (they
	// go with the words around them).
	same := func(i, j int) bool { return a[i] == b[j] && strings.TrimSpace(a[i]) != "" }
	l := make([][]int, n+1)
	for i := range l {
		l[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if same(i, j) {
				l[i][j] = l[i+1][j+1] + 1
			} else {
				l[i][j] = max(l[i+1][j], l[i][j+1])
			}
		}
	}
	var out []string // tokens of the answer
	i, j := 0, 0
	for i < n || j < m {
		if i < n && j < m && same(i, j) && l[i][j] == l[i+1][j+1]+1 {
			out = append(out, a[i])
			i, j = i+1, j+1
			continue
		}
		// A run of differences, up to the next token both share.
		i0, j0 := i, j
		for i < n || j < m {
			if i < n && j < m && same(i, j) && l[i][j] == l[i+1][j+1]+1 {
				break
			}
			if j < m && (i >= n || l[i][j+1] >= l[i+1][j]) {
				j++
			} else {
				i++
			}
		}
		was := strings.Join(a[i0:i], "")
		if name := nameIn(b[j0:j], vocab); name != "" && nameFix(was, name) {
			out = append(out, leadSpace(was)+name+trailSpace(was))
			continue
		}
		// The change may be part of a name, with the same text around it
		// ("前端小玩" → "前端小王"): a few tokens either side, alike in both.
		if name, back, ahead := nameAround(a, b, i0, i, j0, j, out, vocab); name != "" {
			out = append(out[:len(out)-back], name)
			i, j = i+ahead, j+ahead
			continue
		}
		out = append(out, was)
	}
	return strings.Join(out, "")
}

// nameAround looks for a vocabulary word made of the model's change and up
// to six tokens before and after it that heard and the answer share. It
// gives the word and how many tokens before (already in out) and after it
// takes in.
func nameAround(a, b []string, i0, i, j0, j int, out []string, vocab map[string]bool) (string, int, int) {
	for span := 1; span <= 12; span++ {
		for back := 0; back <= span && back <= 6; back++ {
			ahead := span - back
			if ahead > 6 || back > len(out) || j0-back < 0 || i+ahead > len(a) || j+ahead > len(b) {
				continue
			}
			ok := true
			for k := 0; k < back && ok; k++ {
				ok = out[len(out)-back+k] == b[j0-back+k]
			}
			for k := 0; k < ahead && ok; k++ {
				ok = a[i+k] == b[j+k]
			}
			if !ok {
				continue
			}
			name := strings.TrimSpace(strings.Join(b[j0-back:j+ahead], ""))
			if !vocab[name] {
				continue
			}
			was := strings.Join(out[len(out)-back:], "") + strings.Join(a[i0:i], "") + strings.Join(a[i:i+ahead], "")
			if nameFix(was, name) {
				return leadSpace(was) + name + trailSpace(was), back, ahead
			}
		}
	}
	return "", 0, 0
}

// nameIn: the one vocabulary word among what the model wrote in a place
// (what it added around it is dropped), or "".
func nameIn(toks []string, vocab map[string]bool) string {
	found := ""
	for x := 0; x < len(toks); x++ {
		for y := len(toks); y > x; y-- {
			w := strings.TrimSpace(strings.Join(toks[x:y], ""))
			if w != "" && vocab[w] {
				if found != "" && found != w {
					return "" // two names: not one misheard word
				}
				found = w
				x = y - 1
				break
			}
		}
	}
	return found
}

// nameFix: name may replace was: something was written (not an addition),
// not much longer than the name, and not a word people say (a common
// Chinese word, an English word in ordinary case: "句号", "Michael", "cat";
// recognizers write misheard names in capitals, "CLOU", "DIP SIP").
func nameFix(was, name string) bool {
	w := strings.TrimSpace(was)
	if w == "" || w == name || !isLatin(w) && commonWords[w] {
		return false
	}
	cjk, latin := 0, 0
	for _, tok := range speechTokens(w) {
		switch {
		case isLatin(tok):
			latin += len(tok)
			// (capitals are the recognizer's: "CAT TO" may be "Kettu")
			if tok != strings.ToUpper(tok) && (commonWords[strings.ToLower(tok)] || len(tok) >= 3) {
				return false
			}
		case strings.TrimSpace(tok) != "":
			cjk++
		}
	}
	n := utf8.RuneCountInString(name)
	return cjk <= max(4, n+1) && latin <= 2*n+2
}

func leadSpace(s string) string  { return s[:len(s)-len(strings.TrimLeft(s, " "))] }
func trailSpace(s string) string { return s[len(strings.TrimRight(s, " ")):] }

// speechTokens: an English word (letters and digits) is one token, as is
// each other character (a Chinese character, punctuation, a space).
func speechTokens(s string) []string {
	var out []string
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			out = append(out, word.String())
			word.Reset()
		}
	}
	for _, r := range s {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			word.WriteRune(r)
			continue
		}
		flush()
		out = append(out, string(r))
	}
	flush()
	return out
}
