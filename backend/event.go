package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const DEBUG = true

// EventHandler is a function signature that is used to affect messages on the socket,
// and triggered depending on the type
type EventHandler func(event ClientSent, c *Client) error

const TIME_TO_START_GAME = 0 * time.Second

var (
	problems []*Problem
)

// Singleton to get the problems, s.t. problems are only loaded once (upon program instantiation)
func GetProblems() []*Problem {
	if problems == nil {
		type PProblem struct {
			Latex       string `json:"latex"`
			Description string `json:"description"`
			Title       string `json:"title"`
		}
		type JasonBoy struct {
			Problems []PProblem `json:"problems"`
		}
		var jason JasonBoy

		jsonFile, err := os.Open("problems.json")

		if err != nil {
			log.Println(err)
			return nil
		}
		defer jsonFile.Close()
		byteValue, _ := ioutil.ReadAll(jsonFile)

		// We unmarshal our byteArray which contains our
		// jsonFile's content into 'problems' which we defined above
		json.Unmarshal(byteValue, &jason)
		problems = make([]*Problem, len(jason.Problems))
		for i := 0; i < len(problems); i++ {
			problems[i] = &Problem{
				Latex:       &jason.Problems[i].Latex,
				Description: &jason.Problems[i].Description,
				Title:       &jason.Problems[i].Title,
			}
		}

	}

	return problems
}

func (l *Lobby) getLobbyProblems() []*Problem {
	if l.useCustom {
		return l.CustomProblems
	} else {
		return GetProblems()
	}
}

func protofy(x isServerSent_Message) []byte {
	log.Println(x)
	data, err := proto.Marshal(&ServerSent{Message: x})
	if err != nil {
		log.Println("oh no ", err)
	}
	return data
}

func endGame(c *Client, message string) error {
	var outgoingEvent = &ServerSent_End{&ServerSent_EndGame{}}
	c.egress <- protofy(outgoingEvent)
	return nil
}

func endGameLobby(l *Lobby, message string) error {
	var outgoingEvent = &ServerSent_End{&ServerSent_EndGame{}}
	for client := range l.clients {
		client.egress <- protofy(outgoingEvent)
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
		Score int32  `json:"score"`
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
		log.Println("Failed to save game {} to JSON", l.id)
		return
	}

	logsPath := filepath.Join(".", "logs")
	err = os.MkdirAll(logsPath, os.ModePerm)
	if err != nil {
		log.Println("Failed to create logs directory")
		return
	}

	err = ioutil.WriteFile(filepath.Join(logsPath, l.id+".result.json"), data, 0644)
	if err != nil {
		log.Printf("Failed to save game %s to disk\n", l.id)
		return
	}

	log.Printf("Saved game %s to disk\n", l.id)
}

// EventStartGame is sent when the game is started by the owner
func StartGameHandler(event *ClientSent_RequestStart, c *Client) error {
	lobby := c.lobby

	// var reqevent RequestStartGameEvent
	// if err := json.Unmarshal(event.Payload, &reqevent); err != nil {
	// 	return log.Errorf("bad payload in request: %v", err)
	// }
	if *lobby.owner != c.name {
		return fmt.Errorf("only the owner can start the game")
	} else if lobby.inPlay() {
		return fmt.Errorf("game is already in progress")
	}

	lobby.timeLimit = int(event.Duration.Seconds)

	if len(event.Problems) > 0 {
		lobby.useCustom = true
		lobby.CustomProblems = event.Problems
	}
	lobbyProblems := lobby.getLobbyProblems()

	lobby.CustomOrder = make([]int, len(lobbyProblems))

	if *event.IsRandom {
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

	if !DEBUG {
		time.Sleep(TIME_TO_START_GAME)
	}

	lobby.startGame()

	// Send start game message
	var outgoingEvent = ServerSent{Message: &ServerSent_Start{
		&ServerSent_StartGame{StartTime: timestamppb.New(startTime), Duration: &timestamppb.Timestamp{Seconds: int64(lobby.timeLimit)}}},
	}
	data, _ := proto.Marshal(&outgoingEvent)
	for client := range lobby.clients {
		client.egress <- data
	}

	// Send the first problem (all users get the same problem & their question number starts off at 0)

	var newProblemBroadcast = c.getNewProblem()
	// log.Println(newProblemBroadcast)

	for client := range lobby.clients {
		client.egress <- protofy(&newProblemBroadcast)
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
func GiveAnswerHandler(event *ClientSent_GiveAnswer, c *Client) error {
	if !c.lobby.inPlay() {
		return fmt.Errorf("game is not in progress")
	}
	user := c.lobby.userMapping[c.name]
	problem := c.lobby.getLobbyProblems()[c.lobby.CustomOrder[user.questionNumber]]

	if !problem.CheckAnswer(event.GetAnswer()) {
		c.egress <- protofy(&ServerSent_Wrong{})
		return fmt.Errorf("bad payload in request")
	}

	// gainedPoints = ⌈latexSolutionLength / 10⌉
	gainedPoints := int32(math.Ceil(float64(len(*problem.Latex)) / float64(10)))
	c.lobby.userMapping[c.name] = User{
		password: user.password, questionNumber: user.questionNumber + 1, score: user.score + gainedPoints,
	}
	user = c.lobby.userMapping[c.name]

	var clientsScoreUpdateEvent = &ServerSent_ScoreUpdate_{ScoreUpdate: &ServerSent_ScoreUpdate{Name: &c.name, Score: &user.score}}

	for client := range c.lobby.clients {
		client.egress <- protofy(clientsScoreUpdateEvent)
	}

	if user.questionNumber == int32(len(c.lobby.getLobbyProblems())) {
		endGame(c, "Ran out of problems!")
	} else {
		c.sendClientProblem()
	}

	return nil
}

func (client *Client) getNewProblem() ServerSent_NewProblem_ {
	lobby := client.lobby
	user := lobby.userMapping[client.name]

	newProblemBroadcast := ServerSent_NewProblem_{
		NewProblem: &ServerSent_NewProblem{Problem: lobby.getLobbyProblems()[lobby.CustomOrder[user.questionNumber]]}}

	return newProblemBroadcast
}

// @dev Pre-condition: client hasn't run out of problems
func (client *Client) sendClientProblem() error {
	newProblemBroadcast := client.getNewProblem()
	client.egress <- protofy(&newProblemBroadcast)

	return nil
}

func RequestProblemHandler(event *ClientSent_RequestProblem, c *Client) error {
	if !c.lobby.inPlay() {
		return fmt.Errorf("game is not in progress")
	}
	user := c.lobby.userMapping[c.name]
	user = User{password: user.password, questionNumber: user.questionNumber + 1, score: user.score}

	c.lobby.userMapping[c.name] = user

	if user.questionNumber == int32(len(c.lobby.getLobbyProblems())) {
		endGame(c, "Ran out of questions!")
		return nil
	}

	c.sendClientProblem()
	return nil
}
