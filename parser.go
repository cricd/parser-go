package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"time"

	log "github.com/Sirupsen/logrus"
	cricd "github.com/cricd/cricd-go"
	cricsheet "github.com/cricd/cricsheet"
)

/*
	TODO:
		- Players DOB doesn't work
		- Players gender doesn't work
		- Tests
*/

var cricsheetToCricdMap = map[string]string{
	"bowled":                "bowled",
	"caught":                "caught",
	"caught and bowled":     "caught", // We don't have caught and bowled in cricd
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

// Translateteams takes cricsheet teams and turns them into cricd teams
func translateTeams(csts cricsheet.Teams) ([]cricd.Team, error) {
	var allTeams []cricd.Team
	for _, cst := range csts {
		ct := cricd.NewTeam(nil)
		ct.Name = cst
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

// translateNumberOfInnings works out the number of innings in an event based on the event type
func translateNumberOfInnings(c *cricsheet.Event) (noOfInnings int) {
	if strings.ToLower(c.Info.MatchType) == "test" {
		return 2
	}
	return 1
}

func translateStartDate(c *cricsheet.Event) (s time.Time, err error) {
	date, err := time.Parse(cricd.DateFormat, c.Info.Dates[0]) // Assume the first date is the start
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to parse date for event")
		return time.Time{}, err
	}
	return date, nil
}

func translateFielder(c *cricsheet.Event) (name string) {
	dType := mustDetermineEventType(&c.Delivery)

	//  If it's a run out/stumped then the fielder will be in the wicket
	if dType == "runOut" || dType == "stumped" {
		if len(c.Delivery.Wicket.Fielders) > 0 {
			return c.Delivery.Wicket.Fielders[0]
		}
		log.Error("Failed to find fielder for event")
	}
	// If it's caught (or caught and bowled)
	if dType == "caught" {
		if len(c.Delivery.Wicket.Fielders) > 0 {
			return c.Delivery.Wicket.Fielders[0]
		}
		return c.Delivery.Bowler
	}
	return ""
}

func translateBatsmanOut(c *cricsheet.Event) (name string) {
	dType := mustDetermineEventType(&c.Delivery)
	if dType == "runOut" {
		return c.Delivery.Wicket.PlayerOut
	}
	return ""
}

func translateRuns(c *cricsheet.Event) (runs int) {
	dType := mustDetermineEventType(&c.Delivery)
	// Set the runs
	if dType == "legBye" || dType == "bye" {
		return c.Delivery.Runs.Extras
	}

	return c.Delivery.Runs.Batsman
}

// Translate event turns a cricsheet event in to a cricd delivery
func translateEvent(c cricsheet.Event) (cricd.Delivery, error) {
	cdd := cricd.NewDelivery(nil)
	allTeams, err := translateTeams(c.Info.Teams)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to get or create teams so exiting")
		return cricd.Delivery{}, err
	}

	// Get all the match info
	m := cricd.NewMatch(nil)
	m.HomeTeam = allTeams[0]
	m.AwayTeam = allTeams[1]
	m.LimitedOvers = c.Info.Overs
	m.NumberOfInnings = translateNumberOfInnings(&c)
	m.StartDate, err = translateStartDate(&c)
	if err != nil {
		log.Infof("Failed to work out startDate, setting it to epoch")
	}

	// Get Match
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

	// Set timestamp
	cdd.Timestamp = m.StartDate.Format(cricd.DateFormat)

	// Assign teams
	if c.BattingTeam == allTeams[0].Name {
		cdd.Ball.BattingTeam = allTeams[0]
		cdd.Ball.FieldingTeam = allTeams[1]
	} else {
		cdd.Ball.BattingTeam = allTeams[1]
		cdd.Ball.FieldingTeam = allTeams[0]
	}

	// Set the over, ball and innings info
	cdd.Ball.Innings = c.InningsNumber
	cdd.Ball.Over = c.Delivery.Over
	cdd.Ball.Ball = c.Delivery.Ball

	// Set all the players

	// Get the striker
	striker := cricd.NewPlayer(nil)
	striker.Name = c.Delivery.Batsman
	ok, err = striker.GetOrCreatePlayer(cdd.Ball.BattingTeam.ID)
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
	nonStriker := cricd.NewPlayer(nil)
	nonStriker.Name = c.Delivery.NonStriker
	ok, err = nonStriker.GetOrCreatePlayer(cdd.Ball.BattingTeam.ID)
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
	bowler := cricd.NewPlayer(nil)
	bowler.Name = c.Delivery.Bowler
	ok, err = bowler.GetOrCreatePlayer(cdd.Ball.FieldingTeam.ID)
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
	cdd.Runs = translateRuns(&c)

	// Translate Fielder
	fielder := cricd.NewPlayer(nil)
	fielderName := translateFielder(&c)
	if fielderName != "" {
		fielder.Name = fielderName
		// Get the fielder now
		ok, err = fielder.GetOrCreatePlayer(cdd.Ball.FieldingTeam.ID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Failed to get or create fielder")
			return cricd.Delivery{}, nil
		}
		if !ok {
			log.Error("Failed to get or create fielder without error")
			return cricd.Delivery{}, nil
		}
		if fielder.ID != 0 {
			cdd.Fielder = &fielder
		}
	}

	// Get dismissed batsman
	dismissed := cricd.NewPlayer(nil)
	dismissedName := translateBatsmanOut(&c)
	if dismissedName != "" {
		dismissed.Name = dismissedName
		ok, err = dismissed.GetOrCreatePlayer(cdd.Ball.BattingTeam.ID)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Error("Failed to get or create dismissed player")
			return cricd.Delivery{}, nil
		}
		if !ok {
			log.Error("Failed to get or create dismissed player without error")
			return cricd.Delivery{}, nil
		}
		if dismissed.ID != 0 {
			cdd.Batsman = &dismissed
		}
	}
	return cdd, nil
}

/*
//TODO: Tests
func processAll(cses []cricsheet.Event) <-chan cricd.Delivery {
	out := make(chan cricd.Delivery, 50)
	go func() {
		for _, event := range cses {
			e, err := process(event)
			if err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Failed to process event")
			}
			out <- e
		}
		close(out)
	}()
	return out
}

//TODO: test
func pushAll(in <-chan cricd.Delivery) {
	go func() {
		for e := range in {
			ok, err := e.Push()
			if err != nil {
				log.WithFields(log.Fields{"error": err}).Error("Failed to push event to Event API")
			}
			if !ok {
				log.Error("Failed to push event to Event API without error")
			}
		}
	}()
	return
} */

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
	log.Infof("Setting gamepath to %s", gamePath)

	tick := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-tick.C:
			log.Info("Did not find any files needed for processing, will try again 5s")
			files, err := ioutil.ReadDir(gamePath)
			if err != nil {
				log.WithFields(log.Fields{"error": err}).Fatal("Failed to read files in game path")
				break
			}
			for _, file := range files {
				gameFile := filepath.Join(gamePath, file.Name())
				// Check if it's a YAML file
				if strings.HasSuffix(file.Name(), ".yaml") {
					log.WithFields(log.Fields{"fileName": file.Name()}).Info("Found file with .YAML suffix to process")
					// 2. Parse the file in to a cricsheet.Game that holds all the events
					game := cricsheet.Game{}
					err = game.Read(gameFile)
					if err != nil {
						log.WithFields(log.Fields{"error": err}).Fatal("Failed to get delivery information from file so stopping processing")
						break
					}
					log.Info("Flattening game")
					cses, err := game.Flatten()
					if err != nil {
						log.WithFields(log.Fields{"error": err}).Error("Failed to get over and ball for delivery stopping processing")
						break
					}
					log.Info("Translating events")
					for k, event := range cses {
						log.Infof("Translating event %d/%d", k, len(cses)+1)
						e, err := translateEvent(event)
						if err != nil {
							log.WithFields(log.Fields{"error": err}).Error("Failed to translate event so not processing match")
							break
						}
						if (cricd.Delivery{}) == e {
							log.Error("Failed to translate event so not processing match")
							break
						}
						ok, err := e.Push()
						if err != nil {
							log.WithFields(log.Fields{"error": err}).Error("Failed to push event to Event API")
							break
						}
						if !ok {
							log.Error("Failed to push event to Event API without error")
							break
						}
					}
					// Rename the file
					log.WithFields(log.Fields{"fileName": file.Name()}).Info("Removing file")
					err = os.Remove(gameFile)
					if err != nil {
						log.WithFields(log.Fields{"error": err}).Error("Failed to remove file")
					}
					log.Infof("Successfully processed file %s", file.Name())
				}
			}
		}
	}
}
