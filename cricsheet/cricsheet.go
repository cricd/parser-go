package cricsheet

import (
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

// Wicket describes a dismissal
type Wicket struct {
	Kind      string   `yaml:"kind"`
	PlayerOut string   `yaml:"player_out"`
	Fielders  []string `yaml:"fielders"`
}

// Delivery describes a cricket delivery
type Delivery struct {
	Over       int
	Ball       int
	Batsman    string `yaml:"batsman"`
	NonStriker string `yaml:"non_striker"`
	Bowler     string `yaml:"bowler"`
	Runs       struct {
		Batsman int `yaml:"batsman"`
		Extras  int `yaml:"extras"`
		Total   int `yaml:"total"`
	} `yaml:"runs"`
	Wicket Wicket `yaml:"wicket"`
	Extras map[string]int
}

// Inning describes an individual innings in cricket
type Inning struct {
	Team       string                `yaml:"team"`
	Deliveries []map[string]Delivery `yaml:"deliveries"`
}

// Teams describes the names of teams playing a game of cricket
type Teams []string

// GameInfo describes the meta-data about the game of cricket
type GameInfo struct {
	City      string   `yaml:"city"`
	Dates     []string `yaml:"dates"`
	Gender    string   `yaml:"gender"`
	MatchType string   `yaml:"match_type"`
	Outcome   struct {
		Winner string `yaml:"winner"`
	} `yaml:"outcome"`
	Overs int   `yaml:"overs"`
	Teams Teams `yaml:"teams"` // First team will always be home team
	Toss  struct {
		Decision string `yaml:"decision"`
		Winner   string `yaml:"winner"`
	} `yaml:"toss"`
	Umpires []string `yaml:"umpires"`
	Venue   string   `yaml:"venue"`
}

// Game holds all data related to a game of cricket
type Game struct {
	Meta struct {
		Created     string  `yaml:"created"`
		DataVersion float64 `yaml:"data_version"`
		Revision    int     `yaml:"revision"`
	} `yaml:"meta"`
	Info    GameInfo
	Innings []map[string]Inning
}

// Event is a flattened structure that represents one delivery and associated data
type Event struct {
	Info          GameInfo
	InningsNumber int
	BattingTeam   string
	Delivery      Delivery
}

// Read takes a []byte of YAML and tries to parse it in to a Game struct
func (g *Game) Read(gameFile string) error {
	gf, err := ioutil.ReadFile(gameFile)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Failed to read YAML file")
		return err
	}
	err = yaml.Unmarshal(gf, &g)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to unmarshall game from file")
		return err
	}
	return nil
}

// Flatten takes a game and flattens it in to a slice of Events
func (g *Game) Flatten() (events []Event, err error) {
	var cses []Event
	for k, inn := range g.Innings {
		for _, dvs := range inn {
			for _, dv := range dvs.Deliveries {
				for dk, d := range dv {
					var cse Event
					cse.Info = g.Info
					cse.BattingTeam = dvs.Team
					cse.InningsNumber = k + 1 // 1 index array
					cse.Delivery = d

					// Pull the over and ball from the key
					overBall := strings.SplitN(dk, ".", 2)
					cse.Delivery.Over, err = strconv.Atoi(overBall[0])
					cse.Delivery.Ball, err = strconv.Atoi(overBall[1])
					if err != nil {
						log.WithFields(log.Fields{"error": err}).Error("Failed to get over and ball for delivery")
						return nil, err
					}
					cses = append(cses, cse)
				}
			}
		}
	}
	return cses, nil
}

func init() {
	// Set up logging
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)
}
