package audit

import "time"

// heatCells partitions the requested interval without overlapping weekdays or
// emitting future cells. Daily boundaries follow the selected IANA timezone.
func heatCells(f Filter) ([]HeatCell, string) {
	location, _ := time.LoadLocation(f.Timezone) // normalize validates the timezone.
	granularity := "hour"
	if *f.To-*f.From > int64(8*24*time.Hour/time.Millisecond) {
		granularity = "day"
	}
	rollingHours := *f.To-*f.From <= int64(24*time.Hour/time.Millisecond)
	cells := []HeatCell{}
	for at := time.UnixMilli(*f.From).In(location); at.UnixMilli() < *f.To; {
		next := at.Add(time.Hour)
		if !rollingHours {
			boundary := nextCalendarDay(at)
			if granularity == "day" || next.After(boundary) {
				next = boundary
			}
		}
		end := min(next.UnixMilli(), *f.To)
		cells = append(cells, HeatCell{StartedAt: at.UnixMilli(), EndedAt: end,
			Weekday: (int(at.Weekday()) + 6) % 7, Hour: at.Hour()})
		at = time.UnixMilli(end).In(location)
	}
	return cells, granularity
}

// nextCalendarDay finds the first instant after the current local calendar date.
// Constructing midnight with time.Date can normalize a skipped midnight back
// into this date. Search forward instead, including repeated midnights and
// skipped dates. The 48-hour window bounds the search, not the bucket duration.
func nextCalendarDay(at time.Time) time.Time {
	date := at.Format("2006-01-02")
	location := at.Location()
	lo, hi := at.Unix(), at.Add(48*time.Hour).Unix()
	for lo < hi {
		mid := lo + (hi-lo)/2
		if time.Unix(mid, 0).In(location).Format("2006-01-02") <= date {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return time.Unix(lo, 0).In(location)
}
