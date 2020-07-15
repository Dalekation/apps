package buyingservice

import (
	context "context"
	ft "finPrj/internal/fintech"
	pg "finPrj/internal/postgres"
	"finPrj/internal/robots"
	"io"
	sync "sync"
	"time"

	"go.uber.org/zap"
	grpc "google.golang.org/grpc"
)

type RoboTrader struct {
	IsBuying bool //false if is_selling
	Robot    *robots.Robot
}

func (rt *RoboTrader) Buy(price float64, rs *pg.RobotStorage) error {
	rt.IsBuying = false
	rt.Robot.FactYield -= price
	err := rs.UpdateRobot(rt.Robot)
	return err
}

func (rt *RoboTrader) Sell(price float64, rs *pg.RobotStorage) error {
	rt.IsBuying = true
	rt.Robot.FactYield += price
	rt.Robot.DealsCount++
	err := rs.UpdateRobot(rt.Robot)
	return err
}

type BuyingService struct {
	logger *zap.Logger
	rs     *pg.RobotStorage
	robots map[string]map[int64]*RoboTrader
	mutex  sync.Mutex
	conn   *grpc.ClientConn
}

func NewBuyingService(logger *zap.Logger, rs *pg.RobotStorage, conn *grpc.ClientConn) *BuyingService {
	return &BuyingService{
		logger: logger,
		rs:     rs,
		mutex:  sync.Mutex{},
		conn:   conn,
		robots: make(map[string]map[int64]*RoboTrader),
	}
}

func (wr *BuyingService) ActivateNewRobots(ctx context.Context) {
	sleeper := time.Tick(3 * time.Second)
	go func() {
		for {
			select {
			case <-sleeper:
				robos, err := wr.rs.RobotsToRun()
				if err != nil {
					wr.logger.Sugar().Errorf("ActivateRobots:: can't activate robots %s", err)
					return
				}

				wr.mutex.Lock()

				for _, robot := range robos {
					if len(wr.robots[robot.Ticker]) == 0 {
						wr.robots[robot.Ticker] = make(map[int64]*RoboTrader)
						go wr.ListenPrices(ctx, robot.Ticker)
					}

					if wr.robots[robot.Ticker][robot.RobotID] == nil {
						rt := RoboTrader{
							IsBuying: true,
							Robot:    &robot,
						}
						wr.robots[robot.Ticker][robot.RobotID] = &rt
					} else {
						wr.robots[robot.Ticker][robot.RobotID].Robot = &robot
					}

				}
				wr.mutex.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (wr *BuyingService) ListenPrices(ctx context.Context, ticker string) {
	client := ft.NewTradingServiceClient(wr.conn)

	stream, err := client.Price(ctx, &ft.PriceRequest{Ticker: ticker})
	if err != nil {
		wr.logger.Sugar().Errorf("ListenPrices:: can't start listen to %s %s", ticker, err)
		return
	}

	for {
		price, err := stream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			wr.logger.Sugar().Errorf("ListenPrices:: can't recv from stream %s %s", ticker, err)
			return
		}

		inactiveRobots := []int64{}

		for id, rs := range wr.robots[ticker] {
			if rs.IsBuying {
				if rs.Robot.BuyPrice >= price.GetBuyPrice() {
					err = rs.Buy(price.GetBuyPrice(), wr.rs)
					if err != nil {
						inactiveRobots = append(inactiveRobots, id)
					}
				}
			} else {
				if rs.Robot.SellPrice <= price.GetSellPrice() {
					err = rs.Sell(price.GetSellPrice(), wr.rs)
					if err != nil {
						inactiveRobots = append(inactiveRobots, id)
					}
				}
			}
			if rs.Robot.PlanEnd != nil {
				if rs.Robot.PlanEnd.Before(time.Now().UTC()) {
					inactiveRobots = append(inactiveRobots, id)
				}
			} else {
				if !rs.Robot.IsActive {
					inactiveRobots = append(inactiveRobots, id)
				}
			}

		}

		wr.DeleteRoboTraders(ticker, inactiveRobots...)
	}
}

func (wr *BuyingService) DeleteRoboTraders(ticker string, inactiveUsers ...int64) {
	wr.mutex.Lock()
	defer wr.mutex.Unlock()
	for _, num := range inactiveUsers {
		delete(wr.robots[ticker], num)
	}
}
