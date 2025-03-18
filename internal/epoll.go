package server

import (
	"fmt"
	"log"
	"reflect"
	"sync"
	"syscall"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/sys/unix"
)

type WsConn struct {
	Id   string
	Conn *websocket.Conn
}

type Epoll struct {
	Fd          int
	Connections map[int]*WsConn
	Lock        *sync.RWMutex
}

func NewEpoll() (*Epoll, error) {
	fd, err := unix.EpollCreate1(0)
	if err != nil {
		log.Fatal("could not create epoll")
	}
	return &Epoll{
		Fd:          fd,
		Connections: make(map[int]*WsConn),
		Lock:        &sync.RWMutex{},
	}, nil
}

func (ep *Epoll) Add(conn *websocket.Conn) error {
	fd := ConnToFd(conn)

	err := unix.EpollCtl(
		ep.Fd, syscall.EPOLL_CTL_ADD,
		fd,
		&unix.EpollEvent{
			Events: unix.POLLIN | unix.POLLHUP,
			Fd:     int32(fd),
		})
	if err != nil {
		return err
	}
	ep.Lock.Lock()
	ep.Connections[fd] = &WsConn{
		Id:   uuid.NewString(),
		Conn: conn,
	}
	if len(ep.Connections)%100 == 0 {
		log.Printf("Total number of connections: %v", len(ep.Connections))
	}
	ep.Lock.Unlock()
	return nil
}

func (ep *Epoll) Remove(conn *websocket.Conn) error {
	if conn == nil {
		return fmt.Errorf("conn is nil")
	}
	fd := ConnToFd(conn)
	err := unix.EpollCtl(
		ep.Fd,
		syscall.EPOLL_CTL_DEL,
		fd,
		nil,
	)
	if err != nil {
		return err
	}
	ep.Lock.Lock()
	defer ep.Lock.Unlock()
	delete(ep.Connections, fd)
	if len(ep.Connections)%100 == 0 {
		log.Printf("Total number of connections: %v", len(ep.Connections))
	}
	return nil
}

func (ep *Epoll) Wait() ([]*WsConn, error) {
	events := make([]unix.EpollEvent, 100)
	n, err := unix.EpollWait(
		ep.Fd,
		events,
		100,
	)

	if err != nil {
		return nil, err
	}
	ep.Lock.RLock()
	defer ep.Lock.RUnlock()
	var connections []*WsConn
	for i := 0; i < n; i++ {
		connections = append(connections, ep.Connections[int(events[i].Fd)])
	}
	return connections, nil
}

func ConnToFd(conn *websocket.Conn) int {
	connVal := reflect.Indirect(reflect.ValueOf(conn)).FieldByName("conn").Elem()
	tcpConn := reflect.Indirect(connVal).FieldByName("conn")
	fdVal := tcpConn.FieldByName("fd")
	pfdVal := reflect.Indirect(fdVal).FieldByName("pfd")
	return int(pfdVal.FieldByName("Sysfd").Int())
}
