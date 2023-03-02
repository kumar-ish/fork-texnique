package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"time"
)

const DEBUG = true

// Event is the struct for messages sent over the websocket
// Type used to differ between different actions
type Event struct {
	// Type is the message type sent
	Type string `json:"type"`
	// Payload is the data Based on the Type
	Payload json.RawMessage `json:"payload"`
}

// EventHandler is a function signature that is used to affect messages on the socket,
// and triggered depending on the type
type EventHandler func(event Event, c *Client) error

const (
	// EventNewMember is sent when a new member joins the game
	// server -> client
	EventNewMember = "new_member"
	// EventRemoveMember is sent when a member leaves the game
	// server -> client
	EventRemoveMember = "remove_member"
	// EventStartGameOwner is sent when the game is started by the owner, by the owner
	// client -> server
	EventStartGameOwner = "start_game_owner"
	// EventStartGame is sent when the game is started by the owner, to all the players
	// server -> client
	EventStartGame = "start_game"
	// EventNewProblem is sent when a new problem is generated
	// server -> client
	EventNewProblem = "new_problem"
	// EventRequestProblem is sent when a user requests a new problem
	// client -> server
	EventRequestProblem = "request_problem"
	// EventGiveAnswer is sent when a user answers a problem
	// client -> server
	EventGiveAnswer = "give_answer"
	// EventNewScoreUpdate is sent when a user answers a problem
	// server -> client
	EventNewScoreUpdate = "new_score_update"
	// EventEndGame is sent when the game is over
	// server -> client
	EventEndGame = "end_game"
)

const TIME_TO_START_GAME = 10 * time.Second

// NewMemberEvent is returned when a new member joins the game
type NewMemberEvent struct {
	Name string `json:"name"`
}

// RemoveMemberEvent is returned when a member leaves the game
type RemoveMemberEvent struct {
	Name string `json:"name"`
}

// StartGameEvent is returned when the game is started by the owner
type StartGameEvent struct {
	StartTimestamp time.Time `json:"startTimestamp"`
	Duration       int       `json:"duration"`
}

// NewProblemEvent is returned when a new problem is generated
type NewProblemEvent struct {
	Problem Problem `json:"problem"`
}

// AnswerEvent is returned when a user answers a problem
type AnswerEvent struct {
	Answer string `json:"answer"`
}

// NewScoreUpdateEvent is returned when a user answers a problem
type NewScoreUpdateEvent struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

// EndGameEvent is returned when the game is over
type EndGameEvent struct {
	Message string `json:"message"`
}

var (
	problems *Problems
)

// Singleton to get the problems, s.t. problems are only loaded once (upon program instantiation)
func GetProblems() *Problems {
	if problems == nil {
		jsonFile, err := os.Open("problems.json")

		if err != nil {
			fmt.Println(err)
			return nil
		}
		defer jsonFile.Close()
		byteValue, _ := ioutil.ReadAll(jsonFile)

		// We unmarshal our byteArray which contains our
		// jsonFile's content into 'problems' which we defined above
		json.Unmarshal(byteValue, &problems)
	}

	return problems
}

func endGame(c *Client, message string) {
	var broadMessage = EndGameEvent{message}

	data, err := json.Marshal(broadMessage)
	if err != nil {
		fmt.Errorf("failed to marshal broadcast message: %v", err)
	}

	var outgoingEvent = Event{EventEndGame, data}
	for client := range c.lobby.clients {
		client.egress <- outgoingEvent
	}
}

// EventStartGame is sent when the game is started by the owner
func StartGameHandler(event Event, c *Client) error {
	if *c.lobby.owner != c.name {
		return fmt.Errorf("only the owner can start the game")
	}

	var broadMessage = StartGameEvent{time.Now().Add(TIME_TO_START_GAME), c.lobby.timeLimit}

	if !DEBUG {
		time.Sleep(TIME_TO_START_GAME)
	}

	data, err := json.Marshal(broadMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast message: %v", err)
	}

	// Send start game message
	var outgoingEvent = Event{EventStartGame, data}
	for client := range c.lobby.clients {
		client.egress <- outgoingEvent
	}

	// Send the first problem
	var newProblemBroadcast = NewProblemEvent{(*GetProblems()).Problems[0]}

	data, err = json.Marshal(newProblemBroadcast)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast message: %v", err)
	}

	outgoingEvent = Event{EventNewProblem, data}
	for client := range c.lobby.clients {
		client.egress <- outgoingEvent
	}

	// End the game after the duration of the game
	time.AfterFunc(time.Duration(c.lobby.timeLimit)*time.Second, func() { endGame(c, "Game Over!") })

	return nil
}

// EventGiveAnswer is sent when a user answers a problem
func GiveAnswerHandler(event Event, c *Client) error {
	var chatevent AnswerEvent
	if err := json.Unmarshal(event.Payload, &chatevent); err != nil {
		return fmt.Errorf("bad payload in request: %v", err)
	}
	user := c.lobby.userMapping[c.name]
	problem := (*GetProblems()).Problems[user.questionNumber]
	if !problem.CheckAnswer(chatevent.Answer) {
		c.egress <- Event{"wrong_answer", nil}
		return fmt.Errorf("bad payload in request")
	}

	// gainedPoints = ⌈latexSolutionLength / 10⌉
	gainedPoints := int(math.Ceil(float64(len(problem.Latex)) / float64(10)))
	c.lobby.userMapping[c.name] = User{
		password: user.password, questionNumber: user.questionNumber + 1, score: user.score + gainedPoints,
	}
	user = c.lobby.userMapping[c.name]

	var broadMessage = NewScoreUpdateEvent{c.name, user.score}

	data, err := json.Marshal(broadMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast message: %v", err)
	}

	var clientsScoreUpdateEvent = Event{EventNewScoreUpdate, data}

	for client := range c.lobby.clients {
		client.egress <- clientsScoreUpdateEvent
	}

	if user.questionNumber == len(c.lobby.Problems) {
		endGame(c, "Ran out of problems!")
	} else {
		// Send client new problem
		var newProblemBroadcast = NewProblemEvent{(*GetProblems()).Problems[user.questionNumber]}

		data, err := json.Marshal(newProblemBroadcast)
		if err != nil {
			return fmt.Errorf("failed to marshal broadcast message: %v", err)
		}

		var outgoingEvent = Event{EventNewProblem, data}
		c.egress <- outgoingEvent
	}

	return nil
}

func RequestProblemHandler(event Event, c *Client) error {
	user := c.lobby.userMapping[c.name]
	user = User{password: user.password, questionNumber: user.questionNumber + 1, score: user.score}

	c.lobby.userMapping[c.name] = user

	if user.questionNumber == len(c.lobby.Problems) {
		endGame(c, "Ran out of questions!")
		return nil
	}

	var newProblemBroadcast = NewProblemEvent{(*GetProblems()).Problems[user.questionNumber]}

	data, err := json.Marshal(newProblemBroadcast)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast message: %v", err)
	}

	var outgoingEvent = Event{EventNewProblem, data}
	c.egress <- outgoingEvent

	return nil
}
