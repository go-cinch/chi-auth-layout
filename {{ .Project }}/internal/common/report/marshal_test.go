package report

import (
	"encoding/json"
	"testing"
)

func TestMarshalFields(t *testing.T) {
	type record struct {
		ID    int64   `json:"id"`
		Value *string `json:"value,omitempty"`
	}
	for _, test := range []struct {
		fields Fields
		want   string
	}{
		{nil, `{"id":0}`},
		{Fields{"value"}, `{"value":null}`},
		{Fields{"id", "value"}, `{"id":0,"value":null}`},
	} {
		data, err := MarshalFields(record{}, test.fields)
		if err != nil || string(data) != test.want {
			t.Fatalf("%s %v", data, err)
		}
	}
	if _, err := MarshalFields(json.RawMessage(`invalid`), Fields{"id"}); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
