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

type WsConn struct {
	Id   string
	Conn *websocket.Conn
}

type Server struct {
	port        string
	server      *http.Server
	connLock    sync.RWMutex
	Conns       map[*websocket.Conn]*WsConn
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
		Conns:       make(map[*websocket.Conn]*WsConn, 0),
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
	// go func() {
	// 	s.connLock.Lock()
	// 	s.Conns[c] = &WsConn{
	// 		Id:   uuid.NewString(),
	// 		Conn: c,
	// 	}
	// 	s.connLock.Unlock()
	// }()
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
				// s.connLock.RLock()
				// _, ok := s.Conns[conn]
				// s.connLock.RUnlock()
				if conn == nil {
					s.epoller.Remove(conn)
					// delete(s.Conns, conn)
					continue
				}

				_, msg, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(
						err,
						websocket.CloseNormalClosure,
						websocket.CloseGoingAway,
						websocket.CloseNoStatusReceived) {
						log.Printf(fmt.Sprintf("error parsing message: %v", err))
					} else {
						log.Printf(fmt.Sprintf("connection error: %v", err))
					}
					conn.Close()
					s.epoller.Remove(conn)
					// delete(s.Conns, conn)
					break
				}
				// s.connLock.RLock()
				// log.Printf(fmt.Sprintf("from client %s: %s\n", s.Conns[conn].Id, msg))
				err1 := conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("{\"clientId\": %s}", msg)))
				if err1 != nil {
					log.Printf(fmt.Sprintf("connection error: %v", err1))
				}
				// s.connLock.RUnlock()
			}
		case <-s.exitChannel:
			return
		}
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
