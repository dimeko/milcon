package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

type Server struct {
	port        string
	server      *http.Server
	connLock    sync.RWMutex
	epoller     *Epoll
	exitChannel chan struct{}
	running     sync.WaitGroup
}

func NewServer(port string) *Server {
	_ep, err := NewEpoll()
	if err != nil {
		log.Fatal("could not create epoller")
	}
	return &Server{
		port:        port,
		epoller:     _ep,
		exitChannel: make(chan struct{}),
		running:     sync.WaitGroup{},
		connLock:    sync.RWMutex{},
	}
}

func (s *Server) error(w http.ResponseWriter, code int, err error) {
	http.Error(w, http.StatusText(code), code)
}

func (s *Server) handlerWebSocket(w http.ResponseWriter, r *http.Request) {
	var upgrader = websocket.Upgrader{}
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("failed to upgrade")
		s.error(w, http.StatusInternalServerError, fmt.Errorf("failed to upgrade connection: %w", err))
		return
	}
	if err := s.epoller.Add(c); err != nil {
		log.Printf("failed to add connection")
		c.Close()
	}
}

func (s *Server) handlerWrapper(handlerFunc func(http.ResponseWriter, *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			r := recover()
			if r != nil {
				log.Println("recovered: %v", r)
				s.error(w, http.StatusInternalServerError, fmt.Errorf("%v\n%v", r, string(debug.Stack())))
			}
		}()
		handlerFunc(w, r)
	})
}

func (s *Server) MainLoop() {
	log.Printf("Started main loop")
	for {
		select {
		default:
			connections, err := s.epoller.Wait()
			if err != nil {
				log.Printf("Failed to epoll wait %v", err)
				continue
			}
			for _, conn := range connections {
				if conn == nil {
					// s.epoller.Remove(conn.Conn)
					continue
				}

				_, _, err := conn.Conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(
						err,
						websocket.CloseNormalClosure,
						websocket.CloseGoingAway,
						websocket.CloseNoStatusReceived) {
						log.Printf("error parsing message: %s", err)
					} else {
						log.Printf("connection error: %s", err)
					}
					s.epoller.Remove(conn.Conn)
					continue
				}
				err1 := conn.Conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("{\"clientId\": %s}", conn.Id)))
				if err1 != nil {
					log.Printf("connection error: %v", err1)
				}
			}
		case <-s.exitChannel:
			return
		}
	}
}

func (s *Server) MockServerMessages() {
	for {
		time.Sleep(5 * time.Second)
		_counter := 0
		s.epoller.Lock.Lock()
		for _, v := range s.epoller.Connections {
			if _counter == 1000 {
				break // just breaking after the first 1000 messages sent from the server (map iteration has random order)
			}
			err0 := v.Conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("{\"clientId\": %s, \"msg\": %s}", v.Id, "hello")))
			if err0 != nil {
				log.Printf("error sending message to: %s", v.Id)
				continue
			}
			_counter++
		}
		s.epoller.Lock.Unlock()
	}
}

func (s *Server) Start(exitChannel chan os.Signal) error {
	r := mux.NewRouter()
	r.Handle("/ws", s.handlerWrapper(s.handlerWebSocket))
	s.server = &http.Server{
		Addr:    ":" + s.port,
		Handler: handlers.CombinedLoggingHandler(os.Stdout, r),
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				panic(err)
			}
		}
	}()
	setLimit()
	go s.MainLoop()
	go s.MockServerMessages()

	return nil
}

func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		log.Printf("error: %v\n", err)
	}
	close(s.exitChannel)
}

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
