package bitrix

import (
	"encoding/json"
	"testing"
)

func TestParseArgs(t *testing.T) {
	got, err := ParseArgs([]string{"id=12", "filter[STAGE_ID]=WON", "filter[>DATE_MODIFY]=2026-10-01", "select[]=ID", "select[]=TITLE", "order[ID]=DESC", "q=a=b"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	want := `{"filter":{"\u003eDATE_MODIFY":"2026-10-01","STAGE_ID":"WON"},"id":"12","order":{"ID":"DESC"},"q":"a=b","select":["ID","TITLE"]}`
	if string(b) != want {
		t.Fatalf("\n%s\n%s", b, want)
	}
	for _, bad := range [][]string{{"noequals"}, {"a[b=1"}, {"a=1", "a[b]=2"}, {"x[][y]=1"}} {
		if _, err := ParseArgs(bad); err == nil {
			t.Errorf("принято %v", bad)
		}
	}
}

func TestUnslash(t *testing.T) {
	for in, want := range map[string]string{
		`"https:\/\/x.ru\/a"`: `"https://x.ru/a"`,
		`"c:\\/path"`:         `"c:\\/path"`,
		`"\"q\"\/"`:           `"\"q\"/"`,
	} {
		if got := string(Unslash([]byte(in))); got != want {
			t.Errorf("%s → %s, ждали %s", in, got, want)
		}
	}
}
