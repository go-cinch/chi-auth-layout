package report

import (
	"testing"
)

func TestProjectionSelectsRequestedFields(t *testing.T) {
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
