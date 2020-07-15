package services

import (
	"context"
	"finPrj/internal/robots"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type RobotsPatch struct {
	logger *zap.Logger
	mutex  sync.Mutex
	nextID int64
	users  map[int64]*websocket.Conn
}

func NewRobotsPatch(logger *zap.Logger) *RobotsPatch {
	return &RobotsPatch{
		logger: logger,
		mutex:  sync.Mutex{},
		users:  make(map[int64]*websocket.Conn),
	}
}

func (robo *RobotsPatch) PrepareSocket(w http.ResponseWriter, r *http.Request) {
	var up = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	if r.Header.Get("Origin") != "http://"+r.Host {
		http.Error(w, "Origin not allowed", http.StatusForbidden)
		return
	}

	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		robo.logger.Sugar().Warnf("PrepareSocket:: can't upgrade connection %s", err)
		return
	}

	conn.SetPingHandler(func(appData string) error {
		err := conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second*1))
		if err == websocket.ErrCloseSent {
			return nil
		} else if e, ok := err.(net.Error); ok && e.Temporary() {
			return nil
		}
		return err
	})

	robo.AddUser(conn)
}

func (robo *RobotsPatch) AddUser(conn *websocket.Conn) {
	robo.mutex.Lock()
	defer robo.mutex.Unlock()

	robo.nextID++
	robo.users[robo.nextID] = conn
}

func (robo *RobotsPatch) Broadcast(robot *robots.Robot) {
	robo.mutex.Lock()
	inactiveusers := make([]int64, 0)
	for id, conn := range robo.users {
		if err := conn.WriteJSON(robot); err != nil {
			robo.logger.Sugar().Warnf("Broadcast:: can't write ro socket %s", err)
			inactiveusers = append(inactiveusers, id)
		}
	}
	robo.mutex.Unlock()
	robo.Removeusers(inactiveusers...)
}

func (robo *RobotsPatch) Removeusers(IDs ...int64) {
	robo.mutex.Lock()
	defer robo.mutex.Unlock()

	for _, id := range IDs {
		robo.users[id].Close()
		delete(robo.users, id)
	}
}

func (robo *RobotsPatch) ScanUpdates(ctx context.Context, roboUpd <-chan *robots.Robot) {
	for {
		select {
		case newRobot, ok := <-roboUpd:
			if ok {
				robo.Broadcast(newRobot)
			} else {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
