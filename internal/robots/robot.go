package robots

import (
	"time"
)

type Robot struct {
	RobotID       int64      `json:"robot_id"`
	OwnerUserID   int64      `json:"owner_user_id"`
	ParentRobotID int64      `json:"parent_robot_id"`
	IsFavourite   bool       `json:"is_favorite"`
	IsActive      bool       `json:"is_active"`
	Ticker        string     `json:"ticker,omitempty"`
	BuyPrice      float64    `json:"buy_price"`
	SellPrice     float64    `json:"sell_price,omitempty"`
	PlanStart     *time.Time `json:"plan_start,omitempty"`
	PlanEnd       *time.Time `json:"plan_end,omitempty"`
	PlanYield     float64    `json:"plan_yield"`
	FactYield     float64    `json:"fact_yield"`
	DealsCount    int64      `json:"deals_counts"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
	ActivatedAt   *time.Time `json:"activated_at,omitempty"`
	DeactivatedAt *time.Time `json:"deactivated_at,omitempty"`
	CreatedAt     *time.Time `json:"created_at"`
}

type Storage interface {
	Create(robo *Robot) error
	GetByTickerAndOwnerID(ticker string, ownerID int64) ([]Robot, error)
	GetByOwnerID(ownerID int64) ([]Robot, error)
	GetByRobotID(roboID int64) (*Robot, error)
	GetAllRobots() ([]Robot, error)
	UpdateRobot(robo *Robot) error
	ActivateRobot(robo *Robot) error
	DeactivateRobot(robo *Robot) error
	NextID() (int64, error)
	DeleteRobot(robo *Robot) error
}
