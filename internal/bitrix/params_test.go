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

// Оператор с «=» стоит внутри ключа: резать по нему нельзя (b24 0.1.1 из-за этого падал на >=CLOSEDATE).
func TestParseArgsOperatorsWithEquals(t *testing.T) {
	got, err := ParseArgs([]string{"filter[>=CLOSEDATE]=2025-01-01", "filter[<=CLOSEDATE]=2026-12-31", "filter[!=STAGE_ID]=NEW", "filter[%TITLE]=a=b"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	want := `{"filter":{"!=STAGE_ID":"NEW","%TITLE":"a=b","\u003c=CLOSEDATE":"2026-12-31","\u003e=CLOSEDATE":"2025-01-01"}}`
	if string(b) != want {
		t.Fatalf("\n%s\n%s", b, want)
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
