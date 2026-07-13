package cli

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCompactRowAnnotatesMentionsInStableDeduplicatedOrder(t *testing.T) {
	t.Parallel()
	row := map[string]any{
		"id":     "cmt_1",
		"status": "open",
		"body":   "Ping @Alice and @review-bot",
		"mentions": []any{
			map[string]any{"principal": map[string]any{"type": "user", "id": "usr_1", "name": "Alice", "email": "alice@example.com"}},
			map[string]any{"principal": map[string]any{"type": "user", "id": "usr_1", "name": "Renamed duplicate"}},
			map[string]any{"principal": map[string]any{"type": "service_account", "id": "sa_1", "name": "review-bot"}},
			map[string]any{"principal": map[string]any{"name": "missing identity"}},
		},
	}
	want := "id=cmt_1  status=open  body=Ping @Alice and @review-bot  mentions=@Alice,@review-bot"
	if got := compactRow(row); got != want {
		t.Fatalf("compactRow()\n got: %q\nwant: %q", got, want)
	}

	row["mentions"] = []any{}
	if got := compactRow(row); strings.Contains(got, "mentions=") {
		t.Fatalf("empty mention annotation rendered in %q", got)
	}
}

func TestJSONRenderingLeavesMentionPayloadUnchanged(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"items":[{"id":"cmt_1","body":"Ping @Alice","mentions":[{"principal":{"type":"user","id":"usr_1","name":"Alice"},"offset":5,"length":6}]}]}`)
	var out bytes.Buffer
	if err := render(&out, &globals{json: true}, payload); err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON render changed payload\n got: %#v\nwant: %#v", got, want)
	}
}
