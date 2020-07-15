package postgres

import (
	"database/sql"
	"time"

	robots "finPrj/internal/robots"

	"github.com/pkg/errors"
)

var _ robots.Storage = &RobotStorage{}

type RobotStorage struct {
	statementStorage

	CreateRobotStmt           *sql.Stmt
	GetByTickerAndOwnerIDStmt *sql.Stmt
	GetByTickerStmt           *sql.Stmt
	GetByOwnerIDStmt          *sql.Stmt
	GetByRobotIDStmt          *sql.Stmt
	GetAllRobotsStmt          *sql.Stmt
	UpdateRobotStmt           *sql.Stmt
	ActivateRobotStmt         *sql.Stmt
	DeactivateRobotStmt       *sql.Stmt
	NextIDStmt                *sql.Stmt
	DeleteStmt                *sql.Stmt
	RobotsToRunStmt           *sql.Stmt

	roboUpd chan<- *robots.Robot
}

func NewRobotStorage(db *DB, roboUpd chan<- *robots.Robot) (*RobotStorage, error) {
	rs := &RobotStorage{statementStorage: newStatementsStorage(db), roboUpd: roboUpd}

	stmts := []stmt{
		{Query: createRobotQuery, Dst: &rs.CreateRobotStmt},
		{Query: getByTickerAndOwnerIDQuery, Dst: &rs.GetByTickerAndOwnerIDStmt},
		{Query: getByTickerQuery, Dst: &rs.GetByTickerStmt},
		{Query: getByOwnerIDQuery, Dst: &rs.GetByOwnerIDStmt},
		{Query: getByRobotIDQuery, Dst: &rs.GetByRobotIDStmt},
		{Query: getAllRobotsQuery, Dst: &rs.GetAllRobotsStmt},
		{Query: updateRobotQuery, Dst: &rs.UpdateRobotStmt},
		{Query: activateRobotQuery, Dst: &rs.ActivateRobotStmt},
		{Query: deactivateRobotQuery, Dst: &rs.DeactivateRobotStmt},
		{Query: nextIDQuery, Dst: &rs.NextIDStmt},
		{Query: deleteQuery, Dst: &rs.DeleteStmt},
		{Query: robotsToRunQuery, Dst: &rs.RobotsToRunStmt},
	}

	if err := rs.initStatements(stmts); err != nil {
		return nil, errors.Wrap(err, "can't init statements in users")
	}

	return rs, nil
}

const createRobotQuery = `INSERT INTO robots (robot_id, owner_user_id, is_favourite,
is_active, parent_robot_id, ticker, buy_price, sell_price, plan_start,
plan_end, plan_yield, fact_yield, deals_counts, activated_at,
deactivated_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`

func (rs *RobotStorage) Create(robo *robots.Robot) error {
	_, err := rs.CreateRobotStmt.Exec(robo.RobotID, robo.OwnerUserID, robo.IsFavourite,
		robo.IsActive, robo.ParentRobotID, robo.Ticker, robo.BuyPrice, robo.SellPrice, robo.PlanStart,
		robo.PlanEnd, robo.PlanYield, robo.FactYield, robo.DealsCount,
		robo.ActivatedAt, robo.DeactivatedAt, robo.CreatedAt)
	if err != nil {
		return errors.Wrapf(err, "can't create new robot")
	}

	rs.roboUpd <- robo

	return nil
}

const getByTickerAndOwnerIDQuery = `SELECT robot_id, owner_user_id, is_favourite,
is_active, parent_robot_id, ticker, buy_price, sell_price, plan_start,
plan_end, plan_yield, fact_yield, deals_counts, deleted_at,
activated_at, deactivated_at, created_at FROM robots WHERE owner_user_id = $1 AND ticker = $2`

const getByTickerQuery = `SELECT robot_id, owner_user_id, is_favourite,
is_active, parent_robot_id, ticker, buy_price, sell_price, plan_start,
plan_end, plan_yield, fact_yield, deals_counts, deleted_at,
activated_at, deactivated_at, created_at FROM robots WHERE ticker = $1`

//we expect that one of ticker or id is not zero value
//in other case you should use GetAllRobots
func (rs *RobotStorage) GetByTickerAndOwnerID(ticker string, ownerID int64) ([]robots.Robot, error) {
	if ticker == "" {
		rows, err := rs.GetByOwnerIDStmt.Query(ownerID)
		if err != nil {
			return nil, errors.Wrapf(err, "can't get by tick&ownID")
		}
		return scanRobots(rows, "by tick&ownID")
	}

	if ownerID == 0 {
		rows, err := rs.GetByTickerStmt.Query(ticker)
		if err != nil {
			return nil, errors.Wrapf(err, "can't get by tick&ownID")
		}
		return scanRobots(rows, "by tick&ownID")
	}

	rows, err := rs.GetByTickerAndOwnerIDStmt.Query(ownerID, ticker)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get by tick&ownID")
	}
	return scanRobots(rows, "by tick&ownID")

}

const getByOwnerIDQuery = `SELECT robot_id, owner_user_id, is_favourite,
is_active, parent_robot_id, ticker, buy_price, sell_price, plan_start,
plan_end, plan_yield, fact_yield, deals_counts, deleted_at,
activated_at, deactivated_at, created_at FROM robots WHERE owner_user_id = $1`

func (rs *RobotStorage) GetByOwnerID(ownerID int64) ([]robots.Robot, error) {
	rows, err := rs.GetByOwnerIDStmt.Query(ownerID)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get by ownerID")
	}

	return scanRobots(rows, "by ownerID")
}

const getByRobotIDQuery = `SELECT robot_id, owner_user_id, is_favourite,
is_active, parent_robot_id, ticker, buy_price, sell_price, plan_start,
plan_end, plan_yield, fact_yield, deals_counts, deleted_at,
activated_at, deactivated_at, created_at FROM robots WHERE robot_id = $1`

func (rs *RobotStorage) GetByRobotID(roboID int64) (*robots.Robot, error) {
	row := rs.GetByRobotIDStmt.QueryRow(roboID)

	robot := robots.Robot{}

	err := scanRobot(row, &robot)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}

		return nil, errors.Wrapf(err, "can't get robot by id")
	}

	return &robot, nil
}

const getAllRobotsQuery = `SELECT robot_id, owner_user_id, is_favourite,
is_active, parent_robot_id, ticker, buy_price, sell_price, plan_start,
plan_end, plan_yield, fact_yield, deals_counts, deleted_at,
activated_at, deactivated_at, created_at FROM robots`

func (rs *RobotStorage) GetAllRobots() ([]robots.Robot, error) {
	rows, err := rs.GetAllRobotsStmt.Query()
	if err != nil {
		return nil, errors.Wrapf(err, "can't get all robots")
	}

	return scanRobots(rows, "all robots")
}

const updateRobotQuery = `UPDATE robots SET owner_user_id=$1, is_favourite=$2,
is_active=$3, parent_robot_id=$4, ticker=$5, buy_price=$6, sell_price=$7, plan_start=$8,
plan_end=$9, plan_yield=$10, fact_yield=$11, deals_counts=$12,
activated_at=$13, deactivated_at=$14, created_at=$15 WHERE robot_id = $16`

//expects that all field are filled with current data
func (rs *RobotStorage) UpdateRobot(robo *robots.Robot) error {
	_, err := rs.UpdateRobotStmt.Exec(robo.OwnerUserID, robo.IsFavourite,
		robo.IsActive, robo.ParentRobotID, robo.Ticker, robo.BuyPrice, robo.SellPrice, robo.PlanStart,
		robo.PlanEnd, robo.PlanYield, robo.FactYield, robo.DealsCount,
		robo.ActivatedAt, robo.DeactivatedAt, robo.CreatedAt, robo.RobotID)
	if err != nil {
		return errors.Wrapf(err, "can't update robot")
	}

	rs.roboUpd <- robo

	return nil
}

const activateRobotQuery = `UPDATE robots SET is_active=TRUE, activated_at=$1 WHERE robot_id = $2`

func (rs *RobotStorage) ActivateRobot(robo *robots.Robot) error {
	timeNow := time.Now().UTC()
	robo.ActivatedAt = &timeNow
	robo.IsActive = true
	_, err := rs.ActivateRobotStmt.Exec(robo.ActivatedAt, robo.RobotID)
	if err != nil {
		return errors.Wrapf(err, "can't activate robot %d", robo.RobotID)
	}

	rs.roboUpd <- robo

	return nil
}

const deactivateRobotQuery = `UPDATE robots SET is_active=FALSE, deactivated_at=$1 WHERE robot_id = $2`

func (rs *RobotStorage) DeactivateRobot(robo *robots.Robot) error {
	timeNow := time.Now().UTC()
	robo.DeactivatedAt = &timeNow
	robo.IsActive = false
	_, err := rs.DeactivateRobotStmt.Exec(robo.DeactivatedAt, robo.RobotID)
	if err != nil {
		return errors.Wrapf(err, "can't deactivate robot %d", robo.RobotID)
	}

	rs.roboUpd <- robo

	return nil
}

const nextIDQuery = `SELECT MAX(robot_id) FROM robots`

func (rs *RobotStorage) NextID() (int64, error) {
	row := rs.NextIDStmt.QueryRow()
	var nextID sql.NullInt64
	err := row.Scan(&nextID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 1, nil
		}

		return 0, errors.Wrapf(err, "can't get next robot id")
	}

	return nextID.Int64 + 1, nil
}

//can be used for both deleting and recovering robot
const deleteQuery = `UPDATE robots SET deleted_at = $1 WHERE robot_id = $2`

func (rs *RobotStorage) DeleteRobot(robo *robots.Robot) error {
	timeNow := time.Now().UTC()
	robo.DeletedAt = &timeNow
	_, err := rs.DeleteStmt.Exec(robo.DeletedAt, robo.RobotID)
	if err != nil {
		return errors.Wrapf(err, "can't delete robot %d", robo.RobotID)
	}

	rs.roboUpd <- robo

	return nil
}

const robotsToRunQuery = `SELECT robot_id, owner_user_id, is_favourite,
is_active, parent_robot_id, ticker, buy_price, sell_price, plan_start,
plan_end, plan_yield, fact_yield, deals_counts, deleted_at,
activated_at, deactivated_at, created_at FROM robots 
WHERE (deleted_at is NULL) and ((plan_start < $1) and ($1 < plan_end) or (is_active = true)) `

func (rs *RobotStorage) RobotsToRun() ([]robots.Robot, error) {
	timeNow := time.Now().UTC()
	rows, err := rs.RobotsToRunStmt.Query(timeNow)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get robots to run")
	}

	return scanRobots(rows, "robots to run")
}

func scanRobot(scanner sqlScanner, robo *robots.Robot) error {
	err := scanner.Scan(&robo.RobotID, &robo.OwnerUserID, &robo.IsFavourite,
		&robo.IsActive, &robo.ParentRobotID, &robo.Ticker, &robo.BuyPrice, &robo.SellPrice, &robo.PlanStart,
		&robo.PlanEnd, &robo.PlanYield, &robo.FactYield, &robo.DealsCount, &robo.DeletedAt,
		&robo.ActivatedAt, &robo.DeactivatedAt, &robo.CreatedAt)

	return err
}

func scanRobots(multiScanner sqlMultiScanner, msg string) ([]robots.Robot, error) {
	robot := robots.Robot{}
	robotsList := make([]robots.Robot, 0)
	for multiScanner.Next() {
		err := scanRobot(multiScanner, &robot)
		if err != nil {
			return nil, errors.Wrapf(err, "can't scan"+msg)
		}

		robotsList = append(robotsList, robot)
	}

	return robotsList, nil
}
