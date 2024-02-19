package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func dummy(r *http.Request) bool {
	return true
}

var (
	/**
	websocketUpgrader is used to upgrade incomming HTTP requests into a persistent websocket connection
	*/
	websocketUpgrader = websocket.Upgrader{
		// Apply the Origin Checker
		CheckOrigin:     dummy,
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

var (
	ErrEventNotSupported = errors.New("this event type is not supported")
)

func (p *Problem) CheckAnswer(submittedAnswer string) bool {
	return true // TODO: Implement this (check against answer)
}

type User struct {
	password       string
	questionNumber int32
	score          int32
}

type GameState string

const (
	WaitingForPlayers GameState = "waiting"
	InPlay            GameState = "playing"
	Finished          GameState = "finished"
	DNE               GameState = "dne"
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

	useCustom      bool
	CustomProblems []*Problem
	CustomOrder    []int

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
		userMapping:    make(map[string]User),
		otpMapping:     make(map[string]string),
		timeLimit:      600,
		id:             id,
		name:           name,
		owner:          nil,
		gameState:      WaitingForPlayers,
		startTime:      nil,
		clients:        make(ClientList),
		otps:           NewRetentionMap(ctx, 1*time.Second),
		CustomProblems: nil,
		CustomOrder:    nil,
		useCustom:      false,
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
func (m *Manager) routeEvent(event *ClientSent, c *Client) {
	// Check if Handler is present in Map
	switch event.Message.(type) {
	case *ClientSent_RequestStart_:
		err := StartGameHandler(event.GetRequestStart(), c)
		log.Println(err)
	case *ClientSent_Answer:
		GiveAnswerHandler(event.GetAnswer(), c)
	case *ClientSent_RequestProblem_:
		RequestProblemHandler(event.GetRequestProblem(), c)
	}

	log.Print(time.Now().Format("2006/01/02 15:04:05") +
		" Event from " + c.name + " in lobby " + c.lobby.name + ": ",
	)

	if reflect.TypeOf(event.Message) != nil {
		log.Println(reflect.TypeOf(event.Message).String())
	}
}

// loginHandler is used to verify an user authentication and return a one time password
func (m *Manager) loginHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	var req LoginRequest
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	err = proto.Unmarshal(body, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	lobbyId := req.LobbyId
	lobby, lobbyExists := m.lobbies[*lobbyId]
	if !lobbyExists {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Hashed password from the request
	hashedReqPassword, err := HashPassword(*req.Password)
	if err != nil {
		log.Println(err)
		return
	}

	user, userExists := lobby.userMapping[*req.Username]
	if !userExists {
		user.password = hashedReqPassword
		// Initialise user
		lobby.userMapping[*req.Username] = user
	}

	// authenticate user / verify access token
	if CheckPasswordHash(req.Password, user.password) {
		// If authentication passes, set the owner of the lobby
		isOwner := false
		if lobby.owner == nil {
			lobby.owner = req.Username
			isOwner = true
		}

		// add a new OTP
		otp := lobby.otps.NewOTP()
		lobby.otpMapping[otp.Key] = *req.Username

		// format to return otp in to the frontend
		resp := LoginResponse{
			Otp:     &otp.Key,
			IsOwner: &isOwner,
		}

		data, err := proto.Marshal(&resp)
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
		log.Println("asdasd")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	lobbyName := r.URL.Query().Get("l")
	lobby, lobbyExists := m.lobbies[lobbyName]
	if !lobbyExists {
		log.Println("aswwwwwdasd")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if lobby.gameState == Finished {
		log.Println("asdasd")
		// Don't allow users to connected if the game has ended
		w.WriteHeader(http.StatusGone)
		return
	}

	// Verify OTP is existing
	if !lobby.otps.VerifyOTP(otp) {
		log.Println("otp ", otp, " not found in ", lobby.id)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	log.Println("New connection from ", lobby.otpMapping[otp] == *lobby.owner)
	// Begin by upgrading the HTTP request
	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		log.Println("hi")
		return
	}

	// Create New Client
	client := NewClient(conn, m, lobby, otp)
	// Add the newly created client to the manager
	lobby.addClient(client)

	go client.readMessages()
	go client.writeMessages()

	if lobby.gameState == WaitingForPlayers {
		// Sending newMember events to all joined clients
		var broadMessage = ServerSent_Add{Add: &ServerSent_AddMember{Name: &client.name}}
		for c := range client.lobby.clients {
			if c.name != client.name {
				c.egress <- protofy(&broadMessage)
			}
			var existingClientMessage = ServerSent_Add{Add: &ServerSent_AddMember{Name: &c.name}}
			client.egress <- protofy(&existingClientMessage)
			// var smallMessage = NewMemberEvent{c.name}
			// data, err = json.Marshal(smallMessage)
			// if err != nil {
			// 	log.Println(err)
			// 	return
			// }
			// var smallOutgoingEvent = Event{EventNewMember, data}
			// client.egress <- smallOutgoingEvent
		}
	} else if lobby.gameState == InPlay {
		var outgoingEvent = &ServerSent_Start{
			Start: &ServerSent_StartGame{StartTime: timestamppb.New(*lobby.startTime), Duration: &timestamppb.Timestamp{Seconds: int64(lobby.timeLimit)}}}
		client.egress <- protofy(outgoingEvent)

		newProblemMessage := client.getNewProblem()
		client.egress <- protofy(&newProblemMessage)
	}
}

func (m *Manager) lobbyStatus(w http.ResponseWriter, r *http.Request) {
	type lobbyStatusRequest struct {
		Id string `json:"lobbyId"`
	}
	var req lobbyStatusRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	type response struct {
		Status GameState `json:"lobbyStatus"`
	}

	lobby, lobbyExists := m.lobbies[req.Id]

	if !lobbyExists {
		var resp response
		// If lobby doesn't exist in map, either it's been deleted or the game has ended
		logFilepath := filepath.Join(".", "logs", req.Id+".result.json")
		if _, err := os.Stat(logFilepath); errors.Is(err, os.ErrNotExist) {
			resp = response{Status: DNE}
		} else {
			resp = response{Status: Finished}
		}
		data, err := json.Marshal(resp)

		if err != nil {
			log.Println(err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(data)
		return
	}

	resp := response{Status: lobby.gameState}
	data, err := json.Marshal(resp)
	if err != nil {
		log.Println(err)
	}
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
}

func (m *Manager) createLobbyHandler(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	var bodyBytes []byte
	var err error
	if r.Body != nil {
		bodyBytes, err = ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("Body reading error: %v", err)
			return
		}
		defer r.Body.Close()
	}
	req := CreateLobbyReq{}
	err = proto.Unmarshal(bodyBytes, &req)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id := uuid.New().String()
	m.lobbies[id] = NewLobby(m.ctx, *req.LobbyName, id)

	resp := CreateLobbyRes{
		LobbyId: &id,
	}

	data, err := proto.Marshal(&resp)
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
