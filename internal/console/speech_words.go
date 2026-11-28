package console

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Speech recognition hears names it does not know poorly, above all English
// words in Chinese ("Claude", "Codex", "GitHub"). Two things help, for voice
// calls and for dictation alike:
//
//   - the recognizer is told the words to expect (Apple's contextualStrings):
//     the workspace's agents, projects and members, a list of words people
//     here use all the time, and the words the person added in Settings ›
//     Voice;
//   - what it heard is corrected, for those words only: a known word's
//     casing ("github" → "GitHub"), its spelling with spaces ("Deep Seek"),
//     and how a Chinese recognizer tends to write it ("克劳德", "cloud" →
//     "Claude"). Nothing else in what was said is touched.

// commonSpeechWords are words people working with myco say all the time.
var commonSpeechWords = []string{
	"Claude", "Claude Code", "Codex", "myco", "Hub", "runner", "agent",
	"GitHub", "PR", "CI", "commit", "push", "pull", "merge", "rebase", "branch", "release", "tag",
	"DeepSeek", "OpenAI", "GPT", "Anthropic", "Opus", "Sonnet", "Haiku", "Gemini", "Kimi", "Qwen", "Doubao",
	"API", "SDK", "token", "bug", "README", "Markdown", "JSON", "React", "TypeScript", "Python",
	"Docker", "Xcode", "iOS", "Android", "macOS", "Windows", "Linux", "WSL", "VS Code", "Cursor", "Wails",
}

// speechAliases: how recognizers write some common words (Chinese ones
// mostly hear them by sound). Only forms nobody says for anything else:
// no real words ("cloud", "codecs", "cat", "句号"), which would change what
// people mean (the negative set in speech_words_test.go keeps it so).
// Letters said apart ("P R", "A P I") and casing are rules, not aliases.
var speechAliases = map[string][]string{
	"Claude":   {"克劳德", "克劳迪", "克洛德", "可劳德", "克老德", "Claud", "Clode"},
	"Codex":    {"扣德克斯", "口德克斯", "寇德克斯", "科德克斯", "抠得克斯", "Kodex"},
	"myco":     {"麦扣", "迈扣"},
	"DeepSeek": {"迪普西克", "迪普seek"},
	"GitHub":   {"吉特哈布"},
}

// unsafeAlias says why a person's alias is not used: one character, or a
// word people say for other things (it would change what they mean).
func unsafeAlias(a string) string {
	a = strings.TrimSpace(a)
	switch {
	case utf8.RuneCountInString(a) < 2:
		return "too short"
	case isLatin(a) && len(a) < 3:
		return "too short"
	case commonWords[strings.ToLower(a)]:
		return "a common word"
	}
	return ""
}

// commonWords: everyday words an alias must not be (a recognizer writes
// these when people mean them).
var commonWords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`
		句号 逗号 问号 妹妹 姐姐 哥哥 弟弟 妈妈 爸爸 宝宝 朋友 同事 老板 老师 我们 你们 他们 她们 它们 这个 那个 一个 什么 怎么 为什么 可以 没有 知道 觉得 现在 今天 明天 昨天
		时候 东西 事情 问题 工作 公司 项目 代码 电脑 手机 包裹 包子 口袋 扣子 克拉 卡通 猫咪 小猫 小狗 冰箱 蛋糕 咖啡 米饭 面条 苹果 香蕉 西瓜 云端 云朵 天气 下雨 开会 吃饭 睡觉 回家
		hub cursor windows python react docker opus sonnet haiku gemini markdown token bug tag branch release
		half cat dog cloud clouds code codes codec codecs cool cold call cloth close clock club deep seek sick sip dip git get hub hug pub up my co go do me be we see sea you ok okay yes no
		the and for are but not all any can had her was one our out day has him his how man new now old see two way who boy did its let put say she too use make like time just know take people year good some them other than then look only come over think also back after work first well even want because these give most
	`) {
		m[w] = true
	}
	return m
}()

// maxSpeechWords: Apple suggests keeping contextual strings to about a
// hundred.
const maxSpeechWords = 100

// parseSpeechWords reads the person's words, one per line: a word, or a
// word with how it is misheard ("Claude = 克劳德, cloud"). Returns the words
// and the aliases.
func parseSpeechWords(lines []string) ([]string, map[string][]string) {
	var words []string
	aliases := map[string][]string{}
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		word, rest, has := strings.Cut(strings.ReplaceAll(l, "＝", "="), "=")
		word = strings.TrimSpace(word)
		if word == "" {
			continue
		}
		words = append(words, word)
		if has {
			for _, a := range strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == '，' || r == '、' || r == ';' || r == '；' }) {
				if a = strings.TrimSpace(a); a != "" && a != word && unsafeAlias(a) == "" {
					aliases[word] = append(aliases[word], a)
				}
			}
		}
	}
	return words, aliases
}

// vocabCache keeps the workspace's names for a few minutes (a call or a
// dictation should not wait on the hubs each time).
type vocabCache struct {
	mu    sync.Mutex
	words []string
	at    time.Time
}

const vocabFresh = 5 * time.Minute

// workspaceWords: the names of agents, projects and members in the
// workspaces this App is signed in to (cached; at most a moment's wait).
func (b *Backend) workspaceWords(ctx context.Context) []string {
	b.vocab.mu.Lock()
	if time.Since(b.vocab.at) < vocabFresh {
		w := b.vocab.words
		b.vocab.mu.Unlock()
		return w
	}
	b.vocab.mu.Unlock()
	if b.Store == nil {
		return nil
	}
	hubs, err := b.Store.List()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var mu sync.Mutex
	var words []string
	add := func(ws ...string) {
		mu.Lock()
		defer mu.Unlock()
		for _, w := range ws {
			if w = strings.TrimSpace(w); w != "" && !strings.HasPrefix(w, "@") {
				words = append(words, w)
			}
		}
	}
	var wg sync.WaitGroup
	for _, h := range hubs {
		c, err := b.client(h)
		if err != nil {
			continue
		}
		wg.Add(3)
		go func() {
			defer wg.Done()
			if ov, err := c.Overview(ctx, 0); err == nil {
				for _, s := range ov.Agents {
					add(s.Agent.DisplayName, s.Agent.Name)
				}
			}
		}()
		go func() {
			defer wg.Done()
			if ps, err := c.ListProjects(ctx, false); err == nil {
				for _, p := range ps {
					add(p.Title, p.Name)
				}
			}
		}()
		go func() {
			defer wg.Done()
			if ms, err := c.Members(ctx, false); err == nil {
				for _, m := range ms {
					add(m.DisplayName, m.Name)
				}
			}
		}()
	}
	wg.Wait()
	words = speakable(words)
	b.vocab.mu.Lock()
	b.vocab.words, b.vocab.at = words, time.Now()
	b.vocab.mu.Unlock()
	return words
}

// speakable keeps names a person would say: ids and generated names
// ("invited-m9ixux9a", "cloud-hdkwmcki") are not words to listen for;
// a name with parts is listened for by its parts too ("pm-codex" → "pm codex").
func speakable(names []string) []string {
	var out []string
	for _, n := range names {
		if looksGenerated(n) {
			continue
		}
		out = append(out, n)
		if strings.ContainsAny(n, "-_") {
			out = append(out, strings.NewReplacer("-", " ", "_", " ").Replace(n))
		}
	}
	return out
}

// looksGenerated: a part with letters and digits mixed, or five consonants
// in a row ("hdkwmcki"), as random ids have.
func looksGenerated(n string) bool {
	for _, part := range strings.FieldsFunc(n, func(r rune) bool { return r == '-' || r == '_' || r == ' ' }) {
		letters, digits := 0, 0
		for _, r := range part {
			switch {
			case r >= '0' && r <= '9':
				digits++
			case r < unicode.MaxASCII && unicode.IsLetter(r):
				letters++
			}
		}
		if letters > 0 && digits > 0 && len(part) >= 6 {
			return true
		}
		if consonantRun(part) >= 5 {
			return true
		}
	}
	return false
}

// speechVocab is what the recognizer listens for and what is corrected:
// the person's words first, then the workspace's names, then the common
// words, each once, at most maxSpeechWords.
func (b *Backend) speechVocab(ctx context.Context) (words []string, aliases map[string][]string) {
	mine, aliases := parseSpeechWords(b.voiceState().stored().Words)
	seen := map[string]bool{}
	for _, list := range [][]string{mine, b.workspaceWords(ctx), commonSpeechWords} {
		for _, w := range list {
			k := strings.ToLower(w)
			if seen[k] || len(words) >= maxSpeechWords {
				continue
			}
			seen[k] = true
			words = append(words, w)
		}
	}
	return words, aliases
}

// speechFixer corrects what was heard for the vocabulary's words only.
type speechFixer struct {
	rules []fixRule
}

type fixRule struct {
	re *regexp.Regexp
	to string
}

// latinEdge: what may not touch an English word for it to be a word of its
// own ("pr" in "print" is not "PR"; "cloud" in "cloud-hdkwmcki" is a name).
const latinEdge = `A-Za-z0-9_\-.@/`

func newSpeechFixer(lang string, words []string, extra map[string][]string) *speechFixer {
	zh := lang == "" || strings.HasPrefix(strings.ToLower(lang), "zh")
	f := &speechFixer{}
	have := map[string]bool{}
	for _, w := range words {
		have[w] = true
	}
	latinRule := func(from, to string) {
		// The words around it must not be English too: "cloud" alone in
		// Chinese is "Claude", "the cloud server" stays.
		pat := `(?i)(^|[^` + latinEdge + `])` + spaced(regexp.QuoteMeta(from)) + `($|[^` + latinEdge + `])`
		f.rules = append(f.rules, fixRule{regexp.MustCompile(pat), "${1}" + to + "${2}"})
	}
	// Longer words first: "Claude Code" before "Claude".
	sorted := append([]string(nil), words...)
	sort.SliceStable(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })
	for _, w := range sorted {
		if !isLatin(w) || len(w) < 2 {
			continue
		}
		// Its casing, and its parts said apart ("Deep Seek", "Git Hub").
		// In English a capitalised word may be a plain one ("cursor",
		// "windows"): only odd casings ("GitHub", "macOS", "PR") are fixed.
		// Nor an everyday word ("hub", "python") in any language.
		if w != strings.ToLower(w) && (zh || !titleCase(w)) && !commonWords[strings.ToLower(w)] {
			latinRule(w, w)
		}
		if parts := camelParts(w); len(parts) > 1 {
			latinRule(strings.Join(parts, " "), w)
		}
		// In capitals, as some recognizers write all English ("PUSH",
		// "IOS", "CLAUDE CODE"): the word as it is written.
		if up := strings.ToUpper(w); up != w {
			pat := `(^|[^` + latinEdge + `])` + spaced(regexp.QuoteMeta(up)) + `($|[^` + latinEdge + `])`
			f.rules = append(f.rules, fixRule{regexp.MustCompile(pat), "${1}" + w + "${2}"})
		}
		// Letters said one by one: "P R", "A P I", "c.i." → the word.
		if spelled := spelledLetters(w); spelled != nil {
			f.rules = append(f.rules, fixRule{spelled, "${1}" + w + "${2}"})
		}
	}
	aliasesOf := func(w string) []string {
		var out []string
		if zh {
			out = append(out, speechAliases[w]...)
		}
		return append(out, extra[w]...)
	}
	for _, w := range sorted {
		if !have[w] {
			continue
		}
		for _, a := range aliasesOf(w) {
			if isLatin(a) {
				if !zh && strings.EqualFold(a, "cloud") {
					continue // in English, "cloud" is a cloud
				}
				if zh {
					f.rules = append(f.rules, fixRule{zhOnlyAround(a), "${1}" + w + "${2}"})
				} else {
					latinRule(a, w)
				}
			} else {
				f.rules = append(f.rules, fixRule{regexp.MustCompile(regexp.QuoteMeta(a)), w})
			}
		}
	}
	return f
}

// zhOnlyAround matches a Latin alias standing alone between Chinese, spaces,
// punctuation or the ends: not next to other English words.
func zhOnlyAround(a string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_\-.@/ ]\s*|^\s*)` + spaced(regexp.QuoteMeta(a)) + `(\s*[^A-Za-z0-9_\-.@/ ]|\s*$)`)
}

// spaced lets the spaces in a phrase be any run of spaces.
func spaced(quoted string) string { return strings.ReplaceAll(quoted, ` `, `\s+`) }

func (f *speechFixer) Fix(s string) string {
	if f == nil || s == "" {
		return s
	}
	for _, r := range f.rules {
		// Twice: matches that share an edge ("cloud cloud") both change.
		s = r.re.ReplaceAllString(s, r.to)
		s = r.re.ReplaceAllString(s, r.to)
	}
	return s
}

func isLatin(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return strings.IndexFunc(s, unicode.IsLetter) >= 0
}

// camelParts splits "DeepSeek" into "Deep", "Seek" (not "PR" or "macOS").
func camelParts(w string) []string {
	if strings.ContainsAny(w, " -_") {
		return nil
	}
	var parts []string
	start := 0
	rs := []rune(w)
	for i := 1; i < len(rs); i++ {
		if unicode.IsUpper(rs[i]) && unicode.IsLower(rs[i-1]) {
			parts = append(parts, string(rs[start:i]))
			start = i
		}
	}
	parts = append(parts, string(rs[start:]))
	for _, p := range parts {
		if len(p) < 2 {
			return nil
		}
	}
	return parts
}

// titleCase: "Claude", "Cursor" (as opposed to "GitHub", "PR", "macOS").
func titleCase(w string) bool {
	rs := []rune(w)
	if len(rs) == 0 || !unicode.IsUpper(rs[0]) {
		return false
	}
	for _, r := range rs[1:] {
		if unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// consonantRun: the most Latin consonants in a row.
func consonantRun(s string) int {
	most, run := 0, 0
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' && !strings.ContainsRune("aeiouy", r) {
			run++
			most = max(most, run)
		} else {
			run = 0
		}
	}
	return most
}

// spelledLetters matches an all-capitals word of 2 to 5 letters said one
// letter at a time ("P R", "a p i", "C.I."), standing alone.
func spelledLetters(w string) *regexp.Regexp {
	if n := len(w); n < 2 || n > 5 || w != strings.ToUpper(w) || !isLatin(w) {
		return nil
	}
	var parts []string
	for _, r := range w {
		if r < 'A' || r > 'Z' {
			return nil
		}
		parts = append(parts, string(r))
	}
	return regexp.MustCompile(`(?i)(^|[^` + latinEdge + `])` + strings.Join(parts, `[\s.]+`) + `\.?($|[^` + latinEdge + `])`)
}
