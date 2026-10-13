package audit

import (
	"testing"
	"time"
)

func TestHeatCellsUseActualIntervals(t *testing.T) {
	shanghai, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 10, 12, 14, 30, 0, 0, shanghai)
	cases := []struct {
		name string
		from time.Time
		to   time.Time
		zone string
		want int
		mode string
	}{
		{"rolling day", now.Add(-24 * time.Hour), now, "Asia/Shanghai", 24, "hour"},
		{"seven dates", time.Date(2026, 10, 6, 0, 0, 0, 0, shanghai), now, "Asia/Shanghai", 6*24 + 15, "hour"},
		{"thirty dates", time.Date(2026, 9, 13, 0, 0, 0, 0, shanghai), now, "Asia/Shanghai", 30, "day"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cells, mode := heatCells(Filter{From: c.from.UnixMilli(), To: c.to.UnixMilli(), Timezone: c.zone})
			if len(cells) != c.want || mode != c.mode {
				t.Fatalf("got %d %s", len(cells), mode)
			}
			cursor := c.from.UnixMilli()
			for _, cell := range cells {
				if cell.StartedAt != cursor || cell.EndedAt <= cursor || cell.EndedAt > c.to.UnixMilli() {
					t.Fatalf("invalid interval: %+v", cell)
				}
				cursor = cell.EndedAt
			}
			if cursor != c.to.UnixMilli() {
				t.Fatal("range not fully covered")
			}
		})
	}
}

func TestHeatCellsDSTAndCalendarDays(t *testing.T) {
	location, _ := time.LoadLocation("America/New_York")
	for _, c := range []struct {
		month      time.Month
		day, hours int
	}{
		{time.March, 7, 47},
		{time.October, 31, 49},
	} {
		from := time.Date(2026, c.month, c.day, 0, 0, 0, 0, location)
		to := from.AddDate(0, 0, 2)
		cells, _ := heatCells(Filter{From: from.UnixMilli(), To: to.UnixMilli(), Timezone: location.String()})
		if len(cells) != c.hours {
			t.Fatalf("DST range: got %d hours, want %d", len(cells), c.hours)
		}
		for i := 1; i < len(cells); i++ {
			if cells[i-1].EndedAt != cells[i].StartedAt {
				t.Fatal("gap or overlap")
			}
		}
	}
	from := time.Date(2026, 10, 15, 0, 0, 0, 0, location)
	to := from.AddDate(0, 0, 30)
	cells, mode := heatCells(Filter{From: from.UnixMilli(), To: to.UnixMilli(), Timezone: location.String()})
	if mode != "day" || len(cells) != 30 {
		t.Fatal(mode, len(cells))
	}
	foundFold := false
	for _, cell := range cells {
		if cell.EndedAt-cell.StartedAt == 25*3600000 {
			foundFold = true
		}
	}
	if !foundFold {
		t.Fatal("daily cells ignored the DST fold")
	}
}

func TestHeatCellsMidnightTransitions(t *testing.T) {
	for _, c := range []struct {
		zone, date string
		hours      int
	}{
		{"America/Santiago", "2026-09-06", 23},
		{"America/Havana", "2026-03-08", 23},
		{"America/Havana", "2026-11-01", 25},
	} {
		t.Run(c.zone+"/"+c.date, func(t *testing.T) {
			location, err := time.LoadLocation(c.zone)
			if err != nil {
				t.Fatal(err)
			}
			// Noon exists on these transition dates, unlike the skipped midnight.
			noon, err := time.ParseInLocation("2006-01-02 15:04", c.date+" 12:00", location)
			if err != nil {
				t.Fatal(err)
			}
			for _, days := range []int{3, 10} {
				t.Run(map[int]string{3: "hour", 10: "day"}[days], func(t *testing.T) {
					start := noon.AddDate(0, 0, -1)
					from := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, location)
					to := from.AddDate(0, 0, days)
					cells, mode := heatCells(Filter{From: from.UnixMilli(), To: to.UnixMilli(), Timezone: c.zone})
					want := days
					if days == 3 {
						want = 2*24 + c.hours
					}
					if len(cells) != want || mode != map[int]string{3: "hour", 10: "day"}[days] {
						t.Fatalf("got %d %s cells, want %d", len(cells), mode, want)
					}
					cursor, duration := from.UnixMilli(), int64(0)
					for _, cell := range cells {
						if cell.StartedAt != cursor || cell.EndedAt <= cursor || cell.EndedAt > to.UnixMilli() {
							t.Fatalf("invalid interval: %+v", cell)
						}
						if time.UnixMilli(cell.StartedAt).In(location).Format("2006-01-02") == c.date {
							duration += cell.EndedAt - cell.StartedAt
						}
						cursor = cell.EndedAt
					}
					if cursor != to.UnixMilli() || duration != int64(c.hours)*3600000 {
						t.Fatalf("range end %d, transition duration %d", cursor, duration)
					}
				})
			}
		})
	}
}

func TestNextCalendarDaySkipsMissingDate(t *testing.T) {
	location, err := time.LoadLocation("Pacific/Apia")
	if err != nil {
		t.Fatal(err)
	}
	// Apia skipped all of December 30 when it crossed the date line.
	at := time.Date(2011, 12, 29, 23, 59, 59, 999000000, location)
	boundary := nextCalendarDay(at)
	if boundary.Format("2006-01-02 15:04:05") != "2011-12-31 00:00:00" || boundary.Sub(at) != time.Millisecond {
		t.Fatalf("next available date: %s", boundary)
	}
}
