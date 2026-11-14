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
		if got := u.Add(s.in); got != s.want {
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
		if got := e.Add(s.in); got != s.want {
			t.Fatalf("Add(%q) = %q, want %q", s.in, got, s.want)
		}
	}
}

// A session that ended (a pause) keeps its words, even when the next one
// starts the same way.
func TestUtterancesCommit(t *testing.T) {
	var u Utterances
	u.Add("我们先看")
	u.Add("我们先看一下日志")
	u.Commit()
	if got := u.Add("我们"); got != "我们先看一下日志我们" {
		t.Fatalf("after a pause: %q", got)
	}
	if got := u.Add("我们再跑测试"); got != "我们先看一下日志我们再跑测试" {
		t.Fatalf("growing: %q", got)
	}
	u.Commit()
	u.Commit() // nothing new: no change
	if got := u.Add("ok"); got != "我们先看一下日志我们再跑测试ok" {
		t.Fatalf("again: %q", got)
	}
}
