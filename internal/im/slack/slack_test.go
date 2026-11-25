package slack

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestMrkdwn(t *testing.T) {
	in := "# Done\n**Fixed** the [build](https://ci/x?a=1&b=2) & tests\n```\na **b** <c>\n```"
	want := "*Done*\n*Fixed* the <https://ci/x?a=1&b=2|build> &amp; tests\n```\na **b** <c>\n```"
	if got := Mrkdwn(in); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestInbound(t *testing.T) {
	a := &Adapter{bot: "UBOT"}
	in, ok := a.inbound("C1", "channel", "U1", "<@UBOT> ask <@U2> &amp; go", "1.2", "")
	if !ok || !in.Mentioned || !in.Group || in.Text != "ask <@U2> & go" || in.ThreadKey != "ts:C1:1.2" || in.ReplyTo != "1.2" || in.MessageID != "C1:1.2" {
		t.Fatalf("%+v", in)
	}
	in, _ = a.inbound("C1", "channel", "U1", "more", "1.5", "1.2")
	if in.ThreadKey != "ts:C1:1.2" || in.ReplyTo != "1.2" || in.Mentioned {
		t.Fatalf("reply %+v", in)
	}
	in, _ = a.inbound("D1", "im", "U1", "hi", "3.4", "")
	if in.Group || in.ThreadKey != "chat:D1" {
		t.Fatalf("dm %+v", in)
	}
	if _, ok := a.inbound("D1", "im", "UBOT", "echo", "3.5", ""); ok {
		t.Fatal("its own message")
	}
}

func TestCreateURL(t *testing.T) {
	u, err := url.Parse(CreateURL("myco"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(u.Query().Get("manifest_json")), &m); err != nil {
		t.Fatal(err)
	}
	s := u.Query().Get("manifest_json")
	for _, want := range []string{`"socket_mode_enabled":true`, `"app_mention"`, `"message.im"`, `"chat:write"`, `"/myco"`} {
		if !strings.Contains(s, want) {
			t.Errorf("manifest lacks %s", want)
		}
	}
}
