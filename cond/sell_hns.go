package cond

// sell_hns.go — 해드앤숄더(Head and Shoulders) 천장 패턴 (2026-07-04).
//
// 정식 정의를 5이평 박스 인프라로 구현:
//   5박스 시퀀스 R(왼어깨 LS) – S(골 T1) – R(머리 H) – S(골 T2) – R(오른어깨 RS)
//   Box.Price = 구간 극값(저항=고점, 지지=저점), BoxPosition = 극값 캔들 인덱스.
//
// 경성 조건 (패턴 정의):
//   - 머리가 최고점: H > LS AND H > RS
//   - 어깨 대칭: |LS - RS| / H ≤ 10%
//   - 골이 어깨 아래: T1, T2 < min(LS, RS) (구조 무결성)
//   - 선행 상승 추세: 머리 캔들에서 MA20 > MA60 (천장 반전 패턴의 전제)
//   - RS 인지 갭 ≤ 15봉 (W/M과 동일)
//
// 트리거 (별도): 넥라인(T1–T2 저점 연결 직선) 종가 하향 이탈 — NecklineValue 참조.
//
// 연성 속성 (게이트 아님, 신호에 기록해 사후 분석용):
//   거래량 감소(RS/LS 비율 — 정식 조건이지만 소표본 방지 위해 기록만),
//   어깨 비대칭 %, 머리 돌출 %, 넥라인 기울기, 패턴 폭.

import "RESTGo/box"

// HNSLookbackBars 는 5박스 탐색 구간 (W바텀 50의 2배 — 패턴이 넓음).
const HNSLookbackBars = 100

// HNSShoulderTolerance 는 어깨 대칭 허용치 (머리 가격 대비).
const HNSShoulderTolerance = 0.10

// HNSPattern 은 해드앤숄더 탐지 결과 + 사후 분석용 속성.
type HNSPattern struct {
	LSPos, T1Pos, HPos, T2Pos, RSPos int
	LSPrice, T1Price, HPrice, T2Price, RSPrice float64
	// 속성 (게이트 아님)
	ShoulderDiffPct float64 // |LS-RS|/H × 100
	HeadPromPct     float64 // (H - max(LS,RS))/H × 100 — 머리 돌출 정도
	NeckSlopePct    float64 // 봉당 넥라인 기울기 / H × 100 (양수 = 우상향 넥라인)
	VolumeRatio     float64 // avgVol(RS±2) / avgVol(LS±2) — 1 미만이면 정식 거래량 감소 충족
	PatternWidth    int     // RSPos - LSPos
}

// NecklineValue 는 t 캔들 시점의 넥라인 가격 (T1–T2 직선 외삽).
func (p *HNSPattern) NecklineValue(t int) float64 {
	slope := (p.T2Price - p.T1Price) / float64(p.T2Pos-p.T1Pos)
	return p.T1Price + slope*float64(t-p.T1Pos)
}

// FindHNSPattern 은 pos(오른어깨 resist 박스 인지 캔들)에서 해드앤숄더 5박스를 찾는다.
func FindHNSPattern(ctx *box.TradingContext, lookback int) (*HNSPattern, bool) {
	pos := ctx.Position
	candles := ctx.CandleList

	startPos := pos - lookback
	if startPos < 0 {
		startPos = 0
	}

	// 탐색 구간 내 지지/저항 박스만 수집 (DefBox 등 미분류 제외)
	type slot struct {
		bpos  int
		btype int
		price float64
	}
	var slots []slot
	for _, b := range ctx.BoxList {
		if b.BoxPosition < startPos || b.BoxPosition >= pos+1 {
			continue
		}
		if b.KindOfBox == box.KindDefBox { // DefBox는 패턴 구조에서 제외 (엔진 경로 오염 방지)
			continue
		}
		if b.BoxType != box.BoxTypeSupport && b.BoxType != box.BoxTypeResistance {
			continue
		}
		slots = append(slots, slot{b.BoxPosition, b.BoxType, b.Price})
	}
	n := len(slots)
	if n < 5 {
		return nil, false
	}

	rs, t2, h, t1, ls := slots[n-1], slots[n-2], slots[n-3], slots[n-4], slots[n-5]
	const maxRSGap = 15
	if rs.btype != box.BoxTypeResistance || pos-rs.bpos > maxRSGap {
		return nil, false
	}
	if t2.btype != box.BoxTypeSupport || h.btype != box.BoxTypeResistance ||
		t1.btype != box.BoxTypeSupport || ls.btype != box.BoxTypeResistance {
		return nil, false
	}
	if !(ls.bpos < t1.bpos && t1.bpos < h.bpos && h.bpos < t2.bpos && t2.bpos < rs.bpos) {
		return nil, false
	}
	if h.price <= 0 {
		return nil, false
	}

	// 머리가 최고점
	if h.price <= ls.price || h.price <= rs.price {
		return nil, false
	}
	// 어깨 대칭
	diff := ls.price - rs.price
	if diff < 0 {
		diff = -diff
	}
	if diff/h.price > HNSShoulderTolerance {
		return nil, false
	}
	// 골이 어깨 아래
	minShoulder := ls.price
	if rs.price < minShoulder {
		minShoulder = rs.price
	}
	if t1.price >= minShoulder || t2.price >= minShoulder {
		return nil, false
	}
	// 선행 상승 추세: 머리 캔들 MA20 > MA60
	hc := candles[h.bpos]
	if hc.Ma20 <= 0 || hc.Ma60 <= 0 || hc.Ma20 <= hc.Ma60 {
		return nil, false
	}

	maxShoulder := ls.price
	if rs.price > maxShoulder {
		maxShoulder = rs.price
	}
	p := &HNSPattern{
		LSPos: ls.bpos, T1Pos: t1.bpos, HPos: h.bpos, T2Pos: t2.bpos, RSPos: rs.bpos,
		LSPrice: ls.price, T1Price: t1.price, HPrice: h.price, T2Price: t2.price, RSPrice: rs.price,
		ShoulderDiffPct: diff / h.price * 100,
		HeadPromPct:     (h.price - maxShoulder) / h.price * 100,
		NeckSlopePct:    (t2.price - t1.price) / float64(t2.bpos-t1.bpos) / h.price * 100,
		VolumeRatio:     avgVolumeAround(candles, rs.bpos, 2) / nonZero(avgVolumeAround(candles, ls.bpos, 2)),
		PatternWidth:    rs.bpos - ls.bpos,
	}
	return p, true
}

// avgVolumeAround 는 pos±w 구간 평균 거래량.
func avgVolumeAround(candles []*box.Candle, pos, w int) float64 {
	lo, hi := pos-w, pos+w
	if lo < 0 {
		lo = 0
	}
	if hi > len(candles)-1 {
		hi = len(candles) - 1
	}
	sum, n := 0.0, 0
	for i := lo; i <= hi; i++ {
		sum += candles[i].Volume
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

func nonZero(v float64) float64 {
	if v == 0 {
		return 1
	}
	return v
}
