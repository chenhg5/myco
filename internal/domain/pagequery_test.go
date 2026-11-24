package domain

import "testing"

func TestPageQueryRejectsUnsafeAndInvalidInputs(t *testing.T) {
	cases := []PageQueryInput{
		{Fields: "n,title);DROP TABLE page_records--"}, {Fields: "n.foo"}, {Fields: "n,,title"},
		{Sort: "-n);DROP"}, {Sort: "$.n"}, {Where: []string{"x' OR 1=1:yes"}},
		{Where: []string{"status"}}, {Where: []string{"n:gte:banana"}}, {Where: []string{"n:in:[]"}},
		{Where: []string{"flag:exists:maybe"}}, {Where: []string{`n:in:[1,{"x":1}]`}},
		{Where: []string{"n:1e999"}}, {Limit: -1}, {Limit: 1001}, {After: "bad", Count: true},
	}
	for _, in := range cases {
		if _, err := ParsePageQuery(in); !IsCode(err, CodeInvalidArgument) {
			t.Errorf("accepted invalid query %+v: %v", in, err)
		}
	}
	q, _ := ParsePageQuery(PageQueryInput{Limit: 1, Sort: "-n"})
	if _, err := q.Offset("page", "issues", 1); err != nil {
		t.Fatal(err)
	}
	q.After = "malformed"
	if _, err := q.Offset("page", "issues", 1); !IsCode(err, CodeInvalidArgument) {
		t.Fatal("bad cursor accepted")
	}
	forged := PageQuery{Sort: []RecordSort{{Field: "n');DROP TABLE page_records--"}}}
	if forged.Validate() == nil {
		t.Fatal("direct repository query bypasses field validation")
	}
}
