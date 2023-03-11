package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
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

// server -> client events
const (
	// EventNewMember is sent when a new member joins the game
	EventNewMember = "new_member"
	// EventRemoveMember is sent when a member leaves the game
	EventRemoveMember = "remove_member"
	// EventStartGame is sent when the game is started by the owner, to all the players
	EventStartGame = "start_game"
	// EventNewProblem is sent when a new problem is generated for the user
	EventNewProblem = "new_problem"
	// EventNewScoreUpdate is sent when a user answers a problem correctly
	EventNewScoreUpdate = "new_score_update"
	// EventWrongAnswer is sent when a user answers a problem incorrectly
	EventWrongAnswer = "wrong_answer"
	// EventEndGame is sent when the game is over
	EventEndGame = "end_game"
)

// client -> server events
const (
	// EventStartGameOwner is sent when the game is started by the owner, by the owner
	EventStartGameOwner = "start_game_owner"
	// EventRequestProblem is sent when a user requests a new problem
	EventRequestProblem = "request_problem"
	// EventGiveAnswer is sent when a user answers a problem
	EventGiveAnswer = "give_answer"
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

type ProblemsObject struct {
	Problems []Problem `json:"problems"`
}

// AnswerEvent is passed in when the game is started by the owner
type RequestStartGameEvent struct {
	Duration                int            `json:"durationTime"`
	OrderIsRandom           bool           `json:"randomOrder"`
	UseCustomProblems       bool           `json:"useCustomProblems"`
	CustomProblems          ProblemsObject `json:"customProblems"`
	ExclusiveCustomProblems bool           `json:"exclusiveCustomProbems"`
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

func endGame(c *Client, message string) error {
	var broadMessage = EndGameEvent{message}

	data, err := json.Marshal(broadMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast message: %v", err)
	}

	var outgoingEvent = Event{EventEndGame, data}
	c.egress <- outgoingEvent
	return nil
}

func endGameLobby(l *Lobby, message string) error {
	var broadMessage = EndGameEvent{message}

	data, err := json.Marshal(broadMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast message: %v", err)
	}

	var outgoingEvent = Event{EventEndGame, data}
	for client := range l.clients {
		client.egress <- outgoingEvent
	}
	return nil
}

// EventStartGame is sent when the game is started by the owner
func StartGameHandler(event Event, c *Client) error {
	// var reqevent RequestStartGameEvent
	// if err := json.Unmarshal(event.Payload, &reqevent); err != nil {
	// 	return fmt.Errorf("bad payload in request: %v", err)
	// }
	if *c.lobby.owner != c.name {
		return fmt.Errorf("only the owner can start the game")
	} else if c.lobby.inPlay() {
		return fmt.Errorf("game is already in progress")
	}
	var chatevent RequestStartGameEvent
	if err := json.Unmarshal(event.Payload, &chatevent); err != nil {
		return fmt.Errorf("bad payload in request: %v", err)
	}
	log.Println(chatevent)
	c.lobby.timeLimit = chatevent.Duration
	var randomOrder = chatevent.OrderIsRandom
	var UseCustomProblems = chatevent.UseCustomProblems
	var customProblems = chatevent.CustomProblems
	var exclusiveCustomProblems = chatevent.ExclusiveCustomProblems
	if UseCustomProblems {
		newProblems := customProblems.Problems
		log.Println(GetProblems().Problems, randomOrder, UseCustomProblems, customProblems, exclusiveCustomProblems)
		if !exclusiveCustomProblems {
			localProblems := GetProblems()

			newProblems = append(newProblems, localProblems.Problems...)
		}
		c.lobby.CustomProblems = newProblems
		booleanArray := make([]bool, len(c.lobby.CustomProblems))
		c.lobby.CustomOrder = make([]int, len(c.lobby.CustomProblems))
		for i := 0; i < len(c.lobby.CustomProblems); i++ {
			// Generate x as a random value between 0 and the length of the problems array
			// and as long as the randomly chosen problem isn't already selected
			x := rand.Intn(len(booleanArray))
			for booleanArray[x] {
				x = rand.Intn(len(booleanArray))
			}
			if randomOrder {
				c.lobby.CustomOrder[i] = x
			} else {
				c.lobby.CustomOrder[i] = i
			}
			booleanArray[x] = true
		}
	} else {
		if !randomOrder {

			for i := 0; i < len(c.lobby.Problems); i++ {

				c.lobby.Problems[i] = i
			}

		}
	}
	var broadMessage = StartGameEvent{time.Now().Add(TIME_TO_START_GAME), c.lobby.timeLimit}

	if !DEBUG {
		time.Sleep(TIME_TO_START_GAME)
	}

	data, err := json.Marshal(broadMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast message: %v", err)
	}

	c.lobby.startGame()

	// Send start game message
	var outgoingEvent = Event{EventStartGame, data}
	for client := range c.lobby.clients {
		client.egress <- outgoingEvent
	}

	// Send the first problem (all users get the same problem & their question number starts off at 0)

	var newProblemBroadcast = NewProblemEvent{GetProblems().Problems[c.lobby.Problems[0]]}
	if UseCustomProblems {
		newProblemBroadcast = NewProblemEvent{c.lobby.CustomProblems[c.lobby.CustomOrder[0]]}
	}

	data, err = json.Marshal(newProblemBroadcast)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast message: %v", err)
	}

	outgoingEvent = Event{EventNewProblem, data}
	for client := range c.lobby.clients {
		client.egress <- outgoingEvent
	}

	// End the game after the duration of the game
	time.AfterFunc(time.Duration(c.lobby.timeLimit)*time.Second, func() {
		c.lobby.endGame()

		endGameLobby(c.lobby, "Game over!")

		// We can delete the lobby from the map now and have that be GC'd later
		// TODO: worry about other deletions?
		delete(c.manager.lobbies, c.lobby.id)
	})

	return nil
}

// EventGiveAnswer is sent when a user answers a problem
func GiveAnswerHandler(event Event, c *Client) error {
	if !c.lobby.inPlay() {
		return fmt.Errorf("game is not in progress")
	}
	var chatevent AnswerEvent
	if err := json.Unmarshal(event.Payload, &chatevent); err != nil {
		return fmt.Errorf("bad payload in request: %v", err)
	}
	user := c.lobby.userMapping[c.name]
	problem := GetProblems().Problems[c.lobby.Problems[user.questionNumber]]
	if !problem.CheckAnswer(chatevent.Answer) {
		c.egress <- Event{EventWrongAnswer, nil}
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

	if user.questionNumber == len(c.lobby.Problems) || (c.lobby.CustomProblems != nil && user.questionNumber == len(c.lobby.CustomProblems)) {
		endGame(c, "Ran out of problems!")
	} else {
		// Send client new problem
		var newProblemBroadcast = NewProblemEvent{GetProblems().Problems[user.questionNumber]}
		if c.lobby.CustomProblems != nil {
			newProblemBroadcast = NewProblemEvent{c.lobby.CustomProblems[user.questionNumber]}
		}
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
	if !c.lobby.inPlay() {
		return fmt.Errorf("game is not in progress")
	}
	user := c.lobby.userMapping[c.name]
	user = User{password: user.password, questionNumber: user.questionNumber + 1, score: user.score}

	c.lobby.userMapping[c.name] = user

	if user.questionNumber == len(c.lobby.Problems) || (c.lobby.CustomProblems != nil && user.questionNumber == len(c.lobby.CustomProblems)) {
		endGame(c, "Ran out of questions!")
		return nil
	}

	var newProblemBroadcast = NewProblemEvent{GetProblems().Problems[c.lobby.Problems[user.questionNumber]]}
	if c.lobby.CustomProblems != nil {
		newProblemBroadcast = NewProblemEvent{c.lobby.CustomProblems[user.questionNumber]}
	}
	data, err := json.Marshal(newProblemBroadcast)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast message: %v", err)
	}

	var outgoingEvent = Event{EventNewProblem, data}
	c.egress <- outgoingEvent

	return nil
}
