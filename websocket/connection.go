/*Package websocket provides abstraction over websocket connections. Since underlying websocket object does not support
 concurrent writes and the collection of websockets have to be maintained in a thread safe manner, all these operations
 are serialized in operations chan.

Stress test comparison using sync.mutex vs queued operations
=== RUN   TestSandboxStress
Received 133499 messages in 20s
--- PASS: TestSandboxStress (21.01s)
PASS

Run with writes, reads and sends are queued in the same channel
=== RUN   TestSandboxStress
Received 140566 messages in 20s
--- PASS: TestSandboxStress (21.01s)
PASS
*/
package websocket

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/qulia/go-log/log"
)

type socketOperationType int

const (
	add socketOperationType = iota
	remove
	send
)

type socketOperation struct {
	opType socketOperationType
	socket *websocket.Conn
	msg    *Message
}

// ConnectionManager manages web socket connections
type ConnectionManager struct {
	sockets    map[*websocket.Conn]bool // Using map for faster removal and access
	upgrader   websocket.Upgrader
	operations chan *socketOperation
}

// NewConnectionManager default connection manager
func NewConnectionManager() *ConnectionManager {
	log.V("New connection manager\n")
	cm := new(ConnectionManager)
	cm.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	cm.sockets = make(map[*websocket.Conn]bool)
	cm.operations = make(chan *socketOperation, 1)
	go func() {
		for op := range cm.operations {
			switch op.opType {
			case add:
				cm.addSocket(op.socket)
			case remove:
				cm.removeSocket(op.socket)
			case send:
				for socket := range cm.sockets {
					log.V("Sending message on websocket\n")
					err := socket.WriteJSON(op.msg)
					if err != nil {
						log.E(err, "Write was not successful, will remove the socket\n")
						cm.operations <- &socketOperation{
							opType: remove,
							socket: socket,
							msg:    nil,
						}
					}
				}
			}
		}
	}()
	return cm
}

// Receive upgrade http to websocket and listen
func (cm *ConnectionManager) Receive(
	w http.ResponseWriter, r *http.Request, onReceive func(*Message)) {
	log.V("Receive\n")
	socket, err := cm.upgrader.Upgrade(w, r, nil)

	// TODO make this log.E
	log.F(err, "Upgrade to websocket failed\n")
	cm.operations <- &socketOperation{
		opType: add,
		socket: socket,
	}

	// TODO handle failures
	go cm.receive(socket, onReceive)
}

// Send messages on web socket
func (cm *ConnectionManager) Send(msg *Message) {
	cm.operations <- &socketOperation{
		opType: send,
		socket: nil,
		msg:    msg,
	}
}

func (cm *ConnectionManager) receive(
	socket *websocket.Conn, onReceive func(*Message)) {
	for {
		msg := Message{}
		err := socket.ReadJSON(&msg)

		if err != nil {
			log.E(err, "Error reading message from the socket\n")
			cm.operations <- &socketOperation{
				opType: remove,
				socket: socket,
				msg:    nil,
			}
			break
		}

		onReceive(&msg)
	}
}

func (cm *ConnectionManager) addSocket(socket *websocket.Conn) {
	cm.sockets[socket] = true
}

func (cm *ConnectionManager) removeSocket(socket *websocket.Conn) {
	log.E(socket.Close(), "Failed to close socket\n")
	delete(cm.sockets, socket)
}
