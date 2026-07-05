package cond

// buy_doublebump.go — 전략3 Double Bump (이중 고점 돌파) 이식 (2026-07-05, r_stg/전략3).
//
// 원본: r_stg/전략3/double_bump_temp.r 의 SQL 코어를 충실 이식 (R 후처리 제외 조건 ~15종은
// 1차 측정에서 미포함 — 유효 시 증분). 가설: 거래량 수반 1차 범프(1개월 신고가) 후 고점을
// 아직 넘지 못한 되돌림 구간에서, 되돌림의 35% 이상을 회복한 지점이 2차 돌파 진입 후보.
//
// 1차 범프 판정 (bumpPos = i-1, 확정은 i에서 — avg3_a1이 다음날 종가를 쓰므로):
//   high[b] > max(high[b-16..b-1])      (1개월 신고가)
//   high[b] > MA5[b], MA20[b]
//   vol[b] > vol20[b]×1.5 OR vol5[b] > vol20[b]×1.5   (vol20=직전 20봉 평균, vol5=자신 포함 5봉)
//   avg3[b] > avg3_1[b] AND avg3_a1[b] > avg3[b]      (3일 이평 상승 가속, a1은 b+1 종가 포함)
//   closeOrigin[b] × vol5[b] > 8.5억                   (거래대금)
//   진폭: high[b] / min(close[b-6..b-1]) < 2           (2배 이상 범프 제외)
//
// 발화(재시도 준비일) 조건은 stg 쪽 armed 트리거(DoubleBumpRetest)에서 상태와 함께 평가.

import "RESTGo/box"

// DoubleBumpInfo 는 1차 범프 정보 (armed 트리거 상태용).
type DoubleBumpInfo struct {
	BumpPos  int
	High     float64 // 범프 고점 (스케일)
	LowBound float64 // 범프 직전 6봉 최저 종가 (스케일)
	Volume   float64 // 범프 거래량
}

// FindDoubleBump 는 pos-1 캔들이 1차 범프인지 판정한다 (pos에서 확정 — 익일 종가 필요).
func FindDoubleBump(candles []*box.Candle, pos int) (*DoubleBumpInfo, bool) {
	b := pos - 1
	if b < 26 || pos >= len(candles) { // rn1 > 25 정합 (충분한 이력)
		return nil, false
	}
	c := candles[b]
	if c.Ma5 <= 0 || c.Ma20 <= 0 {
		return nil, false
	}
	// 1개월(직전 16봉) 신고가
	hi1mo := 0.0
	for i := b - 16; i < b; i++ {
		if candles[i].High > hi1mo {
			hi1mo = candles[i].High
		}
	}
	if c.High <= hi1mo || c.High <= c.Ma5 || c.High <= c.Ma20 {
		return nil, false
	}
	// 거래량: vol20 = 직전 20봉 평균, vol5 = 자신 포함 5봉 평균
	vol20 := 0.0
	for i := b - 20; i < b; i++ {
		vol20 += candles[i].Volume
	}
	vol20 /= 20
	vol5 := 0.0
	for i := b - 4; i <= b; i++ {
		vol5 += candles[i].Volume
	}
	vol5 /= 5
	if !(c.Volume > vol20*1.5 || vol5 > vol20*1.5) {
		return nil, false
	}
	// 3일 이평 상승 가속 (avg3_a1은 b+1 종가 포함 — pos 시점에서 과거)
	avg3 := (candles[b-2].Close + candles[b-1].Close + c.Close) / 3
	avg31 := (candles[b-3].Close + candles[b-2].Close + candles[b-1].Close) / 3
	avg3a1 := (candles[b-1].Close + c.Close + candles[b+1].Close) / 3
	if !(avg3 > avg31 && avg3a1 > avg3) {
		return nil, false
	}
	// 거래대금 (원본 가격 기준)
	if c.CloseOrigin*vol5 <= 850_000_000 {
		return nil, false
	}
	// 범프 직전 6봉 최저 종가 + 진폭 상한
	low := candles[b-6].Close
	for i := b - 6; i < b; i++ {
		if candles[i].Close < low {
			low = candles[i].Close
		}
	}
	if low <= 0 || c.High/low >= 2 {
		return nil, false
	}
	return &DoubleBumpInfo{BumpPos: b, High: c.High, LowBound: low, Volume: c.Volume}, true
}

// IsDoubleBumpRetestDay 는 범프 정보 대비 pos 캔들이 "재시도 준비일" 조건을 충족하는지.
// (higherCloses 카운트 관리는 호출측 armed 상태에서 — SQL: 범프 고점 종가 돌파 2회 미만)
func IsDoubleBumpRetestDay(candles []*box.Candle, pos int, info *DoubleBumpInfo) bool {
	c := candles[pos]
	if c.Close >= info.High || c.Open >= info.High { // 아직 고점 아래
		return false
	}
	if info.Volume <= c.Volume { // 범프 거래량 > 당일 거래량
		return false
	}
	if c.Close <= info.LowBound+0.35*(info.High-info.LowBound) { // 되돌림 35% 이상 회복
		return false
	}
	if c.CloseOrigin*c.Volume <= 800_000_000 { // 당일 거래대금 8억 초과
		return false
	}
	return true
}
