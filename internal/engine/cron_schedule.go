package engine

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronSchedule represents a compiled cron schedule
type CronSchedule struct {
	hasSeconds bool
	sec        [60]bool
	min        [60]bool
	hour       [24]bool
	dom        [32]bool // 1..31
	mon        [13]bool // 1..12
	dow        [8]bool  // 0..7 (0 and 7 are Sunday)
}

// ParseCronSchedule parses and compiles a 5 or 6 field cron expression
func ParseCronSchedule(expr string) (*CronSchedule, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 && len(fields) != 6 {
		return nil, fmt.Errorf("cron expression must have 5 or 6 fields, got %d", len(fields))
	}

	sched := &CronSchedule{
		hasSeconds: len(fields) == 6,
	}

	offset := 0
	if sched.hasSeconds {
		if err := parseField(fields[0], 0, 59, sched.sec[:]); err != nil {
			return nil, fmt.Errorf("sec field: %w", err)
		}
		offset = 1
	} else {
		sched.sec[0] = true // 5-field cron triggers on 0 second
	}

	if err := parseField(fields[offset], 0, 59, sched.min[:]); err != nil {
		return nil, fmt.Errorf("min field: %w", err)
	}
	if err := parseField(fields[offset+1], 0, 23, sched.hour[:]); err != nil {
		return nil, fmt.Errorf("hour field: %w", err)
	}
	if err := parseField(fields[offset+2], 1, 31, sched.dom[:]); err != nil {
		return nil, fmt.Errorf("dom field: %w", err)
	}
	if err := parseField(fields[offset+3], 1, 12, sched.mon[:]); err != nil {
		return nil, fmt.Errorf("mon field: %w", err)
	}
	if err := parseField(fields[offset+4], 0, 7, sched.dow[:]); err != nil {
		return nil, fmt.Errorf("dow field: %w", err)
	}

	// Unify Sunday (0 and 7)
	if sched.dow[0] || sched.dow[7] {
		sched.dow[0] = true
		sched.dow[7] = true
	}

	return sched, nil
}

func parseField(field string, min, max int, target []bool) error {
	if field == "*" || field == "?" {
		for i := min; i <= max; i++ {
			target[i] = true
		}
		return nil
	}

	parts := strings.Split(field, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("empty part")
		}

		if strings.Contains(part, "/") {
			subParts := strings.Split(part, "/")
			if len(subParts) != 2 {
				return fmt.Errorf("invalid step syntax: %s", part)
			}
			base := subParts[0]
			step, err := strconv.Atoi(subParts[1])
			if err != nil || step <= 0 {
				return fmt.Errorf("invalid step value: %s", subParts[1])
			}

			startRange := min
			endRange := max
			if base != "*" && base != "" {
				if strings.Contains(base, "-") {
					rParts := strings.Split(base, "-")
					if len(rParts) != 2 {
						return fmt.Errorf("invalid range: %s", base)
					}
					s, err1 := strconv.Atoi(rParts[0])
					e, err2 := strconv.Atoi(rParts[1])
					if err1 != nil || err2 != nil || s < min || e > max || s > e {
						return fmt.Errorf("out-of-bounds range: %s", base)
					}
					startRange = s
					endRange = e
				} else {
					s, err := strconv.Atoi(base)
					if err != nil || s < min || s > max {
						return fmt.Errorf("out-of-bounds base: %s", base)
					}
					startRange = s
				}
			}

			for i := startRange; i <= endRange; i += step {
				target[i] = true
			}
			continue
		}

		if strings.Contains(part, "-") {
			rParts := strings.Split(part, "-")
			if len(rParts) != 2 {
				return fmt.Errorf("invalid range syntax: %s", part)
			}
			s, err1 := strconv.Atoi(rParts[0])
			e, err2 := strconv.Atoi(rParts[1])
			if err1 != nil || err2 != nil || s < min || e > max || s > e {
				return fmt.Errorf("out-of-bounds range: %s", part)
			}
			for i := s; i <= e; i++ {
				target[i] = true
			}
			continue
		}

		v, err := strconv.Atoi(part)
		if err != nil || v < min || v > max {
			return fmt.Errorf("out-of-bounds value: %s", part)
		}
		target[v] = true
	}

	return nil
}

// Matches checks if the given time matches the cron schedule
func (s *CronSchedule) Matches(t time.Time) bool {
	sec := t.Second()
	min := t.Minute()
	hour := t.Hour()
	dom := t.Day()
	mon := int(t.Month())
	dow := int(t.Weekday())

	if !s.sec[sec] || !s.min[min] || !s.hour[hour] || !s.dom[dom] || !s.mon[mon] {
		return false
	}

	return s.dow[dow]
}

// Next calculates the next matching time strictly after 'from'
func (s *CronSchedule) Next(from time.Time) time.Time {
	step := time.Minute
	curr := from.Truncate(time.Minute).Add(time.Minute)
	if s.hasSeconds {
		step = time.Second
		curr = from.Truncate(time.Second).Add(time.Second)
	}

	// Scan up to 366 days into the future
	limit := from.Add(366 * 24 * time.Hour)
	for curr.Before(limit) {
		if s.Matches(curr) {
			return curr
		}
		curr = curr.Add(step)
	}

	return time.Time{}
}
