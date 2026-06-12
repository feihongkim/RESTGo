package stg

import "RESTGo/box"

// SellTracking 은 YAML 룰의 tracking 블록 파싱 결과.
type SellTracking struct {
	Immediate bool    `yaml:"immediate"` // true면 TrackAndCheck 우회 (즉시 실행)
	CountMin  int     `yaml:"count_min"` // 누적 횟수 임계
	RatioMin  float64 `yaml:"ratio_min"` // 보유기간 대비 발생 비율 임계
}

// TrackAndCheck 은 조건 발생을 SellConditionHistory에 기록하고
// "누적 횟수 또는 비율" 듀얼 임계값(OR 로직)을 평가한다.
// C# SFunction.Helpers.TrackAndCheckCondition 포팅.
//
// 반환값: 임계값을 충족하여 실제 매도를 트리거할 수 있는지 여부.
func TrackAndCheck(ctx *box.TradingContext, pos *box.TradePosition, conditionName string, triggered bool, tr SellTracking, s SellSettings) bool {
	if tr.Immediate {
		return triggered
	}
	if !triggered {
		return false
	}

	occurrenceCount := pos.RecordSellConditionOccurrence(conditionName, ctx.Position)

	holdingPeriod := ctx.Position - pos.BuyPosition

	countThreshold := tr.CountMin
	if countThreshold <= 0 {
		countThreshold = s.DefaultSellConditionCountThreshold
	}
	countExceeded := occurrenceCount >= countThreshold

	ratioThreshold := tr.RatioMin
	if ratioThreshold <= 0 {
		ratioThreshold = s.DefaultSellConditionRatioThreshold
	}
	ratioExceeded := false
	if holdingPeriod > 0 {
		ratio := float64(occurrenceCount) / float64(holdingPeriod)
		ratioExceeded = ratio >= ratioThreshold
	}

	return countExceeded || ratioExceeded
}
