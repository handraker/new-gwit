package main

import (
	"slices"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestFilterAndHighlight(t *testing.T) {
	s := tcell.NewSimulationScreen("")
	s.Init()
	s.SetSize(120, 40)
	a := &App{s: s, keyword: newLineInput(""), servers: []*Server{
		{"db-mysql-01", "Mysql Master", []string{"alpha"}},
		{"db-pg-01", "postgres", []string{"beta"}},
		{"web-01", "웹 서버", []string{}},
	}}

	for _, r := range "DB  alpha" {
		a.keyword.handle(tcell.NewEventKey(tcell.KeyRune, r, 0))
	}
	a.filter(-1)
	if len(a.filtered) != 1 || a.filtered[0].Host != "db-mysql-01" {
		t.Fatalf("AND filter: got %v", a.filtered)
	}

	a.keyword = newLineInput("서버")
	a.filter(-1)
	if len(a.filtered) != 1 || a.filtered[0].Host != "web-01" {
		t.Fatalf("korean filter: got %v", a.filtered)
	}

	if got := highlight("Mysql-mysql", []string{"sql"}); !slices.Equal(got, []bool{false, false, true, true, true, false, false, false, true, true, true}) {
		t.Fatalf("highlight: %v", got)
	}

	in := newLineInput("abc")
	in.handle(tcell.NewEventKey(tcell.KeyLeft, 0, 0))
	in.handle(tcell.NewEventKey(tcell.KeyBackspace2, 0, 0))
	in.handle(tcell.NewEventKey(tcell.KeyRune, 'X', 0))
	if in.String() != "aXc" || in.cursor != 2 {
		t.Fatalf("lineInput: %q %d", in.String(), in.cursor)
	}

	a.draw() // must not panic with wide runes
}
