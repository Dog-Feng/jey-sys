package domain

type Side string

const (
	SideLong  Side = "long"
	SideShort Side = "short"
)

type Action string

const (
	ActionBuy  Action = "buy"
	ActionSell Action = "sell"
	ActionHold Action = "hold"
)

type Book struct {
	Symbol    string
	BestBid   float64
	BestAsk   float64
	Mid       float64
	SpreadBps float64
	Imbalance float64
	TickSize  float64
}

type Position struct {
	SizeBTC     float64 // signed: long > 0, short < 0
	EntryPrice  float64
	Unrealized  float64
}

func (p Position) Side() string {
	if p.SizeBTC > 1e-12 {
		return "long"
	}
	if p.SizeBTC < -1e-12 {
		return "short"
	}
	return "flat"
}

func (p Position) Abs() float64 {
	if p.SizeBTC < 0 {
		return -p.SizeBTC
	}
	return p.SizeBTC
}

type Allowed struct {
	IncreaseLong  bool
	IncreaseShort bool
	ReduceLong    bool
	ReduceShort   bool
}

type Decision struct {
	Action        Action
	Probabilities map[Action]float64
	Confidence    float64
	LatencyMs     int64
	Late          bool
}

type OrderIntent struct {
	Side        Side
	ReduceOnly  bool
	Price       float64
	SizeBTC     float64
	Skip        bool
	Capped      bool
	SkipReason  string
}

type OrderResult struct {
	ClientOrderIndex int64
	Status           string // placed | sim | skipped | error
	Error            string
}

type AccountSnapshot struct {
	Position Position
	Allowed  Allowed
}

type TradeState struct {
	Symbol        string
	TickID        int64
	HorizonTicks  int
	TickInterval  string
	Mid           float64
	SpreadBps     float64
	BookImbalance float64
	ReturnsBps    map[string]float64
	RecentMids    string
	Position      Position
	Allowed       Allowed
	DualAccount   bool      `json:"dual_account,omitempty"`
	PositionLegA  Position  `json:"position_leg_a,omitempty"`
	PositionLegB  Position  `json:"position_leg_b,omitempty"`
	Phase         string    `json:"phase,omitempty"`
}

type TickEvent struct {
	TickID    int64
	TsMs      int64
	Symbol    string
	Mid       float64
	BestBid   float64
	BestAsk   float64
	SpreadBps float64
	Decision  *Decision
	Intent    OrderIntent
	Order     OrderResult
	Position  Position
	Late      bool
	Phase     string `json:"phase,omitempty"`
}
