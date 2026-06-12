package box

// TradingContext 는 분석 루프 전체에서 공유되는 상태 관리 구조체
type TradingContext struct {
	// 종목 정보
	Shcode string
	Hname  string

	// 캔들/박스 데이터
	CandleList []*Candle
	BoxList    []*Box

	// 현재 분석 위치 (캔들 인덱스)
	Position int

	// 박스 분석 시작 위치 (Curve 상태 변경 시 업데이트)
	Exposition int

	// DefBox 위치 정보
	DefboxPosition  int     // DefBox의 캔들 위치
	DefboxIndex     int     // BoxList 내 DefBox의 인덱스
	DefboxPrice     float64 // DefBox 가격
	MainboxPosition int     // DefBox와 연결된 MainBox의 캔들 위치
	DefCount        int     // DefBox에 연결된 MainBox 개수

	// 상태 플래그
	DefChecker int // DefBox 생성 카운터 (0이면 DefBox 없음)

	// DamChecker: 0=DefBox없음 1=새DefBox감지(돌파대기) 2=DefBox돌파완료(매수조건평가)
	DamChecker int

	// 매수 신호 상태
	Bsig    string
	StgName string

	// FollowUp 매수 상태 (C# TradingContext.PenPosition/MomentumPosition +
	// VirtualTrading.BuyHelper/BuyHelperReport/SellHelper/BuyOn 포팅)
	PenPosition      int    // 마지막 전략 발화 캔들 위치 (재진입 타이밍 기준점)
	MomentumPosition int    // 마지막 신호 모멘텀 위치
	BuyHelper        string // 매수 헬퍼 상태 ("매수대기"/"multidef매수대기"/"!multidef매수대기"는 기록 제외)
	BuyHelperReport  string // 매수 헬퍼 리포트 (제외 없이 기록)
	SellHelper       string // 마지막 완전 청산 포지션의 SellReason (C# VirtualTrading.SellHelper)
	BuyOn            bool   // 실제 매수 실행 여부 (후보군1 신호는 제외)

	// 전략별 마지막 매수 신호 위치 (중복 신호 방지)
	LastBuySignalPosition map[string]int

	// 활성 매수 포지션 리스트 (매수 신호 발생 시 추가, 완전 청산 시 IsActive=false)
	// 매도 평가는 각 캔들마다 이 리스트를 순회한다 (C# context.ActivePositions 포팅)
	ActivePositions []*TradePosition
}

// NewTradingContext 는 초기화된 TradingContext 반환
func NewTradingContext(candleList []*Candle, boxList []*Box) *TradingContext {
	return &TradingContext{
		CandleList:            candleList,
		BoxList:               boxList,
		Position:              0,
		Exposition:            0,
		DefboxIndex:           -1,
		DefChecker:            0,
		DamChecker:            0,
		LastBuySignalPosition: make(map[string]int),
		ActivePositions:       []*TradePosition{},
	}
}

// AddActivePosition 은 새 매수 포지션을 활성 리스트에 추가한다
func (ctx *TradingContext) AddActivePosition(p *TradePosition) {
	ctx.ActivePositions = append(ctx.ActivePositions, p)
}

// ActivePositionsCount 는 IsActive==true 인 포지션 수를 반환한다
func (ctx *TradingContext) ActivePositionsCount() int {
	n := 0
	for _, p := range ctx.ActivePositions {
		if p.IsActive {
			n++
		}
	}
	return n
}

func (ctx *TradingContext) GetCurrentCandle() *Candle {
	if ctx.Position >= 0 && ctx.Position < len(ctx.CandleList) {
		return ctx.CandleList[ctx.Position]
	}
	return nil
}

func (ctx *TradingContext) GetPreviousCandle(daysBack int) *Candle {
	target := ctx.Position - daysBack
	if target >= 0 && target < len(ctx.CandleList) {
		return ctx.CandleList[target]
	}
	return nil
}

func (ctx *TradingContext) GetDefBox() *Box {
	if ctx.DefboxIndex >= 0 && ctx.DefboxIndex < len(ctx.BoxList) {
		return ctx.BoxList[ctx.DefboxIndex]
	}
	return nil
}

func (ctx *TradingContext) GetMainBox() *Box {
	defBox := ctx.GetDefBox()
	if defBox == nil || len(defBox.MainDefLink) == 0 {
		return nil
	}
	mainIdx := defBox.MainDefLink[0]
	if mainIdx >= 0 && mainIdx < len(ctx.BoxList) {
		return ctx.BoxList[mainIdx]
	}
	return nil
}

func (ctx *TradingContext) UpdateBoxInfo() {
	if ctx.DefboxIndex < 0 || ctx.DefboxIndex >= len(ctx.BoxList) {
		return
	}
	defBox := ctx.BoxList[ctx.DefboxIndex]
	ctx.DefboxPrice = defBox.Price
	ctx.DefboxPosition = defBox.BoxPosition
	ctx.DefCount = len(defBox.MainDefLink)

	if len(defBox.MainDefLink) > 0 {
		mainIdx := defBox.MainDefLink[0]
		if mainIdx >= 0 && mainIdx < len(ctx.BoxList) {
			ctx.MainboxPosition = ctx.BoxList[mainIdx].BoxPosition
		}
	}
}

func (ctx *TradingContext) ResetBuySignalPositions() {
	ctx.LastBuySignalPosition = make(map[string]int)
}
