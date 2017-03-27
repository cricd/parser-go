package cricd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	log "github.com/Sirupsen/logrus"
	cache "github.com/patrickmn/go-cache"
)

const DateFormat = "2006-01-02"

var c Config
var playerCache = cache.New(5*time.Minute, 30*time.Second)
var teamCache = cache.New(5*time.Minute, 30*time.Second)
var matchCache = cache.New(5*time.Minute, 30*time.Second)

type Team struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Player struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	DateOfBirth time.Time `json:"dateOfBirth"`
	Gender      string    `json:"gender"`
}

type Match struct {
	ID              int       `json:"id"`
	HomeTeam        Team      `json:"homeTeam"`
	AwayTeam        Team      `json:"awayTeam"`
	StartDate       time.Time `json:"startDate"`
	NumberOfInnings int       `json:"numberOfInnings"`
	LimitedOvers    int       `json:"limitedOvers"`
}

type Innings struct {
	Number       int
	BattingTeam  Team
	FieldingTeam Team
}

type Delivery struct {
	MatchID   int    `json:"match"`
	EventType string `json:"eventType"`
	Timestamp string `json:"timestamp"`
	Ball      struct {
		BattingTeam  Team `json:"battingTeam"`
		FieldingTeam Team `json:"fieldingTeam"`
		Innings      int  `json:"innings"`
		Over         int  `json:"over"`
		Ball         int  `json:"ball"`
	} `json:"ball"`
	Runs    int `json:"runs"`
	Batsmen struct {
		Striker    Player `json:"striker"`
		NonStriker Player `json:"nonStriker"`
	} `json:"batsmen"`
	Bowler  Player  `json:"bowler"`
	Fielder *Player `json:"fielder,omitempty"`
}

type Config struct {
	eventStoreIP    string
	eventStorePort  string
	entityStoreIP   string
	entityStorePort string
}

// TODO: Test me
func (p *Player) GetOrCreatePlayer() (ok bool, err error) {
	k, e := p.Get()

	if e != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to get player")
		return false, err
	}
	if !k {
		k, e := p.Create()
		if e != nil {
			log.WithFields(log.Fields{"error": err}).Error("Failed to create player")
			return false, err
		}
		if !k {
			log.Error("Failed to create player without error")
			return false, nil
		}
	}
	log.Debugf("Returning player with Name: %s ID#: %d", p.Name, p.ID)
	return true, nil
}

// TODO: Test me
func (p *Player) Create() (ok bool, err error) {
	etURL := fmt.Sprintf("http://%s:%s/players", c.entityStoreIP, c.entityStorePort)
	params := url.Values{
		"name": {p.Name},
	}

	log.Debugf("Sending request to create players to: %s", etURL)
	log.Debugf("Using the following params to create players: %s", params)
	res, err := http.PostForm(etURL, params)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to call create player endpoint")
		return false, err
	}
	if res.StatusCode != http.StatusCreated {
		log.WithFields(log.Fields{"response": res.Status, "code": res.StatusCode}).Error("Got not OK response from creating player")
		return false, fmt.Errorf("Got non OK status code when creating player: %d", res.StatusCode)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to read body from create player endpoint")
		return false, err
	}
	var cp Player
	err = json.Unmarshal(body, &cp)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to unmarshal JSON from create player endpoint")
		return false, err
	}
	// If we have a non-null Player
	if (Player{}) != cp {
		p.ID = cp.ID
		return true, nil
	}

	log.Error("Create player endpoint returned no players")
	return false, fmt.Errorf("Failed to return any players when creating a player")

}

// TODO: Test me
func (p *Player) Get() (ok bool, err error) {

	// Try hit the cache first
	player, found := playerCache.Get(p.Name)
	if found {
		log.Debugf("Returning player from the player cache: %d - %s", p.ID, p.Name)
		p.ID = player.(Player).ID
		return true, nil
	}

	etURL := fmt.Sprintf("http://%s:%s/players", c.entityStoreIP, c.entityStorePort)
	req, err := http.NewRequest("GET", etURL, nil)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to create request to get from players endpoint")
		return false, err
	}
	// Build query string
	q := req.URL.Query()
	q.Add("name", p.Name)
	req.URL.RawQuery = q.Encode()
	// Send request
	log.Debugf("Sending request to get player to: %s", req.URL)

	client := &http.Client{Timeout: 1 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to send request to get players endpoint")
		return false, err
	}
	if res.StatusCode != (http.StatusOK) {
		log.WithFields(log.Fields{"response": res.Status, "code": res.StatusCode}).Error("Got not OK response from getting players")
		return false, fmt.Errorf("Received a non OK status code when getting players, got: %d", res.StatusCode)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to read body from get player endpoint")
		return false, err
	}
	var t []Player
	err = json.Unmarshal(body, &t)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to unmarshal JSON from get player endpoint")
		return false, err
	}

	// More than one player we'll take the first
	if len(t) > 1 {
		log.WithFields(log.Fields{"players": len(t)}).Info("More than one team returned from get player endpoint, using the first")
		p.ID = t[0].ID
		log.Infof("Got player with ID#: %d", p.ID)
		playerCache.Set(t[0].Name, t[0], cache.DefaultExpiration)
		return true, nil
		// If there's exactly one, then we're good
	} else if len(t) == 1 {
		p.ID = t[0].ID
		log.Debugf("Returning team from get player endpoint, ID#: %d Name: %s", t[0].ID, t[0].Name)
		playerCache.Set(t[0].Name, t[0], cache.DefaultExpiration)
		return true, nil
	} else {
		// Otherwise we didn't get anything
		log.Info("Not returning any players from get player endpoint")
		return false, nil
	}

}

// TODO: Test me
func (t *Team) Create() (ok bool, err error) {
	etURL := fmt.Sprintf("http://%s:%s/teams", c.entityStoreIP, c.entityStorePort)
	params := url.Values{
		"name": {t.Name},
	}

	log.Debugf("Sending request to create team to: %s", etURL)
	log.Debugf("Using the following params to create team: %s", params)
	res, err := http.PostForm(etURL, params)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to call create team endpoint")
		return false, err
	}
	if res.StatusCode != http.StatusCreated {
		log.WithFields(log.Fields{"response": res.Status, "code": res.StatusCode}).Error("Got not OK response from creating team")
		return false, fmt.Errorf("Got non OK status code when creating team: %d", res.StatusCode)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to read body from create team endpoint")
		return false, err
	}
	var ct Team
	err = json.Unmarshal(body, &ct)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to unmarshal JSON from create team endpoint")
		return false, err
	}
	// If we got a legitimate team
	if ct.ID != 0 {
		t.ID = ct.ID
		log.Debugf("Returning team from create team endpoint, ID#: %d Name: %s", ct.ID, ct.Name)
		return true, nil
	}

	log.Errorf("Failed to create team from create team endpoint")
	return false, fmt.Errorf("Failed to create team from create team endpoint")

}

//TODO: Test me
func (t *Team) Get() (ok bool, err error) {
	// Try hit the cache first
	tm, found := teamCache.Get(t.Name)
	if found {
		log.Debugf("Returning team from the team cache: %s", t.Name)
		t.ID = tm.(Team).ID
		return true, nil
	}
	etURL := fmt.Sprintf("http://%s:%s/teams", c.entityStoreIP, c.entityStorePort)
	req, err := http.NewRequest("GET", etURL, nil)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to get team from team endpoint")
		return false, err
	}
	// Build query string
	q := req.URL.Query()
	q.Add("name", t.Name)
	req.URL.RawQuery = q.Encode()
	// Send request
	log.Debugf("Sending request to get team to: %s", req.URL)

	client := &http.Client{Timeout: 1 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to send request to get team endpoint")
		return false, err
	}
	if res.StatusCode != (http.StatusOK) {
		log.WithFields(log.Fields{"response": res.Status, "code": res.StatusCode}).Error("Got not OK response from getting teams")
		return false, fmt.Errorf("Received a non OK status code when getting teams, got: %d", res.StatusCode)
	}
	body, err := ioutil.ReadAll(res.Body)
	var ct []Team
	err = json.Unmarshal(body, &ct)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to unmarshal JSON from get team endpoint")
		return false, err
	}
	// If we got more than one team
	if len(ct) > 1 {
		log.WithFields(log.Fields{"teams": len(ct)}).Info("More than one team returned from  get team endpoint")
		t.ID = ct[0].ID
		teamCache.Set(ct[0].Name, ct[0], cache.DefaultExpiration)
		return true, nil
	} else if len(ct) == 1 {
		t.ID = ct[0].ID
		teamCache.Set(ct[0].Name, ct[0], cache.DefaultExpiration)
		log.Debugf("Returning team from get team endpoint, ID#: %d Name: %s", t.ID, t.Name)
		return true, nil
	} else {
		log.Info("Not returning any teams from get team endpoint")
		return false, nil
	}
}

// TODO: Test me
func (m *Match) Create() (ok bool, err error) {
	log.Debugf("Creating match between %s and %s", m.HomeTeam, m.AwayTeam)
	etURL := fmt.Sprintf("http://%s:%s/matches", c.entityStoreIP, c.entityStorePort)
	params := url.Values{
		"homeTeam":        {strconv.Itoa(m.HomeTeam.ID)},
		"awayTeam":        {strconv.Itoa(m.AwayTeam.ID)},
		"numberOfInnings": {strconv.Itoa(m.NumberOfInnings)},
		"limitedOvers":    {strconv.Itoa(m.LimitedOvers)},
		"startDate":       {m.StartDate.Format(DateFormat)},
	}

	log.Debugf("Sending request to create match to: %s", etURL)
	log.Debugf("Using the following params to create match: %s", params)
	res, err := http.PostForm(etURL, params)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to call create match endpoint")
		return false, err
	}
	if res.StatusCode != http.StatusCreated {
		log.WithFields(log.Fields{"response": res.Status, "code": res.StatusCode}).Error("Got not OK response from creating match")
		return false, fmt.Errorf("Got non OK status code when creating match: %d", res.StatusCode)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to read body from create match endpoint")
		return false, err
	}

	var matches Match
	err = json.Unmarshal(body, &matches)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to unmarshal JSON from create match endpoint")
		return false, err
	}
	// If we got a legitimate match
	if matches.ID != 0 {
		m.ID = matches.ID
		log.Debugf("Returning match from create match endpoint, ID#:", m.ID)
		return true, nil
	}
	log.Errorf("Not returning any matches from create match endpoint ")
	return false, nil

}

// TODO: Test me
func (m *Match) Get() (ok bool, err error) {

	// Try hit the cache first
	matchKey := base64.StdEncoding.EncodeToString([]byte(m.AwayTeam.Name + m.HomeTeam.Name + m.StartDate.Format(DateFormat)))
	match, found := matchCache.Get(matchKey)
	if found {
		log.Debugf("Returning match from the match cache: %s", matchKey)
		m.ID = match.(Match).ID
		return true, nil
	}

	etURL := fmt.Sprintf("http://%s:%s/matches", c.entityStoreIP, c.entityStorePort)
	req, err := http.NewRequest("GET", etURL, nil)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to create request to get from match endpoint")
	}
	// Build query string
	q := req.URL.Query()
	q.Add("homeTeam", strconv.Itoa(m.HomeTeam.ID))
	q.Add("awayTeam", strconv.Itoa(m.AwayTeam.ID))
	q.Add("numberOfInnings", strconv.Itoa(m.NumberOfInnings))
	q.Add("limitedOvers", strconv.Itoa(m.LimitedOvers))
	q.Add("startDate", m.StartDate.Format(DateFormat))
	req.URL.RawQuery = q.Encode()
	// Send request
	log.Debugf("Sending request to get match to: %s", req.URL)

	client := &http.Client{Timeout: 1 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to send request to get match endpoint")
		return false, err
	}
	if res.StatusCode != http.StatusOK {
		log.WithFields(log.Fields{"response": res.Status, "code": res.StatusCode}).Error("Got not OK response from getting match")
		return false, fmt.Errorf("Received a non OK status code when getting match, got: %d", res.StatusCode)
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to read body from get match endpoint")
		return false, err
	}

	var matches []Match
	err = json.Unmarshal(body, &matches)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to unmarshal JSON from get match endpoint")
		return false, err
	}

	if len(matches) > 1 {
		log.WithFields(log.Fields{"teams": len(matches)}).Info("More than one team returned from get match endpoint")
		m.ID = matches[0].ID
		matchCache.Set(matchKey, matches[0], cache.DefaultExpiration)
		return true, nil
	} else if len(matches) == 1 {
		m.ID = matches[0].ID
		matchCache.Set(matchKey, matches[0], cache.DefaultExpiration)
		log.Debugf("Returning match from get match endpoint, ID#: %d between %s and %s on: %s", matches[0].ID, matches[0].HomeTeam, matches[0].AwayTeam, matches[0].StartDate)
		return true, nil
	} else {
		log.Debugf("Not returning any matches from get match endpoint ")
		return false, nil
	}

}

// TODO: Test
func mustGetConfig(c *Config) {
	eaIP := os.Getenv("EVENTSTORE_IP")
	if eaIP != "" {
		c.eventStoreIP = eaIP
	} else {
		log.WithFields(log.Fields{"value": "EVENTSTORE_IP"}).Info("Unable to find env var, using default `localhost`")
		c.eventStoreIP = "localhost"
	}

	eaPort := os.Getenv("EVENTSTORE_PORT")
	if eaPort != "" {
		c.eventStorePort = eaPort
	} else {
		log.WithFields(log.Fields{"value": "EVENTSTORE_PORT"}).Info("Unable to find env var, using default `2113`")
		c.eventStorePort = "4567"

	}

	etURL := os.Getenv("ENTITYSTORE_IP")
	if etURL != "" {
		c.entityStoreIP = etURL
	} else {
		log.WithFields(log.Fields{"value": "ENTITYSTORE_IP"}).Info("Unable to find env var, using default `localhost`")
		c.entityStoreIP = "localhost"
	}

	etPort := os.Getenv("ENTITYSTORE_PORT")
	if etPort != "" {
		c.entityStorePort = etPort
	} else {
		log.WithFields(log.Fields{"value": "ENTITYSTORE_PORT"}).Info("Unable to find env var, using default `1338`")
		c.entityStorePort = "1338"
	}

}

func (d *Delivery) Push() (ok bool, err error) {
	etURL := fmt.Sprintf("http://%s:%s/event", c.eventStoreIP, c.eventStorePort)
	log.Debugf("Sending request to Event API at %s", etURL)
	json, err := json.Marshal(d)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to marshal delivery to json")
		return false, err
	}
	req, err := http.NewRequest("POST", etURL, bytes.NewBuffer(json))
	req.Header.Set("Content-Type", "application/json")
	params := url.Values{}
	params.Set("NextBall", "false")
	client := &http.Client{Timeout: 2 * time.Second}

	res, err := client.Do(req)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Error("Failed to send to event api")
		return false, err
	}
	if res.StatusCode != http.StatusCreated {
		log.WithFields(log.Fields{"response": res.Status, "code": res.StatusCode}).Error("Got not OK response from event API")
		return false, fmt.Errorf("Received a %d code - %s from event store api", res.StatusCode, res.Status)
	}
	defer res.Body.Close()
	return true, nil
}

func init() {
	mustGetConfig(&c)
}
