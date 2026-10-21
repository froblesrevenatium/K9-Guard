package main

import (
	"reflect"
	"testing"
	"time"
)

func TestParseSchedule(t *testing.T) {
	cases := []struct {
		in    string
		hours float64
		at    []string
		err   bool
	}{
		{"", 0, nil, false},
		{"2", 2, nil, false},
		{"0.5h", 0.5, nil, false},
		{"12:30", 0, []string{"12:30"}, false},
		{"18:30, 8:00", 0, []string{"08:00", "18:30"}, false},
		{"08:00 08:00", 0, []string{"08:00"}, false},
		{"24:00", 0, nil, true},
		{"12:3", 0, nil, true},
		{"abc", 0, nil, true},
	}
	for _, tc := range cases {
		hours, at, err := parseSchedule(tc.in)
		if (err != nil) != tc.err || hours != tc.hours || !reflect.DeepEqual(at, tc.at) {
			t.Errorf("parseSchedule(%q) = %v %v %v", tc.in, hours, at, err)
		}
	}
}

func TestDailyDue(t *testing.T) {
	day := func(d, h, m int) time.Time { return time.Date(2026, 10, d, h, m, 0, 0, time.Local) }
	q := Query{Name: "diaria", Enabled: true, At: []string{"12:30", "18:00"}}
	ranYesterday := QueryState{LastRun: day(19, 18, 0)}
	if due(q, ranYesterday, day(20, 12, 29)) {
		t.Fatal("antes de las 12:30 no debe correr")
	}
	if !due(q, ranYesterday, day(20, 12, 31)) {
		t.Fatal("a las 12:31 debe correr")
	}
	if due(q, QueryState{LastRun: day(20, 12, 30).Add(5 * time.Second)}, day(20, 17, 59)) {
		t.Fatal("ya corrió a las 12:30, debe esperar a las 18:00")
	}
	if !due(q, QueryState{LastRun: day(19, 12, 30)}, day(20, 9, 0)) {
		t.Fatal("si la app estuvo cerrada a las 18:00 de ayer, debe ponerse al día")
	}
	fresh := Query{Name: "nueva", Enabled: true, At: []string{"12:30"}}
	if due(fresh, QueryState{}, day(20, 14, 0)) {
		t.Fatal("una consulta nueva no debe correr por una hora ya pasada")
	}
	if !due(fresh, QueryState{}, day(21, 12, 31)) {
		t.Fatal("una consulta nueva debe correr en la siguiente hora programada")
	}
	q.Enabled = false
	if due(q, ranYesterday, day(20, 12, 31)) {
		t.Fatal("desactivada no corre")
	}
}

func TestEveryHoursDue(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local)
	q := Query{Name: "cada", Enabled: true, EveryHours: 2}
	if !due(q, QueryState{}, now) {
		t.Fatal("sin ejecuciones debe correr de inmediato")
	}
	if due(q, QueryState{LastRun: now.Add(-time.Hour)}, now) {
		t.Fatal("antes de 2 h no debe correr")
	}
	if !due(q, QueryState{LastRun: now.Add(-2 * time.Hour)}, now) {
		t.Fatal("a las 2 h debe correr")
	}
}
