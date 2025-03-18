package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	WS_ENDPOINT = "ws://localhost:6767/ws"
	LOG_FILE    = "client.log"
)

var (
	verbose = false
	logger  *slog.Logger
)

var (
	connectedClients        = 0
	disconnectedClients     = 0
	clientsSentFirstMessage = 0
	clientsReceivedMessage  = 0
)

func connect(c int, s chan<- int, exitChan <-chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			logger.Debug(fmt.Sprintf("panic recovery: %v", r))
			s <- c
		}
	}()
	dialer := websocket.Dialer{
		HandshakeTimeout: 3600 * time.Second,
	}
	conn, _, err0 := dialer.Dial(WS_ENDPOINT, http.Header{})
	if err0 != nil {
		logger.Debug(fmt.Sprintf("client %d could not connect", c))
		s <- c
		return
	}
	connectedClients++
	err1 := conn.WriteMessage(websocket.TextMessage, []byte("{}"))
	if err1 != nil {
		logger.Debug(fmt.Sprintf("connection error: %v", err1))
		s <- c
		return
	}
	clientsSentFirstMessage++
	defer conn.Close()
	for {
		select {
		case <-exitChan:
			s <- c
			return
		default:
			time.Sleep(1 * time.Second)

			_, msg, err2 := conn.ReadMessage()
			if err2 != nil {
				if websocket.IsUnexpectedCloseError(err2, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
					logger.Debug(fmt.Sprintf("%d error parsing message: %v", c, err2))
				} else {
					logger.Debug(fmt.Sprintf("%d connection error: %v", c, err2))
				}
				s <- c
				return
			}
			clientsReceivedMessage++
			logger.Debug(fmt.Sprintf("from client %d: %s\n", c, msg))
		}
	}
}

func printLoop() {
	for {
		cmd := exec.Command("clear")
		cmd.Stdout = os.Stdout
		cmd.Run()
		fmt.Println("Client statistics")
		fmt.Printf("clients connected:          %d\n", connectedClients)
		fmt.Printf("clients disconnected:       %d\n", disconnectedClients)
		fmt.Printf("clients sent first message: %d\n", clientsSentFirstMessage)
		fmt.Printf("clients received response:  %d\n", clientsSentFirstMessage)
		time.Sleep(2 * time.Second)
	}

}

func main() {
	var (
		connectionsNumberArg = flag.Int("n", 1, "determine the number of parallel connections")
		verboseArg           = flag.Bool("v", false, "log more information e.g. messages received")
	)

	flag.Parse()
	go printLoop()

	if *connectionsNumberArg <= 0 {
		log.Fatal("Number of connections must be a positive number")
	}
	var logHandlerOptions *slog.HandlerOptions
	if *verboseArg {
		logHandlerOptions = &slog.HandlerOptions{
			AddSource: false,
			Level:     slog.LevelDebug,
		}
	} else {
		logHandlerOptions = &slog.HandlerOptions{
			AddSource: false,
			Level:     slog.LevelError,
		}
	}

	f, err := os.OpenFile("./"+LOG_FILE, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		log.Fatal("could not open log file")
	}
	logger = slog.New(slog.NewTextHandler(f, logHandlerOptions))
	logger.Debug(fmt.Sprintf("logs in: %s\n", LOG_FILE))

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)

	syncChan := make(chan int, *connectionsNumberArg)
	exitChan := make(chan struct{})
	for i := 0; i < *connectionsNumberArg; i++ {
		go connect(i, syncChan, exitChan)
	}

	counter := 0
	for {
		select {
		case clientIdx := <-syncChan:
			logger.Debug(fmt.Sprintf("client %d was disconnected", clientIdx))
			disconnectedClients++
			counter++
		case <-sigChan:
			close(exitChan)
		}
		if counter == *connectionsNumberArg {
			logger.Debug("all clients have been disconnected successfully")
			break
		} else {
			logger.Debug(fmt.Sprintf("disconnected clients: %d", counter))
		}
	}
}
