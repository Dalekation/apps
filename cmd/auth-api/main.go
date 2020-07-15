package main

import (
	"context"
	bs "finPrj/internal/buyingservice"
	pg "finPrj/internal/postgres"
	"finPrj/internal/robots"
	srvc "finPrj/internal/services"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	"time"
)

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatalf("can't create logger:: %s", err)
	}
	defer logger.Sync()

	cfg := pg.Config{
		URL: "postgres://Dalek:8923456@localhost:5432/fintech" +
			"?sslmode=disable",
		MaxConnections:  100,
		MaxConnLifetime: 20 * time.Second,
	}
	db, err := pg.New(logger, cfg)
	if err != nil {
		logger.Sugar().Fatalf("can't create database:: %s", err)
	}

	err = db.CheckConnection()
	if err != nil {
		logger.Sugar().Fatalf("can't connect database:: %s", err)
	}

	updChan := make(chan *robots.Robot)
	defer close(updChan)

	userStorage, err := pg.NewUserStorage(db)
	if err != nil {
		logger.Sugar().Fatalf("can't create user database:: %s", err)
	}
	roboStorage, err := pg.NewRobotStorage(db, updChan)
	if err != nil {
		logger.Sugar().Fatalf("can't create robots database:: %s", err)
	}
	sessStorage, err := pg.NewSessionStorage(db)
	if err != nil {
		logger.Sugar().Fatalf("can't create sessions database:: %s", err)
	}

	rp := srvc.NewRobotsPatch(logger)
	h := NewHandlers(logger, userStorage, sessStorage, roboStorage, rp)

	r := h.Router()
	ctx, cancel := context.WithCancel(context.Background())

	addr := net.JoinHostPort("", "5000")
	srv := &http.Server{Addr: addr, Handler: r}

	sigquit := make(chan os.Signal, 1)
	signal.Ignore(syscall.SIGHUP, syscall.SIGPIPE)
	signal.Notify(sigquit, syscall.SIGINT, syscall.SIGTERM)
	stopAppCh := make(chan struct{})
	go func() {
		s := <-sigquit
		logger.Sugar().Info("captured signal: %v\n", s)
		logger.Sugar().Info("gracefully shutting down server")
		if err := srv.Shutdown(context.Background()); err != nil {
			logger.Sugar().Fatalf("can't shutdown server: %s", err)
		}
		if err := db.Close(); err != nil {
			logger.Sugar().Fatalf("can't close data base: %s", err)
		}

		cancel()

		fmt.Println("server stopped")
		stopAppCh <- struct{}{}
	}()

	go rp.ScanUpdates(ctx, updChan)

	conn, err := grpc.Dial("localhost:8080", grpc.WithInsecure())
	if err != nil {
		logger.Sugar().Fatalf("can't connect to streaming service %s", err)
	}
	defer conn.Close()

	BuyServ := bs.NewBuyingService(logger, roboStorage, conn)
	BuyServ.ActivateNewRobots(ctx)

	fmt.Println("Launching server")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Sugar().Fatalf("Can't launch server %s", err)
	}

	<-stopAppCh
	//testUser(userStorage)
	//testSession(sessStorage)
	//testRobot(roboStorage)
}

/*
func testUser(usSt *pg.UserStorage) {
	user1 := users.User{
		ID:        1,
		Email:     "hello@a.ru",
		Password:  "dbigshbdisdbgidsgb",
		CreatedAt: time.Now().UTC(),
		FirstName: "ivan",
		LastName:  "ivanov",
		UpdatedAt: time.Now().UTC(),
	}

	user2 := users.User{
		ID:        2,
		Email:     "helllllo@a.ru",
		Password:  "dbigshbdsdfsdgsdgsdgsdisdbgidsgb",
		CreatedAt: time.Now().UTC(),
		FirstName: "serg",
		Birthday:  time.Date(2000, 5, 29, 0, 0, 0, 0, time.UTC),
		LastName:  "sergov",
		UpdatedAt: time.Now().UTC(),
	}

	nextID, err := usSt.NextID()
	if err != nil {
		log.Printf("nextid1::%s", err)
		return
	}
	log.Println(nextID)
	err = usSt.Create(&user1)
	if err != nil {
		log.Printf("cruser1::%s", err)
		return
	}

	nextID, err = usSt.NextID()
	if err != nil {
		log.Printf("nextid2::%s", err)
		return
	}
	log.Println(nextID)
	err = usSt.Create(&user2)
	if err != nil {
		log.Printf("cruser2::%s", err)
		return
	}

	err = usSt.Create(&user1)
	if err != nil {
		log.Printf("cruser3::%s", err)
	}

	user3, err := usSt.GetByID(2)
	if err != nil {
		log.Printf("getbyid1::%s", err)
		return
	}
	log.Println(user3)
	user4, err := usSt.GetByID(3)
	if err != nil {
		log.Printf("getbyid2::%s", err)
	}
	log.Println(user4)
	user5, err := usSt.GetByEmail("hello@a.ru")
	if err != nil {
		log.Printf("getml1::%s", err)
		return
	}
	log.Println(user5)
	user6, err := usSt.GetByEmail("helllllo@a.ru")
	if err != nil {
		log.Printf("getml2::%s", err)
		return
	}
	log.Println(user6)
	user7, err := usSt.GetByEmail("helllllo@asdfsd.ru")
	if err != nil {
		log.Printf("getml3::%s", err)
	}
	log.Println(user7)
	user1.Birthday = time.Now()

	err = usSt.UpdateUser(&user1)
	if err != nil {
		log.Printf("upd1::%s", err)
		return
	}
}

func testSession(ssSt *pg.SessionStorage) {
	session := sessions.Session{
		SessionID:  "fdbigbgdifjgbdjf",
		UserID:     2,
		CreatedAt:  time.Now().UTC(),
		ValidUntil: time.Now().UTC().Add(30 * time.Minute),
	}

	err := ssSt.Create(&session)
	if err != nil {
		log.Fatalf("can't create session:: %s", err)
		return
	}

	session2, err := ssSt.GetByUserID(2)
	if err != nil {
		log.Fatalf("can't get session by user id:: %s", err)
		return
	}
	log.Println(session2)
	session3, err := ssSt.GetByUserID(100)
	if err != nil {
		log.Fatalf("can't get session by user id:: %s", err)
		return
	}
	log.Println(session3)

	session4, err := ssSt.GetByBearer("fdbigbgdifjgbdjf")
	if err != nil {
		log.Fatalf("can't get session by user id:: %s", err)
		return
	}
	log.Println(session4)

	session4, err = ssSt.GetByBearer("fdbigbgdifjgbdj")
	if err != nil {
		log.Fatalf("can't get session by user id:: %s", err)
		return
	}
	log.Println(session4)

	err = ssSt.DeleteByUserID(100)
	if err != nil {
		log.Fatalf("can't get session by user id:: %s", err)
		return
	}

	err = ssSt.DeleteByUserID(2)
	if err != nil {
		log.Fatalf("can't get session by user id:: %s", err)
		return
	}

	session2, err = ssSt.GetByUserID(2)
	if err != nil {
		log.Fatalf("can't get session by user id:: %s", err)
		return
	}
	log.Println(session2)
}

func testRobot(rsSt *pg.RobotStorage) {
	nextID, err := rsSt.NextID()
	if err != nil {
		log.Printf("nextid1::%s", err)
		return
	}
	log.Println(nextID)

	robot := robots.Robot{
		RobotID:     1,
		OwnerUserID: 2,
		IsFavourite: true,
		IsActive:    true,
	}

	err = rsSt.Create(&robot)
	if err != nil {
		log.Fatalf("1:: %s", err)
	}

	robot.RobotID++
	robot.ParentRobotID = 1

	err = rsSt.Create(&robot)
	if err != nil {
		log.Fatalf("2:: %s", err)
	}

	robot.RobotID++
	robot.Ticker = "AAPL"

	err = rsSt.Create(&robot)
	if err != nil {
		log.Fatalf("3:: %s", err)
	}

	err = rsSt.Create(&robots.Robot{
		RobotID:       4,
		OwnerUserID:   1,
		IsFavourite:   true,
		IsActive:      true,
		ParentRobotID: 1,
		Ticker:        "AAPL",
		BuyPrice:      56.5,
		SellPrice:     46.78,
		PlanStart:     time.Date(2000, 10, 02, 15, 0, 0, 0, time.UTC),
		PlanEnd:       time.Date(2000, 10, 02, 19, 0, 0, 0, time.UTC),
		PlanYield:     1000,
		FactYield:     100,
		DealsCount:    10,
		ActivatedAt:   time.Date(2000, 10, 02, 15, 0, 0, 0, time.UTC),
		DeactivatedAt: time.Date(2000, 10, 02, 19, 0, 0, 0, time.UTC),
		CreatedAt:     time.Date(2000, 10, 02, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		log.Fatalf("4:: %s", err)
	}

	nextID, err = rsSt.NextID()
	if err != nil {
		log.Printf("nextid2::%s", err)
		return
	}
	log.Println(nextID)

	one, err := rsSt.GetByTickerAndOwnerID("", 2)
	log.Println(err, one)

	one, err = rsSt.GetByTickerAndOwnerID("AAPL", 0)
	log.Println(err, one)

	one, err = rsSt.GetByTickerAndOwnerID("AAPL", 1)
	log.Println(err, one)

	one, err = rsSt.GetByOwnerID(1)
	log.Println(err, one)

	ne, err := rsSt.GetByRobotID(1)
	log.Println(err, ne)

	one, err = rsSt.GetAllRobots()
	log.Println(err, one)

	robot = robots.Robot{
		RobotID:       1,
		OwnerUserID:   1,
		IsFavourite:   true,
		IsActive:      true,
		ParentRobotID: 1,
		Ticker:        "AAPL",
		BuyPrice:      56.5,
		SellPrice:     46.78,
		PlanStart:     time.Date(2000, 10, 02, 15, 0, 0, 0, time.UTC),
		PlanEnd:       time.Date(2000, 10, 02, 19, 0, 0, 0, time.UTC),
		PlanYield:     1000,
		FactYield:     100,
		DealsCount:    10,
		ActivatedAt:   time.Date(2000, 10, 02, 15, 0, 0, 0, time.UTC),
		DeactivatedAt: time.Date(2000, 10, 02, 19, 0, 0, 0, time.UTC),
		CreatedAt:     time.Date(2000, 10, 02, 15, 0, 0, 0, time.UTC),
	}

	err = rsSt.UpdateRobot(&robot)
	log.Println(err)

	err = rsSt.ActivateRobot(2)
	log.Println(err)

	err = rsSt.DeactivateRobot(3)
	log.Println(err)

	err = rsSt.DeleteRobot(4)
	log.Println(err)
}*/
