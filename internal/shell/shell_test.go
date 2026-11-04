package shell

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		line string
		want []Segment
	}{
		{"just test", []Segment{{"just test", true}}},
		{"cd /x && just test", []Segment{{"cd /x", true}, {"just test", true}}},
		{"just test 2>&1 | tail -3", []Segment{{"just test 2>&1", false}, {"tail -3", true}}},
		{"just test; echo done", []Segment{{"just test", false}, {"echo done", true}}},
		{"(cd h && go test) && just test", []Segment{{"cd h", true}, {"go test", true}, {"just test", true}}},
		{`echo "a && b | c" && git push`, []Segment{{`echo "a && b | c"`, true}, {"git push", true}}},
		{"just test || true", []Segment{{"just test", false}, {"true", true}}},
		{"echo $(date)", []Segment{{"echo $(date)", true}}},
		{"(echo $(date)) && ls", []Segment{{"echo $(date)", true}, {"ls", true}}},
		{"((cd x && just test))", []Segment{{"cd x", true}, {"just test", true}}},
	}
	for _, c := range cases {
		if got := Split(c.line); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Split(%q) = %#v, want %#v", c.line, got, c.want)
		}
	}
}
