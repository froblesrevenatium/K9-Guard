package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var clockPattern = regexp.MustCompile(`^([01]?\d|2[0-3]):([0-5]\d)$`)

func parseSchedule(value string) (float64, []string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil, nil
	}
	if strings.Contains(value, ":") {
		var times []string
		for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
			match := clockPattern.FindStringSubmatch(part)
			if match == nil {
				return 0, nil, fmt.Errorf("hora %q no válida: usa HH:MM, por ejemplo 12:30 o 08:00, 18:30", part)
			}
			hour, _ := strconv.Atoi(match[1])
			normalized := fmt.Sprintf("%02d:%s", hour, match[2])
			if !contains(times, normalized) {
				times = append(times, normalized)
			}
		}
		sort.Strings(times)
		return 0, times, nil
	}
	hours, err := strconv.ParseFloat(strings.TrimSuffix(strings.ToLower(value), "h"), 64)
	if err != nil || hours < 0 {
		return 0, nil, fmt.Errorf("horario no válido: usa horas (6, 0.5) o horas del día (12:30, 08:00, 18:30)")
	}
	return hours, nil, nil
}

func formatSchedule(q Query) string {
	if len(q.At) > 0 {
		return strings.Join(q.At, ", ")
	}
	if q.EveryHours > 0 {
		return strconv.FormatFloat(q.EveryHours, 'f', -1, 64)
	}
	return ""
}

func describeSchedule(q Query) string {
	switch {
	case len(q.At) > 0:
		return "diario a las " + strings.Join(q.At, ", ")
	case q.EveryHours > 0:
		return "cada " + strconv.FormatFloat(q.EveryHours, 'f', -1, 64) + " h"
	default:
		return "manual"
	}
}

func lastOccurrence(times []string, now time.Time) time.Time {
	var latest time.Time
	for _, dayOffset := range []int{0, -1} {
		day := now.AddDate(0, 0, dayOffset)
		for _, clock := range times {
			match := clockPattern.FindStringSubmatch(clock)
			if match == nil {
				continue
			}
			hour, _ := strconv.Atoi(match[1])
			minute, _ := strconv.Atoi(match[2])
			candidate := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, now.Location())
			if !candidate.After(now) && candidate.After(latest) {
				latest = candidate
			}
		}
	}
	return latest
}

var (
	firstSeenMu sync.Mutex
	firstSeen   = map[string]time.Time{}
)

func observed(name string, now time.Time) time.Time {
	firstSeenMu.Lock()
	defer firstSeenMu.Unlock()
	if seen, ok := firstSeen[name]; ok {
		return seen
	}
	firstSeen[name] = now
	return now
}

func due(q Query, state QueryState, now time.Time) bool {
	if !q.Enabled {
		return false
	}
	if len(q.At) > 0 {
		occurrence := lastOccurrence(q.At, now)
		if occurrence.IsZero() {
			return false
		}
		baseline := state.LastRun
		if baseline.IsZero() {
			baseline = observed(q.Name, now)
		}
		return occurrence.After(baseline)
	}
	if q.EveryHours <= 0 {
		return false
	}
	if state.LastRun.IsZero() {
		return true
	}
	return !now.Before(state.LastRun.Add(time.Duration(q.EveryHours * float64(time.Hour))))
}
