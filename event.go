package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"path/filepath"
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

// AnswerEvent is passed in when the game is started by the owner
type RequestStartGameEvent struct {
	Duration          int      `json:"durationTime"`
	OrderIsRandom     bool     `json:"randomOrder"`
	UseCustomProblems bool     `json:"useCustomProblems"`
	CustomProblems    Problems `json:"customProblems"`
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

func (l *Lobby) getLobbyProblems() []Problem {
	if l.useCustom {
		return l.CustomProblems
	} else {
		return GetProblems().Problems
	}
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

// @dev Requires that the lobby is in the Finished state
func (l *Lobby) saveEndedGame() {
	if l.gameState != Finished {
		return
	}

	type Player struct {
		Name  string `json:"name"`
		Score int    `json:"score"`
	}
	type SavedGameResult struct {
		Name           string    `json:"name"`
		Players        []Player  `json:"players"`
		StartTimestamp time.Time `json:"startTimestamp"`
		GameDuration   int       `json:"gameDuration"`
	}

	var savedGameRes = SavedGameResult{l.name, make([]Player, 0, len(l.userMapping)), *l.startTime, l.timeLimit}
	for name, user := range l.userMapping {
		savedGameRes.Players = append(savedGameRes.Players, Player{name, user.score})
	}

	data, err := json.Marshal(savedGameRes)
	if err != nil {
		fmt.Println("Failed to save game {} to JSON", l.id)
		return
	}

	logsPath := filepath.Join(".", "logs")
	err = os.MkdirAll(logsPath, os.ModePerm)
	if err != nil {
		fmt.Println("Failed to create logs directory")
		return
	}

	err = ioutil.WriteFile(filepath.Join(logsPath, l.id+".result.json"), data, 0644)
	if err != nil {
		fmt.Printf("Failed to save game %s to disk\n", l.id)
		return
	}

	fmt.Printf("Saved game %s to disk\n", l.id)
}

// EventStartGame is sent when the game is started by the owner
func StartGameHandler(event Event, c *Client) error {
	lobby := c.lobby

	// var reqevent RequestStartGameEvent
	// if err := json.Unmarshal(event.Payload, &reqevent); err != nil {
	// 	return fmt.Errorf("bad payload in request: %v", err)
	// }
	if *lobby.owner != c.name {
		return fmt.Errorf("only the owner can start the game")
	} else if lobby.inPlay() {
		return fmt.Errorf("game is already in progress")
	}
	var chatevent RequestStartGameEvent
	if err := json.Unmarshal(event.Payload, &chatevent); err != nil {
		return fmt.Errorf("bad payload in request: %v", err)
	}

	lobby.timeLimit = chatevent.Duration

	var randomOrder = chatevent.OrderIsRandom
	var useCustomProblems = chatevent.UseCustomProblems
	var customProblems = chatevent.CustomProblems

	if useCustomProblems {
		lobby.useCustom = true
		lobby.CustomProblems = customProblems.Problems
	}
	lobbyProblems := lobby.getLobbyProblems()
	lobby.CustomOrder = make([]int, len(lobbyProblems))

	if randomOrder {
		booleanArray := make([]bool, len(lobbyProblems))
		for i := 0; i < len(lobbyProblems); i++ {
			x := rand.Intn(len(booleanArray))
			for booleanArray[x] {
				x = rand.Intn(len(booleanArray))
			}
			lobby.CustomOrder[i] = x
			booleanArray[x] = true
		}
	} else {
		for i := 0; i < len(lobbyProblems); i++ {
			lobby.CustomOrder[i] = i
		}
	}

	startTime := time.Now().Add(TIME_TO_START_GAME)
	lobby.startTime = &startTime

	var broadMessage = StartGameEvent{startTime, lobby.timeLimit}

	if !DEBUG {
		time.Sleep(TIME_TO_START_GAME)
	}

	data, err := json.Marshal(broadMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast message: %v", err)
	}

	lobby.startGame()

	// Send start game message
	var outgoingEvent = Event{EventStartGame, data}
	for client := range lobby.clients {
		client.egress <- outgoingEvent
	}

	// Send the first problem (all users get the same problem & their question number starts off at 0)

	var newProblemBroadcast = c.getNewProblem()

	data, err = json.Marshal(newProblemBroadcast)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast message: %v", err)
	}

	outgoingEvent = Event{EventNewProblem, data}
	for client := range lobby.clients {
		client.egress <- outgoingEvent
	}

	// End the game after the duration of the game
	time.AfterFunc(time.Duration(lobby.timeLimit)*time.Second, func() {
		lobby.endGame()

		endGameLobby(lobby, "Game over!")
		for client := range lobby.clients {
			lobby.removeClient(client)
		}

		lobby.saveEndedGame()
		// We can delete the lobby from the map now and have that be GC'd later
		delete(c.manager.lobbies, lobby.id)
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
	problem := c.lobby.getLobbyProblems()[c.lobby.CustomOrder[user.questionNumber]]

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

	if user.questionNumber == len(c.lobby.getLobbyProblems()) {
		endGame(c, "Ran out of problems!")
	} else {
		c.sendClientProblem()
	}

	return nil
}

func (client *Client) getNewProblem() NewProblemEvent {
	lobby := client.lobby
	user := lobby.userMapping[client.name]

	newProblemBroadcast := NewProblemEvent{lobby.getLobbyProblems()[lobby.CustomOrder[user.questionNumber]]}

	return newProblemBroadcast
}

// @dev Pre-condition: client hasn't run out of problems
func (client *Client) sendClientProblem() error {
	newProblemBroadcast := client.getNewProblem()

	data, err := json.Marshal(newProblemBroadcast)
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast message: %v", err)
	}

	var outgoingEvent = Event{EventNewProblem, data}
	client.egress <- outgoingEvent

	return nil
}

func RequestProblemHandler(event Event, c *Client) error {
	if !c.lobby.inPlay() {
		return fmt.Errorf("game is not in progress")
	}
	user := c.lobby.userMapping[c.name]
	user = User{password: user.password, questionNumber: user.questionNumber + 1, score: user.score}

	c.lobby.userMapping[c.name] = user

	if user.questionNumber == len(c.lobby.getLobbyProblems()) {
		endGame(c, "Ran out of questions!")
		return nil
	}

	c.sendClientProblem()
	return nil
}
