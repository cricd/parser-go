package main

import (
	"os"
	"path/filepath"
	"strings"

	"time"

	log "github.com/Sirupsen/logrus"
	cricd "github.com/cricd/cricd-go"
	cricsheet "github.com/cricd/cricsheet"
	"github.com/howeyc/fsnotify"
)

/*
	TODO:
		- Create a logging level
		- ENV var for logging level
		- Players DOB doesn't work
		- Players gender doesn't work
		- Tests
*/

var cricsheetToCricdMap = map[string]string{
	"bowled":                "bowled",
	"caught":                "caught",
	"caught and bowled":     "caught",
	"lbw":                   "lbw",
	"stumped":               "stumped",
	"run out":               "runOut",
	"retired hurt":          "retiredHurt",
	"hit wicket":            "hitWicket",
	"obstructing the field": "obstruction",
	"hit the ball twice":    "doubleHit",
	"handled the ball":      "handledBall",
	"timed out":             "timedOut",
	"legbyes":               "legBye",
	"noballs":               "noBall",
	"penalty":               "penaltyRuns",
	"wides":                 "wide",
	"byes":                  "bye",
}

// TODO: Should this be here as it's doing a translation
func mustDetermineEventType(delivery *cricsheet.Delivery) string {
	if delivery.Wicket.Kind != "" {
		return cricsheetToCricdMap[delivery.Wicket.Kind]
	} else if delivery.Extras != nil {
		for k := range delivery.Extras {
			return cricsheetToCricdMap[k]
		}
	} else {
		return "delivery"
	}
	return "delivery"
}

// TODO: ChangeName and implement properly
func cricdGetTeams(csts cricsheet.Teams) ([]cricd.Team, error) {
	var allTeams []cricd.Team
	for _, cst := range csts {
		ct := cricd.Team{Name: cst}
		ok, err := ct.Get()
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Failed to get team with error")
		}
		if !ok {
			log.Info("Failed to get team  without error so will try to create")
		} else {
			allTeams = append(allTeams, ct)
			continue
		}
		// try to create a team, because we couldn't get one
		ok, err = ct.Create()
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Failed to get team")
			return []cricd.Team{}, err
		}
		if !ok {
			log.Info("Failed to create team without error so quitting")
			return []cricd.Team{}, nil
		}
		allTeams = append(allTeams, ct)
		continue
	}
	return allTeams, nil
}

//TODO: Change name and test
func process(c cricsheet.Event) (cricd.Delivery, error) {
	var cdd cricd.Delivery
	allTeams, err := cricdGetTeams(c.Info.Teams)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to get or create teams so exiting")
		return cricd.Delivery{}, err
	}

	// Get all the match info
	var m cricd.Match
	m.HomeTeam = allTeams[0]
	m.AwayTeam = allTeams[1]
	m.LimitedOvers = c.Info.Overs

	if strings.ToLower(c.Info.MatchType) == "test" {
		m.NumberOfInnings = 2
	} else {
		m.NumberOfInnings = 1
	}
	m.StartDate, _ = time.Parse(cricd.DateFormat, c.Info.Dates[0]) // Assume the first date is the start
	ok, err := m.Get()
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to get match with error")
	}
	if !ok {
		log.Info("Failed to get match without error")
		// We didn't get a match so create it
		ok, err = m.Create()
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Failed to create match so exiting")
			return cricd.Delivery{}, err
		}
		if !ok {
			log.Error("Failed to create match without error")
			return cricd.Delivery{}, err
		}
	}

	//  Set the Match ID
	cdd.MatchID = m.ID
	// Timestamp
	cdd.Timestamp = m.StartDate.Format(cricd.DateFormat)

	if c.BattingTeam == allTeams[0].Name {
		cdd.Ball.BattingTeam = allTeams[0]
		cdd.Ball.FieldingTeam = allTeams[1]
	} else {
		cdd.Ball.BattingTeam = allTeams[1]
		cdd.Ball.FieldingTeam = allTeams[0]
	}

	// Ball and innings info
	cdd.Ball.Innings = c.InningsNumber
	cdd.Ball.Over = c.Delivery.Over
	cdd.Ball.Ball = c.Delivery.Ball

	// Set all the players

	// Get the striker
	var striker = cricd.Player{Name: c.Delivery.Batsman}
	ok, err = striker.GetOrCreatePlayer()
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to get or create striker")
		return cricd.Delivery{}, err
	}
	if !ok {
		log.Error("Failed to get or create striker without error")
		return cricd.Delivery{}, nil
	}
	cdd.Batsmen.Striker = striker

	// Get the non-striker
	var nonStriker = cricd.Player{Name: c.Delivery.NonStriker}
	ok, err = nonStriker.GetOrCreatePlayer()
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to get or create nonStriker")
		return cricd.Delivery{}, err
	}
	if !ok {
		log.Error("Failed to get or create nonStriker without error")
		return cricd.Delivery{}, nil
	}
	cdd.Batsmen.NonStriker = nonStriker

	// Get the bowler
	var bowler = cricd.Player{Name: c.Delivery.Bowler}
	ok, err = bowler.GetOrCreatePlayer()
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to get or create bowler")
		return cricd.Delivery{}, err
	}
	if !ok {
		log.Error("Failed to get or create bowler without error")
		return cricd.Delivery{}, nil
	}
	cdd.Bowler = bowler

	// Event type
	dType := mustDetermineEventType(&c.Delivery)
	cdd.EventType = dType

	// Set the runs
	if dType == "legBye" || dType == "bye" {
		cdd.Runs = c.Delivery.Runs.Extras
	} else {
		cdd.Runs = c.Delivery.Runs.Batsman
	}

	// Get the fielder
	var fielder cricd.Player
	if dType == "runOut" || dType == "stumped" || dType == "caught" {

		if len(c.Delivery.Wicket.Fielders) > 0 {
			fielder.Name = c.Delivery.Wicket.Fielders[0]
			ok, err = fielder.GetOrCreatePlayer()
			if err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Failed to get or create fielder")
				return cricd.Delivery{}, err
			}
			if !ok {
				log.Error("Failed to get or create fielder without error")
				return cricd.Delivery{}, nil
			}
			log.Errorf("Got a dismissal that should have a fielder without one")
			return cricd.Delivery{}, nil

		}
	} else if dType == "caughtAndBowled" {
		fielder = bowler
	}
	if (cricd.Player{}) != fielder {
		cdd.Fielder = &fielder
	}

	return cdd, nil
}

/*
// TODO: Tests
// func processAll(cses []cricsheet.Event) <-chan cricd.Delivery {
// 	out := make(chan cricd.Delivery, 50)
// 	go func() {
// 		for _, event := range cses {
// 			e, err := process(event)
// 			if err != nil {
// 				log.WithFields(log.Fields{"error": err}).Error("Failed to process event")
// 			}
// 			out <- e
// 		}
// 		close(out)
// 	}()
// 	return out
// }

// TODO: test
// func pushAll(in <-chan cricd.Delivery) {
// 	go func() {
// 		for e := range in {
// 			ok, err := e.Push()
// 			if err != nil {
// 				log.WithFields(log.Fields{"error": err}).Error("Failed to push event to Event API")
// 			}
// 			if !ok {
// 				log.Error("Failed to push event to Event API without error")
// 			}
// 		}
// 	}()
// 	return
// } */

func init() {
	debug := os.Getenv("DEBUG")
	if debug == "true" {
		log.WithFields(log.Fields{"value": "DEBUG"}).Info("Setting log level to debug")
		log.SetLevel(log.DebugLevel)
	} else {
		log.Info("Setting log level to info")
		log.SetLevel(log.InfoLevel)
	}
	log.SetOutput(os.Stdout)
}

func main() {

	// 1. Read a file from the folder (in parallel) and put it on a channel
	gameEnv := os.Getenv("GAME_PATH")
	if gameEnv != "" {
		log.WithFields(log.Fields{"value": "GAME_PATH"}).Info("Setting game path to the value provided by the ENV VAR ")

	} else {
		log.WithFields(log.Fields{"value": "GAME_PATH"}).Info("Unable to find env var, using default `/games/`")
		gameEnv = "/games/"
	}
	cwd, err := os.Getwd()
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Failed to get current working directory")
	}
	gamePath := filepath.Join(cwd, gameEnv)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Failed to start file watcher")
	}
	done := make(chan bool)
	go func() {
		for {
			select {
			case ev := <-watcher.Event:
				if ev.IsCreate() {
					// Check if it's a YAML file
					if strings.HasSuffix(ev.Name, ".yaml") {
						log.WithFields(log.Fields{"fileName": ev.Name}).Info("Found file with .YAML suffix to process")
						// 2. Parse the file in to a cricsheet.Game that holds all the events
						game := cricsheet.Game{}
						err = game.Read(ev.Name)
						if err != nil {
							log.WithFields(log.Fields{"error": err}).Fatal("Failed to get delivery information from file")
						}

						// 3. Flatten the game into a slice of events
						cses, err := game.Flatten()
						if err != nil {
							log.WithFields(log.Fields{"error": err}).Error("Failed to get over and ball for delivery")
							os.Exit(0)
						}

						// 4. Iterate over each event
						for _, event := range cses {
							e, err := process(event)
							if err != nil {
								log.WithFields(log.Fields{"error": err}).Error("Failed to process event")
							}
							ok, err := e.Push()
							if err != nil {
								log.WithFields(log.Fields{"error": err}).Error("Failed to push event to Event API")
								continue
							}
							if !ok {
								log.Error("Failed to push event to Event API without error")
								continue
							}
						}
						// Rename the file
						log.WithFields(log.Fields{"fileName": ev.Name}).Info("Removing file")
						err = os.Remove(ev.Name)
						if err != nil {
							log.WithFields(log.Fields{"error": err}).Error("Failed to remove file")
						}
						log.Infof("Successfully processed file %s", ev.Name)
					}

				}

			case err := <-watcher.Error:
				//TODO: Fix me with a proper error
				log.WithFields(log.Fields{"error": err}).Fatal("Error while watching directory")

			}
		}
	}()
	err = watcher.Watch(gamePath)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Failed to watch directory")
		os.Exit(1)
	}
	<-done

	watcher.Close()
}
