package report

import (
	"encoding/json"
	"net/url"
	"reflect"
	"testing"
)

func TestParseFields(t *testing.T) {
	for _, raw := range []string{"f=", "f=total,,counts", "f=total,", "f=nope", "f=total,total", "f=total&f=counts", "f=Total", "f=+"} {
		query, _ := url.ParseQuery(raw)
		if _, err := ParseFields(query, "total", "counts"); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	fields, err := ParseFields(url.Values{"f": {" total , counts "}}, "total", "counts")
	if err != nil || !reflect.DeepEqual(fields, Fields{"total", "counts"}) {
		t.Fatal(fields, err)
	}
	fields, err = ParseFields(url.Values{}, "total")
	if err != nil || fields != nil || !fields.Has("total") {
		t.Fatal(fields, err)
	}
	if err := (Fields{}).Validate("total"); err == nil {
		t.Fatal("accepted empty selection")
	}
}

func TestSelectedSummaryKeepsZerosAndEmptyArrays(t *testing.T) {
	value := New("active")
	value.Fields = Fields{"total", "groups"}
	data, err := json.Marshal(value)
	if err != nil || string(data) != `{"groups":[],"total":0}` {
		t.Fatal(string(data), err)
	}
	var total int64
	projection := Projection{Fields: Fields{"total"}}
	projection.Add("total", "COUNT(*)", &total)
	projection.Add("counts", "expensive()", nil)
	if projection.SQL("records") != "SELECT COUNT(*) FROM records" || len(projection.Targets) != 1 {
		t.Fatal(projection)
	}
	if projection.SQL("") != "SELECT COUNT(*)" {
		t.Fatal(projection)
	}
}
