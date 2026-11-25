package console

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
)

// A voice call: what the person says goes to the model; what the model says
// is spoken, a sentence at a time, as it is written; what it does in myco
// is done as the person, in their conversation with one agent, and the
// model speaks as that agent. The person
// speaking cuts it off. The agent's task at work is watched, and its answer
// or question is told in a pause.

// The model, the voice and myco, as a call needs them (fakes in tests).
type voiceDeps struct {
	audio VoiceAudio
	model voiceModel
	tts   voiceSpeech
	hub   voiceHub
	lang  string
	// endOfSpeech: how long a partial hearing stays the same before the
	// person is taken to have finished (the recognizer's own end of a
	// sentence comes sooner when it is sure).
	endOfSpeech time.Duration
	// quietBeforeNotice: how long the call is quiet before it tells news.
	quietBeforeNotice time.Duration
	// watchEvery: how often handed-out tasks are looked at.
	watchEvery time.Duration
	// stillWorkingEvery: how long the call goes without a word about a task
	// that is not done before it says that it still is (a long task would
	// otherwise be minutes of silence).
	stillWorkingEvery time.Duration
	// memo: the last call's end, for the instructions (voice_memo.go);
	// ended gets what was said in this one when it ends.
	memo  string
	ended func(history []chatMsg)
	// greet: the agent speaks first once the call is on, as one does
	// picking up the phone (off in tests unless they ask for it).
	greet bool
}

type voiceCall struct {
	v *voice
	d voiceDeps

	// muted is guarded by v.mu (the routes set it).
	muted bool

	ctx  context.Context
	stop context.CancelFunc
	in   chan func() // everything runs on the call's loop, in order
	once sync.Once

	// Loop state (only the loop goroutine touches these).
	history   []chatMsg
	partial   string
	partialAt time.Time
	endTimer  *time.Timer
	turn      *voiceTurn
	speaking  bool // audio queued and not yet played
	lastSound time.Time
	notices   []string
	watched   map[string]*watchedTask

	// The conversation with the agent as last read, so a turn need not
	// wait for the hub: read again when the person starts talking.
	convoMu sync.Mutex
	convo   string
	convoAt time.Time
	convoIn chan struct{} // a read under way
}

type voiceTurn struct {
	cancel context.CancelFunc
	id     int // the assistant line being said
	said   *strings.Builder
	done   chan struct{}
}

type watchedTask struct {
	ref      VoiceTaskRef
	what     string // what it was asked, short
	asked    string // the question it asked last (told once)
	finished bool
	toldAt   time.Time // when the person last heard about it
	progress string    // the progress note told last
}

func newVoiceCall(v *voice, d voiceDeps) *voiceCall {
	if d.endOfSpeech == 0 {
		d.endOfSpeech = 900 * time.Millisecond
	}
	if d.quietBeforeNotice == 0 {
		d.quietBeforeNotice = 1500 * time.Millisecond
	}
	if d.watchEvery == 0 {
		d.watchEvery = 4 * time.Second
	}
	if d.stillWorkingEvery == 0 {
		d.stillWorkingEvery = 2 * time.Minute
	}
	ctx, stop := context.WithCancel(context.Background())
	return &voiceCall{v: v, d: d, ctx: ctx, stop: stop, in: make(chan func(), 64), watched: map[string]*watchedTask{}}
}

// begin starts the loop and the watcher; the microphone is already on.
func (c *voiceCall) begin() {
	c.v.setState(stListening)
	go c.loop()
	go c.watch()
	if c.d.greet {
		c.do(c.greetTurn)
	}
	// What the agent is doing already: its answer is news too.
	go func() {
		if ref, err := c.d.hub.Current(c.ctx); err == nil && ref != nil {
			c.watchTask(*ref, "")
		}
	}()
}

func (c *voiceCall) do(f func()) {
	select {
	case c.in <- f:
	case <-c.ctx.Done():
	}
}

func (c *voiceCall) loop() {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-c.ctx.Done():
			if c.d.ended != nil {
				c.d.ended(c.history)
			}
			return
		case f := <-c.in:
			f()
		case <-tick.C:
			c.maybeTellNews()
		}
	}
}

// hangUp ends the call.
func (c *voiceCall) hangUp() {
	c.once.Do(func() {
		c.d.audio.Flush()
		c.d.audio.Stop()
		c.stop()
		v := c.v
		v.mu.Lock()
		if v.call == c {
			v.call = nil
		}
		v.state = stIdle
		v.wakeLocked()
		v.mu.Unlock()
	})
}

// --- what the platform says (VoiceEvents) ---

func (c *voiceCall) Heard(text string, final bool) {
	c.do(func() { c.heard(strings.TrimSpace(text), final) })
}

func (c *voiceCall) Played() {
	c.do(func() {
		c.speaking = false
		c.lastSound = time.Now()
		if c.turn == nil {
			c.v.setState(stListening)
		}
	})
}

func (c *voiceCall) Ended(err string) {
	if err != "" {
		c.v.mu.Lock()
		c.v.add(VoiceEvent{Kind: vNotice, Text: micError(c.en(), err)})
		c.v.mu.Unlock()
	}
	c.hangUp()
}

func micError(en bool, err string) string {
	say := func(zh, en2 string) string {
		if en {
			return en2
		}
		return zh
	}
	switch err {
	case "mic-denied":
		return say("没有麦克风权限：在「系统设置 › 隐私与安全性 › 麦克风」里允许 myco。",
			"No microphone access: allow myco under System Settings › Privacy & Security › Microphone.")
	case "speech-denied":
		return say("没有语音识别权限：在「系统设置 › 隐私与安全性 › 语音识别」里允许 myco。",
			"No speech recognition access: allow myco under System Settings › Privacy & Security › Speech Recognition.")
	case "unavailable":
		return say("这台电脑上现在用不了语音识别。", "Speech recognition is not available on this computer right now.")
	}
	return say("麦克风停了：", "The microphone stopped: ") + err
}

// heard: the person's words so far. While the assistant talks, real words
// (not "嗯", not its own voice coming back) cut it off.
func (c *voiceCall) heard(text string, final bool) {
	c.v.mu.Lock()
	muted := c.muted
	c.v.mu.Unlock()
	if muted || text == "" {
		return
	}
	if (c.turn != nil || c.speaking) && !c.interrupts(text) {
		return
	}
	if c.turn != nil || c.speaking {
		c.interrupt()
	}
	if c.partial == "" {
		c.readConvo() // ready by the time they finish
	}
	c.partial, c.partialAt = text, time.Now()
	c.lastSound = c.partialAt
	c.v.mu.Lock()
	c.v.add(VoiceEvent{Kind: vHearing, Text: text})
	c.v.mu.Unlock()
	if c.endTimer != nil {
		c.endTimer.Stop()
	}
	if final {
		c.finishUtterance()
		return
	}
	at := c.partialAt
	c.endTimer = time.AfterFunc(c.d.endOfSpeech, func() {
		c.do(func() {
			if c.partialAt.Equal(at) {
				c.finishUtterance()
			}
		})
	})
}

// backchannel: what people say to show they listen; not a turn.
var backchannel = map[string]bool{"嗯": true, "嗯嗯": true, "啊": true, "哦": true, "噢": true, "对": true, "对对": true, "好": true, "是": true, "嗯哼": true, "ok": true, "okay": true, "mm": true, "uh-huh": true, "yeah": true}

// interrupts says whether words heard while the assistant talks are the
// person cutting in: at least two real characters, not a backchannel, and
// not what the assistant is saying coming back through the microphone.
func (c *voiceCall) interrupts(text string) bool {
	t := normalizeSpeech(text)
	if len([]rune(t)) < 2 || backchannel[t] {
		return false
	}
	if c.turn != nil {
		if said := normalizeSpeech(c.turn.said.String()); said != "" && strings.Contains(said, t) {
			return false
		}
	}
	return true
}

func normalizeSpeech(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// interrupt stops the assistant: nothing more is said, and what it got to
// say is kept as said.
func (c *voiceCall) interrupt() {
	c.d.audio.Flush()
	c.speaking = false
	if t := c.turn; t != nil {
		t.cancel()
		c.turn = nil
		said := strings.TrimSpace(t.said.String())
		c.v.mu.Lock()
		c.v.update(t.id, said, true)
		c.v.mu.Unlock()
		if said != "" {
			c.history = append(c.history, chatMsg{Role: "assistant", Content: said + c.say("（被打断了）", " (cut off)")})
		}
	}
	c.v.setState(stListening)
}

// finishUtterance: the person has finished; the assistant answers.
func (c *voiceCall) finishUtterance() {
	text := strings.TrimSpace(c.partial)
	c.partial = ""
	if c.endTimer != nil {
		c.endTimer.Stop()
		c.endTimer = nil
	}
	if text == "" || backchannel[normalizeSpeech(text)] {
		c.v.mu.Lock()
		if n := len(c.v.events); n > 0 && c.v.events[n-1].Kind == vHearing {
			c.v.events = c.v.events[:n-1]
			c.v.wakeLocked()
		}
		c.v.mu.Unlock()
		return
	}
	c.v.mu.Lock()
	c.v.add(VoiceEvent{Kind: vUser, Text: text})
	c.v.mu.Unlock()
	c.history = append(c.history, chatMsg{Role: "user", Content: text})
	c.startTurn()
}

// greetTurn: the call is on; the agent says hello first, in a few words,
// and mentions what it has at work or is waiting on, if anything. The
// person speaking first cuts it off like any other turn.
func (c *voiceCall) greetTurn() {
	if c.turn != nil || c.partial != "" || len(c.history) > 0 {
		return // they spoke first
	}
	lead := "（系统消息，不是对方说的）电话刚接通，你先开口：用一句很短的话打招呼，像接起电话那样，用第一人称（比如「我在，有什么要我做的？」）。你手上有活在做、或者在等对方回答，就顺带用半句话提一下；没有就不提。别说你是语音助手，别念对话原文。"
	if c.en() {
		lead = "(System message, not said by the person.) The call has just connected and you speak first: one short sentence of hello, as when picking up the phone, in the first person (e.g. \"Hi, I'm here. What can I do for you?\"). If you have work in progress or are waiting on their answer, mention it in half a sentence; otherwise do not. Do not call yourself a voice assistant, and do not read out the chat."
	}
	c.history = append(c.history, chatMsg{Role: "system", Content: lead})
	c.startTurn()
}

// maybeTellNews: in a quiet moment, news about handed-out tasks becomes a
// turn of its own.
func (c *voiceCall) maybeTellNews() {
	if len(c.notices) == 0 || c.turn != nil || c.speaking || c.partial != "" {
		return
	}
	if time.Since(c.lastSound) < c.d.quietBeforeNotice {
		return
	}
	news := strings.Join(c.notices, "\n")
	c.notices = nil
	lead := "（系统消息，不是对方说的）你自己干活那边有了新情况。用第一人称（「我……」）、一两句自然的话告诉对方，别念原文：\n"
	if c.en() {
		lead = "(System message, not said by the person.) There is news from your own work. Tell them in a sentence or two, in the first person (\"I...\"), in your own words; do not read it out:\n"
	}
	c.history = append(c.history, chatMsg{Role: "system", Content: lead + news})
	c.startTurn()
}

// startTurn runs the model on the conversation and says what it writes.
func (c *voiceCall) startTurn() {
	ctx, cancel := context.WithCancel(c.ctx)
	c.v.mu.Lock()
	ev := c.v.add(VoiceEvent{Kind: vAssistant, Text: ""})
	c.v.mu.Unlock()
	t := &voiceTurn{cancel: cancel, id: ev.ID, said: &strings.Builder{}, done: make(chan struct{})}
	c.turn = t
	c.v.setState(stThinking)
	go c.runTurn(ctx, t, trimHistory(c.history))
}

// trimHistory keeps the conversation to its last messages, starting at a
// person's or system line (never a tool result without its call).
func trimHistory(h []chatMsg) []chatMsg {
	const keep = 30
	if len(h) <= keep {
		return h
	}
	h = h[len(h)-keep:]
	for len(h) > 0 && (h[0].Role == "tool" || (h[0].Role == "assistant" && len(h[0].ToolCalls) > 0)) {
		h = h[1:]
	}
	return h
}

// runTurn: model rounds (tools in between), each sentence spoken as soon
// as it is complete. Runs off the loop; reports back through c.do.
func (c *voiceCall) runTurn(ctx context.Context, t *voiceTurn, history []chatMsg) {
	defer close(t.done)
	// The conversation with the agent as it is now (it moves on its own).
	convo := c.conversation(ctx)
	msgs := append([]chatMsg{{Role: "system", Content: voiceSystemPrompt(c.d.lang, c.d.hub.Name(), convo) + c.d.memo}}, history...)
	sentences := make(chan string, 16)
	spoke := make(chan struct{})
	go func() {
		defer close(spoke)
		for s := range sentences {
			if ctx.Err() != nil {
				continue
			}
			c.do(func() {
				if c.turn == t {
					c.speaking = true
					c.v.setState(stSpeaking)
				}
			})
			err := c.d.tts.Speak(ctx, s, func(pcm []byte, rate int) {
				if ctx.Err() == nil {
					c.d.audio.Play(pcm, rate)
				}
			})
			if err != nil && ctx.Err() == nil {
				c.notice(c.say("说不出声：", "Could not speak: ") + err.Error())
			}
		}
	}()
	var added []chatMsg
	split := &sentenceSplitter{}
	emit := func(s string) {
		if strings.TrimSpace(s) == "" {
			return
		}
		t.said.WriteString(s)
		said := t.said.String()
		c.do(func() {
			if c.turn == t {
				c.v.mu.Lock()
				c.v.update(t.id, strings.TrimSpace(said), false)
				c.v.mu.Unlock()
			}
		})
		sentences <- s
	}
	var failed error
	// About to act with nothing said yet: a few words first, so the person
	// is not left in silence while the model writes the message out.
	onTool := func(name string) {
		if strings.TrimSpace(t.said.String()) != "" {
			return
		}
		if rest := split.flush(); rest != "" {
			emit(rest)
			return
		}
		emit(toolAck(c.d.lang, name))
	}
	for round := 0; round < 5 && ctx.Err() == nil; round++ {
		reply, err := c.d.model.Stream(ctx, msgs, voiceTools, func(delta string) {
			for _, s := range split.add(delta) {
				emit(s)
			}
		}, onTool)
		if err != nil {
			failed = err
			break
		}
		if rest := split.flush(); rest != "" {
			emit(rest)
		}
		msgs = append(msgs, reply)
		added = append(added, reply)
		if len(reply.ToolCalls) == 0 {
			break
		}
		// What it did shows in the conversation: read it again next time.
		c.convoMu.Lock()
		c.convoAt = time.Time{}
		c.convoMu.Unlock()
		for _, call := range reply.ToolCalls {
			out := c.runTool(ctx, call)
			res := chatMsg{Role: "tool", ToolCallID: call.ID, Content: out}
			msgs = append(msgs, res)
			added = append(added, res)
		}
	}
	close(sentences)
	<-spoke
	c.do(func() {
		if c.turn != t {
			return // cut off: interrupt() kept what was said
		}
		c.turn = nil
		if failed != nil && ctx.Err() == nil {
			c.v.mu.Lock()
			c.v.add(VoiceEvent{Kind: vNotice, Text: c.say("语音没能回答：", "The voice could not answer: ") + failed.Error()})
			c.v.mu.Unlock()
		}
		said := strings.TrimSpace(t.said.String())
		c.v.mu.Lock()
		if said == "" {
			// Nothing said (only actions): drop the empty line.
			for i := len(c.v.events) - 1; i >= 0; i-- {
				if c.v.events[i].ID == t.id {
					c.v.events = append(c.v.events[:i], c.v.events[i+1:]...)
					c.v.wakeLocked()
					break
				}
			}
		}
		c.v.mu.Unlock()
		c.history = append(c.history, added...)
		if !c.speaking {
			c.v.setState(stListening)
		}
	})
}

func (c *voiceCall) notice(text string) {
	c.v.mu.Lock()
	c.v.add(VoiceEvent{Kind: vNotice, Text: text})
	c.v.mu.Unlock()
}

// sentenceSplitter cuts streamed text into pieces worth saying on their
// own: at the end of a sentence, or at a comma once a piece is long.
type sentenceSplitter struct{ buf strings.Builder }

func (s *sentenceSplitter) add(delta string) []string {
	var out []string
	for _, r := range delta {
		s.buf.WriteRune(r)
		n := len([]rune(s.buf.String()))
		switch {
		case strings.ContainsRune("。！？!?；;\n", r):
			out = append(out, s.take())
		case strings.ContainsRune("，,、：:", r) && n >= 18:
			out = append(out, s.take())
		case r == '.' && n >= 2:
			out = append(out, s.take())
		}
	}
	return out
}

func (s *sentenceSplitter) take() string {
	t := s.buf.String()
	s.buf.Reset()
	return t
}

func (s *sentenceSplitter) flush() string {
	t := strings.TrimSpace(s.take())
	return t
}

// --- watching the agent's tasks ---

func (c *voiceCall) watchTask(ref VoiceTaskRef, what string) {
	c.do(func() {
		if w := c.watched[ref.ID]; w != nil {
			if w.what == "" {
				w.what = what
			}
			return
		}
		c.watched[ref.ID] = &watchedTask{ref: ref, what: what, toldAt: time.Now()}
	})
}

func (c *voiceCall) watch() {
	tick := time.NewTicker(c.d.watchEvery)
	defer tick.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-tick.C:
		}
		var todo []*watchedTask
		done := make(chan struct{})
		c.do(func() {
			for _, w := range c.watched {
				if !w.finished {
					todo = append(todo, w)
				}
			}
			close(done)
		})
		select {
		case <-done:
		case <-c.ctx.Done():
			return
		}
		for _, w := range todo {
			st, err := c.d.hub.TaskNews(c.ctx, w.ref)
			if err != nil {
				continue
			}
			w := w
			c.do(func() {
				what := ""
				if w.what != "" {
					what = c.say("「"+w.what+"」", " \""+w.what+"\"")
				}
				switch {
				case st.Finished:
					w.finished = true
					w.toldAt = time.Now()
					c.notices = append(c.notices, fmt.Sprintf(c.say("你做完了%s，你在对话里的回复：%s", "You finished%s; your reply in the chat: %s"), what, st.Summary))
				case st.Question != "" && st.Question != w.asked:
					w.asked = st.Question
					w.toldAt = time.Now()
					c.notices = append(c.notices, fmt.Sprintf(c.say("你做%s时有个问题要问对方：%s（你在等对方回答；对方回答了就用 do_work 记下来）",
						"While working on%s you need to ask them: %s (you are waiting for the answer; when they answer, record it with do_work)"), what, st.Question))
				case st.Question == "" && c.stillWorkingDue(w):
					w.toldAt = time.Now()
					c.notices = append(c.notices, c.stillWorking(what, w, st))
				}
			})
		}
	}
}

// toolAck is what is said when the model starts an action before saying
// anything: the agent speaking of its own work.
func toolAck(lang, tool string) string {
	en := strings.HasPrefix(lang, "en")
	switch tool {
	case "do_work":
		if en {
			return "Sure, I'm on it. "
		}
		return "好，我这就去做。"
	case "stop_work":
		if en {
			return "OK, I'll stop. "
		}
		return "好，我先停下。"
	case "check_work":
		if en {
			return "Let me check. "
		}
		return "我看一下。"
	}
	if en {
		return "One moment. "
	}
	return "稍等。"
}

// en: the call is in English.
func (c *voiceCall) en() bool { return strings.HasPrefix(c.d.lang, "en") }

// say picks the call's language.
func (c *voiceCall) say(zh, en string) string {
	if c.en() {
		return en
	}
	return zh
}

// readConvo starts reading the conversation with the agent, unless one is
// under way or the last is a moment old.
func (c *voiceCall) readConvo() <-chan struct{} {
	c.convoMu.Lock()
	defer c.convoMu.Unlock()
	if c.convoIn != nil {
		return c.convoIn
	}
	done := make(chan struct{})
	if time.Since(c.convoAt) < 2*time.Second {
		close(done)
		return done
	}
	c.convoIn = done
	go func() {
		ctx, cancel := context.WithTimeout(c.ctx, 4*time.Second)
		convo, err := c.d.hub.Context(ctx)
		cancel()
		if err != nil && convo == "" {
			convo = c.say("（这会儿读不到对话：", "(The chat cannot be read right now: ") + err.Error() + c.say("）", ")")
		}
		c.convoMu.Lock()
		c.convo, c.convoAt, c.convoIn = convo, time.Now(), nil
		c.convoMu.Unlock()
		close(done)
	}()
	return done
}

// conversation is the conversation with the agent for a turn: read when
// the person started talking (or now, if that was a while ago).
func (c *voiceCall) conversation(ctx context.Context) string {
	c.convoMu.Lock()
	fresh := c.convoIn == nil && time.Since(c.convoAt) < 10*time.Second
	convo := c.convo
	c.convoMu.Unlock()
	if fresh {
		return convo
	}
	select {
	case <-c.readConvo():
	case <-ctx.Done():
	}
	c.convoMu.Lock()
	defer c.convoMu.Unlock()
	return c.convo
}

// stillWorkingDue: a task not done has gone unmentioned for a while, and
// the call has been quiet for a good part of it (not in the middle of a
// conversation about something else).
func (c *voiceCall) stillWorkingDue(w *watchedTask) bool {
	every := c.d.stillWorkingEvery
	return time.Since(w.toldAt) >= every && time.Since(c.lastSound) >= every/2 && c.turn == nil && !c.speaking
}

// stillWorking is the notice that a task is still going: how it stands,
// for how long, and its newest progress note if not told yet (all of it
// the agent's own work, said as "I").
func (c *voiceCall) stillWorking(what string, w *watchedTask, st taskNews) string {
	var b strings.Builder
	if c.en() {
		b.WriteString("(Not a result, only so they know you are still at it) Your work" + what + ": " + statusWord(true, st.Status))
	} else {
		b.WriteString("（不是结果，只是让对方知道你还在做）你手上的" + what + "：" + statusWord(false, st.Status))
	}
	if !st.Since.IsZero() {
		if m := int(time.Since(st.Since).Minutes()); m >= 1 {
			fmt.Fprintf(&b, c.say("，已经 %d 分钟了", ", for %d min now"), m)
		}
	}
	if st.Progress != "" && st.Progress != w.progress {
		w.progress = st.Progress
		b.WriteString(c.say("；你最新写的进展：", "; your latest progress note: ") + st.Progress)
	}
	b.WriteString(c.say("。用一句很短的话、用「我」告诉对方你还在做（可以带上在做什么），别说做完了，也不用问对方要不要等。",
		". In one short sentence, in the first person, tell them you are still on it (you can say what you are doing); do not say it is done, and do not ask whether they want to wait."))
	return b.String()
}
