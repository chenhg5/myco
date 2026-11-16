package console

import "testing"

func TestUtterances(t *testing.T) {
	var u Utterances
	steps := []struct{ in, want string }{
		{"帮", "帮"},
		{"帮我看", "帮我看"},
		{"帮我看一下下载页", "帮我看一下下载页"},
		// a pause: it starts over with the next sentence
		{"然后", "帮我看一下下载页然后"},
		{"然后改一下标题", "帮我看一下下载页然后改一下标题"},
	}
	for _, s := range steps {
		if got := u.Add(s.in, false); got != s.want {
			t.Fatalf("Add(%q) = %q, want %q", s.in, got, s.want)
		}
	}
	var e Utterances
	for _, s := range []struct{ in, want string }{
		{"Please", "Please"},
		{"Please check the", "Please check the"},
		{"Please check the download page", "Please check the download page"},
		{"And", "Please check the download page And"},
		{"And fix the title", "Please check the download page And fix the title"},
		// a revision of the same sentence is not a new one
		{"And fix the titles", "Please check the download page And fix the titles"},
	} {
		if got := e.Add(s.in, false); got != s.want {
			t.Fatalf("Add(%q) = %q, want %q", s.in, got, s.want)
		}
	}
}

// A session that ended (a pause) keeps its words, even when the next one
// starts the same way.
func TestUtterancesCommit(t *testing.T) {
	var u Utterances
	u.Add("我们先看", false)
	u.Add("我们先看一下日志", false)
	u.Commit()
	if got := u.Add("我们", false); got != "我们先看一下日志我们" {
		t.Fatalf("after a pause: %q", got)
	}
	if got := u.Add("我们再跑测试", false); got != "我们先看一下日志我们再跑测试" {
		t.Fatalf("growing: %q", got)
	}
	u.Commit()
	u.Commit() // nothing new: no change
	if got := u.Add("ok", false); got != "我们先看一下日志我们再跑测试ok" {
		t.Fatalf("again: %q", got)
	}
}

// Stopping: the settled sentence may rewrite its first words; it replaces
// what was shown, it is not written again.
func TestUtterancesFinalRewritesTheStart(t *testing.T) {
	var u Utterances
	u.Add("我们先看一下日志", false)
	u.Commit()
	u.Add("妹那边", false)
	u.Add("妹那边那妹妹那边有做了一些改动", false)
	partial := "妹那边那妹妹那边有做了一些改动然后可能你可以参照着Mac那边的改动看移动端这边也能不能一起改一下"
	u.Add(partial, false)
	final := "妹可那边有做了一些改动然后可能你可以参照着Mac那边的改动看移动端这边也能不能一起改一下"
	if got := u.Add(final, true); got != "我们先看一下日志"+final {
		t.Fatalf("final: %q", got)
	}
	// its session ends, and the same final comes once more
	u.Commit()
	if got := u.Add(final, true); got != "我们先看一下日志"+final {
		t.Fatalf("final again: %q", got)
	}
}

// A partial that rewrites the start of a long sentence (about as long as
// it) is the same sentence, not a new one.
func TestUtterancesRevisionIsNotANewSentence(t *testing.T) {
	var u Utterances
	u.Add("那妹妹那边有做了一些改动", false)
	if got := u.Add("妹可那边有做了一些改动吧", false); got != "妹可那边有做了一些改动吧" {
		t.Fatalf("revision: %q", got)
	}
}
