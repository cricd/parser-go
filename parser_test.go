package main

import (
	"testing"

	"github.com/cricd/cricsheet"
	"github.com/stretchr/testify/assert"
)

func TestTranslateRuns(t *testing.T) {
	game := cricsheet.Game{}
	err := game.Read("test.yaml")
	if err != nil {
		t.Errorf("Failed to open testing file - test.yaml")
	}
	tests := []struct {
		name  string
		event cricsheet.Event
		runs  int
	}{{
		name: "Wides",
		event: cricsheet.Event{Info: game.Info, InningsNumber: 1, BattingTeam: "Australia",
			Delivery: cricsheet.Delivery{Over: 0, Ball: 1, Batsman: "AC Gilchrist", NonStriker: "MJ Clarke", Bowler: "DR Tuffey", Runs: cricsheet.Runs{Batsman: 0, Extras: 1, Total: 1}, Wicket: cricsheet.Wicket{}, Extras: map[string]int{"wides": 1}}},
		runs: 0,
	}, {
		name: "LegByes",
		event: cricsheet.Event{Info: game.Info, InningsNumber: 1, BattingTeam: "Australia",
			Delivery: cricsheet.Delivery{Over: 0, Ball: 2, Batsman: "AC Gilchrist", NonStriker: "MJ Clarke", Bowler: "DR Tuffey", Runs: cricsheet.Runs{Batsman: 0, Extras: 1, Total: 1}, Wicket: cricsheet.Wicket{}, Extras: map[string]int{"legbyes": 1}}},
		runs: 1,
	}, {
		name:  "Dot ball",
		event: cricsheet.Event{Info: game.Info, InningsNumber: 1, BattingTeam: "Australia", Delivery: cricsheet.Delivery{Over: 0, Ball: 3, Batsman: "MJ Clarke", NonStriker: "AC Gilchrist", Bowler: "DR Tuffey", Runs: cricsheet.Runs{Batsman: 0, Extras: 0, Total: 0}, Wicket: cricsheet.Wicket{}, Extras: nil}},
		runs:  0,
	}, {
		name:  "Runs",
		event: cricsheet.Event{Info: game.Info, InningsNumber: 1, BattingTeam: "Australia", Delivery: cricsheet.Delivery{Over: 0, Ball: 6, Batsman: "MJ Clarke", NonStriker: "AC Gilchrist", Bowler: "DR Tuffey", Runs: cricsheet.Runs{Batsman: 6, Extras: 0, Total: 6}, Wicket: cricsheet.Wicket{}, Extras: nil}},
		runs:  6,
	}, {
		name:  "Byes",
		event: cricsheet.Event{Info: game.Info, InningsNumber: 1, BattingTeam: "Australia", Delivery: cricsheet.Delivery{Over: 0, Ball: 7, Batsman: "MJ Clarke", NonStriker: "AC Gilchrist", Bowler: "DR Tuffey", Runs: cricsheet.Runs{Batsman: 0, Extras: 1, Total: 1}, Wicket: cricsheet.Wicket{}, Extras: map[string]int{"byes": 1}}},
		runs:  1,
	},
	}

	for _, test := range tests {
		runs := translateRuns(&test.event)
		assert.Equal(t, runs, test.runs)
	}

	/*
		tests := []struct {
			name     string
			event    cricsheet.Event
			delivery cricd.Delivery
			wantErr  bool
		}{
			name: "Wides",
			event: cricsheet.Event{
				Info:          game.Info,
				InningsNumber: 1,
				BattingTeam:   "Australia",
				Delivery: Delivery{
					Over:       0,
					Ball:       1,
					Batsman:    "AC Gilchrist",
					NonStriker: "MJ Clarke",
					Bowler:     "DR Tuffey",
					Runs: Runs{
						Batsman: 0,
						Extras:  1,
						Total:   1},
					Wicket: Wicket{},
					Extras: map[string]int{"wides": 1}}},
			delivery: cricd.Delivery{},
		}
	*/
	return
}
