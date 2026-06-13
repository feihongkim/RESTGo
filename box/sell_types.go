package box

// RecoveryPotential 은 회복 가능성 분류 (Composite Path 임계값 결정용)
type RecoveryPotential int

const (
	RecoveryHigh RecoveryPotential = iota
	RecoveryMedium
	RecoveryLow
)

// SellPath 은 SellDecisionEngine 5-Path 분류
type SellPath string

const (
	PathCritical   SellPath = "critical"
	PathComposite  SellPath = "composite"
	PathExtension  SellPath = "extension"
	PathExpiry     SellPath = "expiry"
	PathIndividual SellPath = "individual"
)

// SellSignal 은 개별 매도 신호 평가 결과 (C# vo/SellSignal.cs 포팅)
type SellSignal struct {
	ConditionName     string
	Path              SellPath
	Priority          int
	SignalWeight      float64 // 부분 매도 weight (0.25/0.5/1.0)
	IsTriggered       bool    // 조건 함수가 true 반환했는지
	ThresholdMet      bool    // TrackAndCheck 임계값 충족
	OccurrenceCount   int     // 누적 발생 횟수
	OccurrenceRatio   float64 // 보유기간 대비 발생 비율
	Category          string  // "Critical"/"Profit"/"Loss"/"Technical"/"Expiry"/"Extension"
	CompositeEligible bool    // Composite Path에서 합산 대상인지
	CompositeWeight   float64 // Composite 합산 시 사용하는 가중치
}

// CanTriggerSell 은 신호가 실제 매도를 트리거할 수 있는지 (Triggered AND ThresholdMet)
func (s *SellSignal) CanTriggerSell() bool {
	return s.IsTriggered && s.ThresholdMet
}

// SellSignalCollection 은 한 포지션 평가 결과 신호 집합 (C# vo/SellSignalCollection.cs 포팅)
type SellSignalCollection struct {
	Position                          *TradePosition
	AllSignals                        []SellSignal
	TriggeredSignals                  []SellSignal // CanTriggerSell == true, Priority 오름차순
	Recovery                          RecoveryPotential
	CompositeStrength                 float64 // composite_eligible 신호들의 weight 합
	IsPeriodExpired                   bool
	IsWaitingForSellSignalAfterExpiry bool
}

// SellDecision 은 5-Path 결정 결과 (C# vo/SellDecision.cs 포팅)
type SellDecision struct {
	ShouldSell                     bool
	SellWeight                     float64 // 0.0 ~ 1.0
	PrimaryReason                  string  // 주요 매도 사유 (조건명)
	DecisionPath                   string  // "Phase2-Critical" 등
	ContributingSignals            []string
	DecisionRationale              string
	RequiresHoldingExtensionUpdate bool // PeriodExpiry 후속 처리 필요 표시
	ShouldEnableHoldingExtension   bool
}

// NoSellDecision 은 매도하지 않음 결정 (C# SellDecision.NoSell 포팅)
func NoSellDecision(reason string) SellDecision {
	return SellDecision{
		ShouldSell:        false,
		DecisionRationale: reason,
	}
}

// SellExecution 은 개별 매도 실행 기록 (C# vo/SellExecution.cs 포팅)
type SellExecution struct {
	ExecutionOrder     int     `json:"executionOrder"`
	SellReason         string  `json:"sellReason"`
	SellQuantity       float64 `json:"sellQuantity"`
	RemainingAfterSell float64 `json:"remainingAfterSell"`
	Weight             float64 `json:"weight"`
	SellPrice          float64 `json:"sellPrice"`
	SellPriceOrigin    float64 `json:"sellPrice_origin"`
	SellDate           string  `json:"sellDate"`
	SellPosition       int     `json:"sellPosition"`
	PartialReturnRate  float64 `json:"partialReturnRate"`
	HoldingDays        int     `json:"holdingDays"`
	ExecutionTime      string  `json:"executionTime"`
	SellCost           float64 `json:"sellCost"`          // 청산 비용 (SellPrice * (FeeRate + SlippageRate))
	NetPartialReturn   float64 `json:"netPartialReturn"`  // 비용 차감 후 부분 수익률
}

// Category 는 SellReason으로부터 카테고리 자동 추론 (C# SellExecution.Category 포팅)
func (e *SellExecution) Category() string {
	r := e.SellReason
	if containsAny(r, []string{"Profit"}) {
		return "ProfitTaking"
	}
	if containsAny(r, []string{"StopLoss", "Fail", "Breakdown", "DeadCross"}) {
		return "LossCutting"
	}
	if containsAny(r, []string{"Period"}) {
		return "AutoLiquidation"
	}
	return "Technical"
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	n, m := len(s), len(sub)
	if m == 0 {
		return 0
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return i
		}
	}
	return -1
}

// TradePosition 은 개별 매수 포지션 정보 + 매도 추적 상태 (C# vo/TradePosition.cs 포팅)
type TradePosition struct {
	// === 식별 ===
	TradeId       string
	StrategyName  string
	BuySignalType string

	// === 매수 정보 ===
	BuyPosition      int
	BuyPrice         float64
	BuyPriceOrigin   float64
	BuyDate          string
	BuyTime          string
	MainBoxIndex     int
	DefBoxIndex      int
	DefBoxPrice      float64
	MainBoxPrice     float64
	MomentumPosition int
	PenPosition      int
	MainboxPosition  int

	// === 매도 정보 (청산 시 기록) ===
	SellPosition    int
	SellPrice       float64
	SellPriceOrigin float64
	SellDate        string
	SellReason      string
	ReturnRate      float64

	// === 비용 모델 (수수료 + 슬리피지) ===
	FeeRate       float64 // 수수료율 (예: 0.0005)
	SlippageRate  float64 // 슬리피지율 (예: 0.0005)
	BuyCost       float64 // 매수 왕복 비용 (BuyPriceOrigin * (FeeRate + SlippageRate))
	NetReturnRate float64 // 비용 차감 후 수익률

	// === 활성 상태 ===
	IsActive bool

	// === Partial Sell 추적 ===
	InitialQuantity   float64
	RemainingQuantity float64
	SellExecutions    []SellExecution

	// === 매도 조건별 발생 이력 (TrackAndCheckCondition) ===
	// Key: 조건명, Value: 조건이 true가 됐던 Position 목록
	SellConditionHistory map[string][]int

	// === Holding Extension 상태 ===
	IsWaitingForSellSignalAfterExpiry bool
	PeriodExpiredAtPosition           int
}

// NewTradePosition 은 매수 신호 발생 시 새 포지션을 생성
func NewTradePosition(tradeId, strategyName string, buyPos int, buyPrice, buyPriceOrigin float64, buyDate string) *TradePosition {
	return &TradePosition{
		TradeId:                 tradeId,
		StrategyName:            strategyName,
		BuyPosition:             buyPos,
		BuyPrice:                buyPrice,
		BuyPriceOrigin:          buyPriceOrigin,
		BuyDate:                 buyDate,
		IsActive:                true,
		InitialQuantity:         1.0,
		RemainingQuantity:       1.0,
		SellPosition:            -1,
		PeriodExpiredAtPosition: -1,
		SellExecutions:          []SellExecution{},
		SellConditionHistory:    map[string][]int{},
	}
}

// IsFullyLiquidated 는 잔여 수량이 사실상 0인지 (≤ 0.0001)
func (p *TradePosition) IsFullyLiquidated() bool {
	return p.RemainingQuantity <= 0.0001
}

// HoldingDays 는 현재 시점 기준 보유 캔들 수
func (p *TradePosition) HoldingDays(currentPosition int) int {
	return currentPosition - p.BuyPosition
}

// RecordSellConditionOccurrence 는 조건 발생 이력을 기록 (같은 position 중복 방지)
// 반환값: 누적 발생 횟수
// C# SFunction.Helpers.TrackAndCheckCondition 와 동일하게 전체 리스트 Contains 검사를 사용한다.
func (p *TradePosition) RecordSellConditionOccurrence(conditionName string, position int) int {
	if p.SellConditionHistory == nil {
		p.SellConditionHistory = map[string][]int{}
	}
	hist := p.SellConditionHistory[conditionName]
	for _, recorded := range hist {
		if recorded == position {
			return len(hist)
		}
	}
	hist = append(hist, position)
	p.SellConditionHistory[conditionName] = hist
	return len(hist)
}

// GetSellConditionOccurrenceCount 는 누적 발생 횟수 조회
func (p *TradePosition) GetSellConditionOccurrenceCount(conditionName string) int {
	if p.SellConditionHistory == nil {
		return 0
	}
	return len(p.SellConditionHistory[conditionName])
}
