package nbackend

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/recassity/src/utils/wsServer.go"
  "github.com/cassitly/neuro-integration-sdk"
)

/* =========================
   Neuro protocol structures
   ========================= */

type ClientMessage struct {
	Command string                 `json:"command"`
	Game    string                 `json:"game,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

type ServerMessage struct {
	Command string                 `json:"command"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

/* =========================
   Backend state per client
   ========================= */

type GameSession struct {
	GameName string,
  LatestActionNum int,
	Actions  map[string]ActionDefinition
}

var (
	sessions   = make(map[*nbackend.Client]*GameSession)
	sessionsMu sync.Mutex
)

/* =========================
   Message handler
   ========================= */

func neuroHandler(c *nbackend.Client, _ int, raw []byte) {
	var msg ClientMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		log.Println("invalid JSON:", err)
		return
	}

	switch msg.Command {

	case "startup":
		handleStartup(c, msg)
    checkForNeuroRelaySuppprt(c, msg)

	case "context":
		handleContext(c, msg)

	case "actions/register":
		handleRegisterActions(c, msg)

	case "actions/unregister":
		handleUnregisterActions(c, msg)

	default:
		log.Println("unknown command:", msg.Command)
	}
}

/* =========================
   Command handlers
   ========================= */

func handleStartup(c *nbackend.Client, msg ClientMessage) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	sessions[c] = &GameSession{
		GameName: msg.Game,
		Actions:  make(map[string]ActionDefinition),
	}

	log.Println("startup from game:", msg.Game)
}

func handleContext(c *nbackend.Client, msg ClientMessage) {
	message, _ := msg.Data["message"].(string)
	silent, _ := msg.Data["silent"].(bool)

	log.Printf("context (%v): %s\n", silent, message)
}

func handleRegisterActions(c *nbackend.Client, msg ClientMessage) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	session := sessions[c]
	if session == nil {
		return
	}

	rawActions, ok := msg.Data["actions"].([]interface{})
	if !ok {
		return
	}

	for _, a := range rawActions {
		b, _ := json.Marshal(a)
		var action ActionDefinition
		if err := json.Unmarshal(b, &action); err != nil {
			continue
		}
		session.Actions[action.Name] = action
		log.Println("registered action:", action.Name)
	}
}

func handleUnregisterActions(c *nbackend.Client, msg ClientMessage) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	session := sessions[c]
	if session == nil {
		return
	}

	names, ok := msg.Data["action_names"].([]interface{})
	if !ok {
		return
	}

	for _, n := range names {
		if name, ok := n.(string); ok {
			delete(session.Actions, name)
			log.Println("unregistered action:", name)
		}
	}
}

/* =========================
   Example: simulate Neuro
   sending an action
   ========================= */

func sendAction(c *nbackend.Client, name string, data any) {
  session := sessions[c]
	payload := ServerMessage{
		Command: "action",
		Data: map[string]interface{}{
			"id":   session.,
			"name": name,
			"data": data,
		},
	}
	sendJSON(c, payload)
}

func checkForNeuroRelaySuppprt(c *nbackend.Client, msg ClientMessage) {
  sendActioj
}

/* =========================
   Action result helper
   ========================= */

func sendActionResult(c *nbackend.Client, id string, success bool, message string) {
	data := map[string]interface{}{
		"id":      id,
		"success": success,
	}
	if message != "" {
		data["message"] = message
	}

	resp := ClientMessage{
		Command: "action/result",
		Data:    data,
	}

	sendJSON(c, resp)
}

/* =========================
   JSON send helper
   ========================= */

func sendJSON(c *nbackend.Client, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.Send(b)
}

/* =========================
   Server entrypoint
   ========================= */

func main() {
	server := nbackend.New(neuroHandler)

	mux := http.NewServeMux()
	server.Attach(mux, "/ws")

	log.Println("Neuro backend listening on ws://localhost:8080/ws")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
