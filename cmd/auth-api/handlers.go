package main

import (
	"encoding/base32"
	"encoding/json"
	pg "finPrj/internal/postgres"
	"finPrj/internal/robots"
	srvc "finPrj/internal/services"
	sessions "finPrj/internal/sessions"
	users "finPrj/internal/users"
	"net/http"
	"strconv"
	"text/template"
	"time"

	"github.com/go-chi/chi"
	"go.uber.org/zap"
)

var templates map[string]*template.Template

//is used to work with websockets and html
//there can situation, when no robot is found for ticker, or at all
//we don't wan't this to be an error
//cause asking for all robots, or for user robots is ok
//if they exist at all
type TemplRobots struct {
	Robots  []robots.Robot
	Ticker  string
	OwnerID string
	Token   string
}

func init() {
	if templates == nil {
		templates = make(map[string]*template.Template)
	}
	templates["robot"] = template.Must(template.ParseFiles("html/robot.html"))
	templates["robots"] = template.Must(template.ParseFiles("html/robots.html"))
	templates["usersrobots"] = template.Must(template.ParseFiles("html/usersrobots.html"))
	templates["error"] = template.Must(template.ParseFiles("html/error.html"))
}

type Handlers struct {
	logger *zap.Logger
	us     *pg.UserStorage
	ss     *pg.SessionStorage
	rs     *pg.RobotStorage
	rp     *srvc.RobotsPatch
}

func NewHandlers(logger *zap.Logger, us *pg.UserStorage, ss *pg.SessionStorage,
	rs *pg.RobotStorage, rp *srvc.RobotsPatch) *Handlers {
	return &Handlers{
		logger: logger,
		us:     us,
		ss:     ss,
		rs:     rs,
		rp:     rp,
	}
}
func (h *Handlers) Router() chi.Router {
	r := chi.NewRouter()
	r.Post("/api/v1/signup", h.SignUp)
	r.Post("/api/v1/signin", h.SignIn)
	r.Put("/api/v1/users/{id}", h.PutUser)
	r.Get("/api/v1/users/{id}", h.GetUser)
	r.Get("/user/{id}/robots", h.UserRobots)
	r.Post("/robot", h.PostRobot)
	r.Get("/robots", h.Robots)
	r.Delete("/robot/{id}", h.DeleteRobot)
	r.Get("/robot/{id}", h.RobotWithID)
	r.Put("/robot/{id}", h.UpdateRobot)
	r.Put("/robot/{id}/activate", h.ActivateRobot)
	r.Put("/robot/{id}/deactivate", h.DeactivateRobot)
	r.Put("/robot/{id}/favourite", h.FavourRobot)
	r.Get("/wsrobots", h.rp.PrepareSocket)
	return r
}

func (h *Handlers) SignUp(w http.ResponseWriter, r *http.Request) {
	user := &users.User{}
	err := json.NewDecoder(r.Body).Decode(user)
	if err != nil {
		h.logger.Sugar().Errorf("SignUp:: can't read user from Body %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	checkUser, err := h.us.GetByEmail(user.Email)
	if err != nil {
		h.logger.Sugar().Errorf("SignUp:: can't getByEmail %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")

	if checkUser != nil {
		w.WriteHeader(http.StatusConflict)
		err := json.NewEncoder(w).Encode(map[string]string{"error": "user " + user.Email + " is already registered"})
		if err != nil {
			h.logger.Sugar().Warnf("SignUp:: can't parse json error %s", err)
		}
		return
	}

	nextID, err := h.us.NextID()
	if err != nil {
		h.logger.Sugar().Errorf("SignUp:: can't get nextID %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	user.ID = nextID
	user.Password = base32.StdEncoding.EncodeToString([]byte(user.Password))
	user.CreatedAt = time.Now().UTC()
	user.UpdatedAt = time.Now().UTC()

	err = h.us.Create(user)
	if err != nil {
		h.logger.Sugar().Errorf("SignUp:: can't create user %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *Handlers) SignIn(w http.ResponseWriter, r *http.Request) {
	request := map[string]string{}

	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		h.logger.Sugar().Errorf("SignIn:: can't parse json %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	checkUser, err := h.us.GetByEmail(request["email"])
	if err != nil {
		h.logger.Sugar().Errorf("SignIn:: can't getByEmail %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")

	if checkUser == nil {
		w.WriteHeader(http.StatusBadRequest)
		err := json.NewEncoder(w).Encode(map[string]string{"error": "no user with such email"})
		if err != nil {
			h.logger.Sugar().Warnf("SignIn:: can't parse json error %s", err)
		}
		return
	}

	hashedPswd := base32.StdEncoding.EncodeToString([]byte(request["password"]))
	if checkUser.Password != hashedPswd {
		w.WriteHeader(http.StatusBadRequest)
		err := json.NewEncoder(w).Encode(map[string]string{"error": "wrong password"})
		if err != nil {
			h.logger.Sugar().Warnf("SignIn:: can't parse json error %s", err)
		}
		return
	}

	base := []byte(request["email"] + request["password"])
	session := sessions.Session{
		SessionID:  base32.StdEncoding.EncodeToString(base),
		UserID:     checkUser.ID,
		CreatedAt:  time.Now().UTC(),
		ValidUntil: time.Now().UTC().Add(30 * time.Minute),
	}

	err = h.ss.DeleteByUserID(session.UserID)
	if err != nil {
		h.logger.Sugar().Errorf("SignIn:: can't delete old session%s", err)
	}

	err = h.ss.Create(&session)
	if err != nil {
		h.logger.Sugar().Errorf("SignIn:: can't create session %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"bearer": session.SessionID})
}

func (h *Handlers) PutUser(w http.ResponseWriter, r *http.Request) {
	id, err := h.getID(w, r)
	if err != nil {
		return
	}

	user, err := h.us.GetByID(id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("PutUser:: can't get user by id %s", err)
		return
	}

	w.Header().Add("Content-Type", "application/json")

	if user == nil {
		w.WriteHeader(http.StatusBadRequest)
		err := json.NewEncoder(w).Encode(map[string]string{"error": "no user with such id"})
		if err != nil {
			h.logger.Sugar().Warnf("PutUser:: can't parse error %s", err)
		}
		return
	}

	status := h.checkAuth(w, r, id)
	if !status {
		return
	}

	user2 := users.User{}
	err = json.NewDecoder(r.Body).Decode(&user2)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Warnf("PutUser:: can't parse user from request %s", err)
		return
	}

	if user2.Email != user.Email {
		checkUser, err := h.us.GetByEmail(user.Email)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			h.logger.Sugar().Errorf("PutUser:: can't get user by email %s", err)
			return
		}

		if checkUser != nil {
			w.WriteHeader(http.StatusBadRequest)
			err := json.NewEncoder(w).Encode(map[string]string{"error": "user with such email already exist"})
			if err != nil {
				h.logger.Sugar().Warnf("PutUser:: can't parse error %s", err)
			}
			return
		}
	}

	user2.UpdatedAt = time.Now().UTC()
	user2.CreatedAt = user.CreatedAt
	err = h.us.UpdateUser(&user2)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("PutUser:: can't update user %s", err)
		return
	}

	err = json.NewEncoder(w).Encode(user2)
	if err != nil {
		h.logger.Sugar().Warnf("PutUser:: can't parse user as answer %s", err)
	}
}

func (h *Handlers) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := h.getID(w, r)
	if err != nil {
		return
	}

	user, err := h.us.GetByID(id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("GetUser:: can't get user by id %s", err)
		return
	}

	w.Header().Add("Content-Type", "application/json")

	if user == nil {
		w.WriteHeader(http.StatusBadRequest)
		err := json.NewEncoder(w).Encode(map[string]string{"error": "no user with such id"})
		if err != nil {
			h.logger.Sugar().Warnf("GetUser:: can't parse error %s", err)
		}
		return
	}

	status := h.checkAuth(w, r, id)
	if !status {
		return
	}

	err = json.NewEncoder(w).Encode(user)
	if err != nil {
		h.logger.Sugar().Warnf("PutUser:: can't parse user as answer %s", err)
	}
}

func (h *Handlers) checkAuth(w http.ResponseWriter, r *http.Request, userID int64) bool {
	var token string
	var err error
	if len(r.Header["Authorization"]) != 0 {
		token = r.Header["Authorization"][0]
	} else {
		w.WriteHeader(http.StatusBadRequest)
		if r.Header["Accept"][0] == "application/json" {
			w.Header().Add("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(map[string]string{"error": "token required"})
		} else {
			w.Header().Add("Content-Type", "text/html")
			err = templates["error"].Execute(w, "token required")
		}
		if err != nil {
			h.logger.Sugar().Warnf("checkAuth:: can't parse error %s", err)
		}
		return false
	}

	sess, err := h.ss.GetByUserID(userID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("checkAuth:: can't get session by id %s", err)
		return false
	}

	if sess == nil {
		w.WriteHeader(http.StatusUnauthorized)
		if r.Header["Accept"][0] == "application/json" {
			w.Header().Add("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(map[string]string{"error": "sign in first"})
		} else {
			w.Header().Add("Content-Type", "text/html")
			err = templates["error"].Execute(w, "sign in first")
		}
		if err != nil {
			h.logger.Sugar().Warnf("checkAuth:: can't parse error %s", err)
		}
		return false
	}

	if sess.SessionID != token {
		w.WriteHeader(http.StatusBadRequest)
		if r.Header["Accept"][0] == "application/json" {
			w.Header().Add("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
		} else {
			w.Header().Add("Content-Type", "text/html")
			err = templates["error"].Execute(w, "access denied")
		}

		if err != nil {
			h.logger.Sugar().Warnf("checkAuth:: can't parse error %s", err)
		}
		return false
	}

	if sess.ValidUntil.Before(time.Now().UTC()) {
		w.WriteHeader(http.StatusUnauthorized)
		if r.Header["Accept"][0] == "application/json" {
			w.Header().Add("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(map[string]string{"error": "authorization time out"})
		} else {
			w.Header().Add("Content-Type", "text/html")
			err = templates["error"].Execute(w, "authorization time out")
		}

		if err != nil {
			h.logger.Sugar().Warnf("checkAuth:: can't parse error %s", err)
		}
		return false
	}
	return true
}

func (h *Handlers) checkAuthByToken(w http.ResponseWriter, r *http.Request) int64 {
	var token string
	var err error
	if len(r.Header["Authorization"]) != 0 {
		token = r.Header["Authorization"][0]
	} else {
		w.WriteHeader(http.StatusBadRequest)
		if r.Header["Accept"][0] == "application/json" {
			w.Header().Add("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(map[string]string{"error": "token required"})
		} else {
			w.Header().Add("Content-Type", "text/html")
			err = templates["error"].Execute(w, "token required")
		}

		if err != nil {
			h.logger.Sugar().Warnf("checkAuthByToken:: can't parse error %s", err)
		}
		return -1
	}

	sess, err := h.ss.GetByBearer(token)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("checkAuthByToken:: can't get session by id %s", err)
		return -1
	}

	if sess == nil {
		w.WriteHeader(http.StatusUnauthorized)
		if r.Header["Accept"][0] == "application/json" {
			w.Header().Add("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(map[string]string{"error": "sign in first"})
		} else {
			w.Header().Add("Content-Type", "text/html")
			err = templates["error"].Execute(w, "sign in first")
		}

		if err != nil {
			h.logger.Sugar().Warnf("checkAuthByToken:: can't parse error %s", err)
		}
		return -1
	}

	if sess.ValidUntil.Before(time.Now().UTC()) {
		w.WriteHeader(http.StatusUnauthorized)
		if r.Header["Accept"][0] == "application/json" {
			w.Header().Add("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(map[string]string{"error": "authorization time out"})
		} else {
			w.Header().Add("Content-Type", "text/html")
			err = templates["error"].Execute(w, "authorization time out")
		}

		if err != nil {
			h.logger.Sugar().Warnf("checkAuthByToken:: can't parse error %s", err)
		}
		return -1
	}
	return sess.UserID
}

func (h *Handlers) checkAuthAndOwner(w http.ResponseWriter, r *http.Request) *robots.Robot {
	ownerID := h.checkAuthByToken(w, r)
	if ownerID < 0 {
		return nil
	}
	robotID, err := h.getID(w, r)
	if err != nil {
		return nil
	}

	robot, err := h.rs.GetByRobotID(robotID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("checkAuthAndOwner:: can't get robot by id %s", err)
		return nil
	}

	if robot == nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		err = json.NewEncoder(w).Encode(map[string]string{"error": "no robot with such id"})
		if err != nil {
			h.logger.Sugar().Warnf("checkAuthAndOwner:: can't parse error %s", err)
		}
		return nil
	}

	if robot.OwnerUserID != ownerID {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		err = json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
		if err != nil {
			h.logger.Sugar().Warnf("checkAuthAndOwner:: can't parse error %s", err)
		}
		return nil
	}

	return robot
}

func (h *Handlers) getID(w http.ResponseWriter, r *http.Request) (int64, error) {
	strID := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(strID, 0, 64)
	if err != nil || id <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		err = json.NewEncoder(w).Encode(map[string]string{"error": "incorrect id"})
		if err != nil {
			h.logger.Sugar().Warnf("getID:: can't parse error msg %s err")
		}
		w.Header().Add("Content-Type", "application/json")
	}
	return id, err
}

func (h *Handlers) UserRobots(w http.ResponseWriter, r *http.Request) {
	id, err := h.getID(w, r)
	if err != nil {
		return
	}

	user, err := h.us.GetByID(id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("UserRobots:: can't get user %s", err)
		return
	}

	if user == nil {
		w.WriteHeader(http.StatusNotFound)
		if r.Header["Accept"][0] == "application/json" {
			w.Header().Add("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(map[string]string{"error": "user not found"})
		} else {
			w.Header().Add("Content-Type", "text/html")
			err = templates["error"].Execute(w, "User not found")
		}
		if err != nil {
			h.logger.Sugar().Warnf("UserRobots:: can't parse error msg %s", err)
		}

		return
	}
	status := h.checkAuth(w, r, id)
	if !status {
		return
	}

	robots, err := h.rs.GetByOwnerID(id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("UserRobots:: can't get robots %s", err)
		return
	}

	if r.Header["Accept"][0] == "application/json" {
		w.Header().Add("Content-Type", "application/json")

		err = json.NewEncoder(w).Encode(robots)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			h.logger.Sugar().Errorf("UserRobots:: can't parse robots(json) %s", err)
			return
		}
	} else {
		w.Header().Add("Content-Type", "text/html")

		tmpl := templates["usersrobots"]
		templrob := TemplRobots{Robots: robots, OwnerID: strconv.FormatInt(id, 10), Token: r.Header["Authorization"][0]}
		err = tmpl.Execute(w, templrob)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			h.logger.Sugar().Errorf("UserRobots:: can't parse robots(html) %s", err)
			return
		}
	}
}

func (h *Handlers) PostRobot(w http.ResponseWriter, r *http.Request) {
	ownerID := h.checkAuthByToken(w, r)
	if ownerID < 0 {
		return
	}

	robot := robots.Robot{}

	err := json.NewDecoder(r.Body).Decode(&robot)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Warnf("PostRobot:: can't parse new robot %s", err)
		return
	}

	robot.OwnerUserID = ownerID

	nextID, err := h.rs.NextID()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("PostRobot:: can't get next id %s", err)
		return
	}

	robot.RobotID = nextID
	timeNow := time.Now().UTC()
	robot.CreatedAt = &timeNow

	err = h.rs.Create(&robot)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Warnf("PostRobot:: can't create robot %s", err)
		return
	}
}

func (h *Handlers) Robots(w http.ResponseWriter, r *http.Request) {
	ownerID := h.checkAuthByToken(w, r)
	if ownerID < 0 {
		return
	}

	_, ok1 := r.URL.Query()["ticker"]
	_, ok2 := r.URL.Query()["user"]

	var ticker, idStr string
	var userID int64
	var err error

	if ok2 {
		idStr = r.URL.Query()["user"][0]
		userID, err = strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			h.logger.Sugar().Errorf("Robots:: can't transfrom to int %s", err)
			return
		}
	}

	if ok1 {
		ticker = r.URL.Query()["ticker"][0]
	}

	var robos []robots.Robot
	if ok1 || ok2 {
		robos, err = h.rs.GetByTickerAndOwnerID(ticker, userID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			h.logger.Sugar().Errorf("Robots:: can't get all robots %s", err)
			return
		}
	} else {
		robos, err = h.rs.GetAllRobots()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			h.logger.Sugar().Errorf("Robots:: can't get all robots %s", err)
			return
		}
	}

	if r.Header["Accept"][0] == "application/json" {
		w.Header().Add("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(robos)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			h.logger.Sugar().Errorf("UserRobots:: can't parse robots(json) %s", err)
			return
		}
	} else {
		w.Header().Add("Content-Type", "text/html")

		tmpl := templates["robots"]
		templrob := TemplRobots{
			Robots:  robos,
			Ticker:  ticker,
			OwnerID: idStr,
			Token:   r.Header["Authorization"][0],
		}

		err = tmpl.Execute(w, templrob)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			h.logger.Sugar().Errorf("UserRobots:: can't parse robots(html) %s", err)
			return
		}
	}
}

func (h *Handlers) RobotWithID(w http.ResponseWriter, r *http.Request) {
	userID := h.checkAuthByToken(w, r)
	if userID < 0 {
		return
	}

	robotID, err := h.getID(w, r)
	if err != nil {
		return
	}

	robot, err := h.rs.GetByRobotID(robotID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("RobotWithID:: can't get robot by id %s", err)
		return
	}

	if robot == nil {
		w.WriteHeader(http.StatusNotFound)
		if r.Header["Accept"][0] == "application/json" {
			w.Header().Add("Content-Type", "application/json")
			err = json.NewEncoder(w).Encode(map[string]string{"error": "no robot with such id"})
		} else {
			w.Header().Add("Content-Type", "text/html")
			err = templates["error"].Execute(w, "no robot with such id")
		}

		if err != nil {
			h.logger.Sugar().Warnf("RobotWithID:: can't parse error msg %s", err)
		}
		return
	}

	if r.Header["Accept"][0] == "application/json" {
		w.Header().Add("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(robot)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			h.logger.Sugar().Errorf("UserRobots:: can't parse robots(json) %s", err)
			return
		}
	} else {
		w.Header().Add("Content-Type", "text/html")

		tmpl := templates["robot"]

		err := tmpl.Execute(w, robot)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			h.logger.Sugar().Errorf("UserRobots:: can't parse robots(html) %s", err)
			return
		}
	}
}

func (h *Handlers) DeleteRobot(w http.ResponseWriter, r *http.Request) {
	robot := h.checkAuthAndOwner(w, r)
	if robot == nil {
		return
	}

	if robot.DeletedAt != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		err := json.NewEncoder(w).Encode(map[string]string{"error": "robot is already deleted"})
		if err != nil {
			h.logger.Sugar().Warnf("DeleteRobot:: can't parse error msg %s", err)
		}
		return
	}

	if robot.PlanStart != nil && robot.PlanEnd != nil {
		if robot.PlanStart.Before(time.Now().UTC()) && robot.PlanEnd.After(time.Now().UTC()) {
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			err := json.NewEncoder(w).Encode(map[string]string{"error": "can't delete now"})
			if err != nil {
				h.logger.Sugar().Warnf("DeleteRobot:: can't parse error msg %s", err)
			}
			return
		}
	}

	err := h.rs.DeleteRobot(robot)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("DeleteRobot:: can't delete robot %s", err)
	}
}

func (h *Handlers) UpdateRobot(w http.ResponseWriter, r *http.Request) {
	robot := h.checkAuthAndOwner(w, r)
	if robot == nil {
		return
	}

	err := json.NewDecoder(r.Body).Decode(robot)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("UpdateRobot:: can't decode robot from request %s", err)
		return
	}

	//we suggest that put doesn't change robot_id and owner_id
	err = h.rs.UpdateRobot(robot)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("UpdateRobot:: can't update robot %s", err)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(robot)
	if err != nil {
		h.logger.Sugar().Warnf("UpdateRobot:: can't parse new robot  %s", err)
	}
}

func (h *Handlers) ActivateRobot(w http.ResponseWriter, r *http.Request) {
	robot := h.checkAuthAndOwner(w, r)
	if robot == nil {
		return
	}

	if robot.DeletedAt != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		err := json.NewEncoder(w).Encode(map[string]string{"error": "can't activate deleted robot"})
		if err != nil {
			h.logger.Sugar().Warnf("ActivateRobot:: can't parse error msg %s", err)
		}
		return
	}

	if robot.IsActive {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		err := json.NewEncoder(w).Encode(map[string]string{"error": "can't activate activated robot"})
		if err != nil {
			h.logger.Sugar().Warnf("ActivateRobot:: can't parse error msg %s", err)
		}
		return
	}

	if robot.PlanStart != nil && robot.PlanEnd != nil {
		if robot.PlanStart.Before(time.Now().UTC()) && robot.PlanEnd.After(time.Now().UTC()) {
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			err := json.NewEncoder(w).Encode(map[string]string{"error": "can't activate now"})
			if err != nil {
				h.logger.Sugar().Warnf("ActivateRobot:: can't parse error msg %s", err)
			}
			return
		}
	}

	err := h.rs.ActivateRobot(robot)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("ActivateRobot:: can't activate robot %s", err)
	}

	w.Header().Add("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(robot)
	if err != nil {
		h.logger.Sugar().Warnf("ActivateRobot:: can't parse new robot  %s", err)
	}
}

func (h *Handlers) DeactivateRobot(w http.ResponseWriter, r *http.Request) {
	robot := h.checkAuthAndOwner(w, r)
	if robot == nil {
		return
	}

	if robot.DeletedAt != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		err := json.NewEncoder(w).Encode(map[string]string{"error": "can't deactivate deleted robot"})
		if err != nil {
			h.logger.Sugar().Warnf("DeactivateRobot:: can't parse error msg %s", err)
		}
		return
	}
	if !robot.IsActive {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		err := json.NewEncoder(w).Encode(map[string]string{"error": "can't deactivate deactivated robot"})
		if err != nil {
			h.logger.Sugar().Warnf("DeactivateRobot:: can't parse error msg %s", err)
		}
		return
	}

	if robot.PlanStart != nil && robot.PlanEnd != nil {
		if robot.PlanStart.Before(time.Now().UTC()) && robot.PlanEnd.After(time.Now().UTC()) {
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			err := json.NewEncoder(w).Encode(map[string]string{"error": "can't deactivate now"})
			if err != nil {
				h.logger.Sugar().Warnf("DeactivateRobot:: can't parse error msg %s", err)
			}
			return
		}
	}

	err := h.rs.DeactivateRobot(robot)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("DeactivateRobot:: can't delete robot %s", err)
	}

	w.Header().Add("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(robot)
	if err != nil {
		h.logger.Sugar().Warnf("DeactivateRobot:: can't parse new robot  %s", err)
	}
}

func (h *Handlers) FavourRobot(w http.ResponseWriter, r *http.Request) {
	ownerID := h.checkAuthByToken(w, r)
	if ownerID < 0 {
		return
	}
	robotID, err := h.getID(w, r)
	if err != nil {
		return
	}

	robot, err := h.rs.GetByRobotID(robotID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("FavourRobot:: can't get robot by id %s", err)
		return
	}

	if robot == nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		err = json.NewEncoder(w).Encode(map[string]string{"error": "no robot with such id"})
		if err != nil {
			h.logger.Sugar().Warnf("FavourRobot:: can't parse error %s", err)
		}
		return
	}

	timeNow := time.Now().UTC()
	robot.OwnerUserID = ownerID
	robot.ParentRobotID = robot.RobotID
	robot.IsFavourite = true
	robot.IsActive = false
	robot.FactYield = 0
	robot.DealsCount = 0
	robot.CreatedAt = &timeNow
	robot.DeletedAt = nil
	robot.ActivatedAt = nil
	robot.DeactivatedAt = nil

	nextID, err := h.rs.NextID()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("FavourRobot:: can't get next id %s", err)
		return
	}

	robot.RobotID = nextID

	err = h.rs.Create(robot)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		h.logger.Sugar().Errorf("FavourRobot:: can't create copy %s", err)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(robot)
	if err != nil {
		h.logger.Sugar().Warnf("FavourRobot:: can't parse new robot  %s", err)
	}
}
