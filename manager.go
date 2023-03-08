package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var (
	/**
	websocketUpgrader is used to upgrade incomming HTTP requests into a persistent websocket connection
	*/
	websocketUpgrader = websocket.Upgrader{
		// Apply the Origin Checker
		CheckOrigin:     checkOrigin,
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

var (
	ErrEventNotSupported = errors.New("this event type is not supported")
)

var handlers = map[string]EventHandler{
	EventStartGameOwner: StartGameHandler,
	EventGiveAnswer:     GiveAnswerHandler,
	EventRequestProblem: RequestProblemHandler,
}

// checkOrigin will check origin and return true if its allowed
func checkOrigin(r *http.Request) bool {
	// Grab the request origin
	origin := r.Header.Get("Origin")

	switch origin {
	// (TODO: do we need to change this when deploying with https?)
	case "http://localhost:8080":
		return true
	default:
		return false
	}
}

type Problem struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Latex       string `json:"latex"`
}

func (p *Problem) CheckAnswer(submittedAnswer string) bool {
	return true // TODO: Implement this (check against answer)
}

type Problems struct {
	Problems []Problem `json:"problems"`
}

type User struct {
	password       string
	questionNumber int
	score          int
}

type GameState int64

const (
	WaitingForPlayers GameState = iota
	InPlay            GameState = iota
	Finished          GameState = iota
)

type Lobby struct {
	id        string
	name      string
	timeLimit int
	startTime *time.Time
	owner     *string
	gameState GameState

	// username to (hashed) password
	userMapping map[string]User
	// otp to username
	otpMapping map[string]string

	// List of 100 problems -- stores the question numbers
	Problems [100]int

	clients ClientList // TODO: investigate needs to be merged with userMapping (?)

	// Using a syncMutex here to be able to lcok state before editing clients
	// Could also use Channels to block
	sync.RWMutex

	// otps is a map of allowed OTP to accept connections from
	otps RetentionMap
}

// UUID to Lobby map
type LobbyList map[string]*Lobby

// Manager is used to hold references to all Clients Registered, and Broadcasting etc
type Manager struct {
	lobbies LobbyList
	ctx     context.Context
}

// NewManager is used to initalize all the values inside the manager
func NewManager(ctx context.Context) *Manager {
	m := &Manager{
		lobbies: make(LobbyList),
		ctx:     ctx,
	}
	return m
}

func NewLobby(ctx context.Context, name string, id string) *Lobby {
	l := &Lobby{
		userMapping: make(map[string]User),
		otpMapping:  make(map[string]string),
		timeLimit:   10,
		id:          id,
		name:        name,
		owner:       nil,
		gameState:   WaitingForPlayers,
		startTime:   nil,
		clients:     make(ClientList),
		otps:        NewRetentionMap(ctx, 5*time.Second),
	}

	localProblems := GetProblems()
	// declare a boolean array the same size as the problems array
	booleanArray := make([]bool, len(localProblems.Problems))

	for i := 0; i < len(l.Problems); i++ {
		// Generate x as a random value between 0 and the length of the problems array
		// and as long as the randomly chosen problem isn't already selected
		x := rand.Intn(len(booleanArray))
		for booleanArray[x] {
			x = rand.Intn(len(booleanArray))
		}

		l.Problems[i] = x
		booleanArray[x] = true
	}

	return l
}

func (lobby *Lobby) startGame() {
	if lobby.gameState != WaitingForPlayers {
		panic("Game is already in progress")
	}
	lobby.gameState = InPlay
}

func (lobby *Lobby) endGame() {
	if lobby.gameState != InPlay {
		panic("Game isn't in progress")
	}
	lobby.gameState = Finished
}

func (lobby *Lobby) inPlay() bool {
	return lobby.gameState == InPlay
}

// routeEvent is used to make sure the correct event goes into the correct handler
func (m *Manager) routeEvent(event Event, c *Client) error {
	// Check if Handler is present in Map
	if handler, ok := handlers[event.Type]; ok {
		println("Event from " + c.name + " in lobby " + c.lobby.name + ": " + event.Type + "")
		// Execute the handler and return any err
		if err := handler(event, c); err != nil {
			return err
		}
		return nil
	} else {
		return ErrEventNotSupported
	}
}

// loginHandler is used to verify an user authentication and return a one time password
func (m *Manager) loginHandler(w http.ResponseWriter, r *http.Request) {

	type userLoginRequest struct {
		Username string `json:"username"`
		Password string `json:"password"`
		LobbyId  string `json:"lobbyId"` // UUID
	}

	var req userLoginRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	lobbyId := req.LobbyId
	lobby, lobbyExists := m.lobbies[lobbyId]
	if !lobbyExists {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Hashed password from the request
	hashedReqPassword, err := HashPassword(req.Password)
	if err != nil {
		log.Println(err)
		return
	}

	user, userExists := lobby.userMapping[req.Username]
	if !userExists {
		user.password = hashedReqPassword
		// Initialise user
		lobby.userMapping[req.Username] = user
	}

	// authenticate user / verify access token
	if CheckPasswordHash(req.Password, user.password) {
		// If authentication passes, set the owner of the lobby
		if lobby.owner == nil {
			lobby.owner = &req.Username
		}

		// add a new OTP
		otp := lobby.otps.NewOTP()
		lobby.otpMapping[otp.Key] = req.Username

		// format to return otp in to the frontend
		type response struct {
			OTP   string `json:"otp"`
			Lobby string `json:"lobby"`
		}
		resp := response{
			OTP:   otp.Key,
			Lobby: lobbyId,
		}

		data, err := json.Marshal(resp)
		if err != nil {
			log.Println(err)
			return
		}
		// return a response to the authenticated user with the OTP
		w.WriteHeader(http.StatusOK)
		w.Write(data)
		return
	}

	// failure to auth
	w.WriteHeader(http.StatusUnauthorized)
}

// serveWS is a HTTP Handler that the has the Manager that allows connections
func (m *Manager) serveWS(w http.ResponseWriter, r *http.Request) {

	// Grab the OTP in the Get param
	otp := r.URL.Query().Get("otp")
	if otp == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	lobbyName := r.URL.Query().Get("l")
	lobby, lobbyExists := m.lobbies[lobbyName]
	if !lobbyExists {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Verify OTP is existing
	if !lobby.otps.VerifyOTP(otp) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	log.Println("New connection")
	// Begin by upgrading the HTTP request
	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	// Create New Client
	client := NewClient(conn, m, lobby, otp)
	// Add the newly created client to the manager
	m.lobbies[lobbyName].addClient(client)

	go client.readMessages()
	go client.writeMessages()
}

func (m *Manager) createLobbyHandler(w http.ResponseWriter, r *http.Request) {
	type createLobbyRequest struct {
		Name string `json:"lobbyName"`
	}
	var req createLobbyRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id := uuid.New().String()
	m.lobbies[id] = NewLobby(m.ctx, req.Name, id)

	// format to return otp in to the frontend
	type response struct {
		LobbyId string `json:"l"`
	}
	resp := response{
		LobbyId: id,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		log.Println(err)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// TODO(madhav): need update these functions?
// addClient will add clients to our clientList
func (m *Lobby) addClient(client *Client) bool {
	// Lock so we can manipulate
	m.Lock()
	defer m.Unlock()

	// Add Client
	m.clients[client] = true
	return true
}

// removeClient will remove the client and clean up
func (m *Lobby) removeClient(client *Client) {
	m.Lock()
	defer m.Unlock()

	// Check if Client exists, then delete it
	if _, ok := m.clients[client]; ok {
		// close connection
		client.connection.Close()
		// remove
		delete(m.clients, client)
	}
}
