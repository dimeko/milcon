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
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	DEFAULT_WS_URL = "ws://localhost:6767/ws"
	LOG_FILE       = "client.log"
	MAX_RETRIES    = 5
)

var (
	verbose = false
	wsUrl   = ""
	logger  *slog.Logger
)

var (
	syncStats               = sync.Mutex{}
	connectedClients        = 0
	disconnectedClients     = 0
	connectionRetries       = 0
	clientsSentFirstMessage = 0
	clientsReceivedMessage  = 0
)

func setLimit() {
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		panic(err)
	}

	log.Printf("set cur limit: %d", rLimit.Cur)
}

func connect(c int, s chan<- int, exitChan <-chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			logger.Debug(fmt.Sprintf("panic recovery: %v", r))
			s <- c
		}
	}()
	dialer := websocket.Dialer{
		HandshakeTimeout: 60 * time.Second,
	}
	var conn *websocket.Conn
	tries := MAX_RETRIES
	for tries > 0 {
		var err0 error
		conn, _, err0 = dialer.Dial(wsUrl, http.Header{})
		if err0 != nil {
			tries--
			syncStats.Lock()
			connectionRetries++
			syncStats.Unlock()
			continue
		}
		break
	}

	if tries == 0 {
		logger.Debug(fmt.Sprintf("client %d could not connect", c))
		s <- c
		return
	}

	syncStats.Lock()
	connectedClients++
	syncStats.Unlock()
	err1 := conn.WriteMessage(websocket.TextMessage, []byte("{}"))
	if err1 != nil {
		logger.Debug(fmt.Sprintf("connection error: %v", err1))
		s <- c
		return
	}
	syncStats.Lock()
	clientsSentFirstMessage++
	syncStats.Unlock()
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
			syncStats.Lock()
			clientsReceivedMessage++
			syncStats.Unlock()

			logger.Debug(fmt.Sprintf("from client %d: %s\n", c, msg))
		}
	}
}

func printLoop(exitChan <-chan struct{}) {
	for {
		select {
		case <-exitChan:
			return
		default:
			cmd := exec.Command("clear")
			cmd.Stdout = os.Stdout
			cmd.Run()
			fmt.Println("----- Stats -----")
			fmt.Printf("connected:          %d\n", connectedClients)
			fmt.Printf("disconnected:       %d\n", disconnectedClients)
			fmt.Printf("connection retries: %d\n", connectionRetries)
			fmt.Printf("sent first message: %d\n", clientsSentFirstMessage)
			fmt.Printf("received response:  %d\n", clientsSentFirstMessage)
			time.Sleep(2 * time.Second)
		}

	}

}

func main() {
	var (
		connectionsNumberArg = flag.Int("n", 1, "determine the number of parallel connections")
		websocketsUrlArg     = flag.String("u", DEFAULT_WS_URL, "determine the full url of the server")
		verboseArg           = flag.Bool("v", false, "log more information e.g. messages received")
	)

	flag.Parse()
	exitChan := make(chan struct{})
	go printLoop(exitChan)
	setLimit()

	wsUrl = *websocketsUrlArg

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
	for i := 0; i < *connectionsNumberArg; i++ {
		go connect(i, syncChan, exitChan)
	}
	counter := 0
	for {
		select {
		case clientIdx := <-syncChan:
			logger.Debug(fmt.Sprintf("client %d was disconnected", clientIdx))
			syncStats.Lock()
			disconnectedClients++
			syncStats.Unlock()
			counter++
		case <-sigChan:
			fmt.Println("exiting, bye ...")
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
