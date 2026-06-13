package box

import (
	"database/sql"
	"fmt"
)

// PairedCandle 은 동일 시각 두 마켓 캔들 쌍.
type PairedCandle struct {
	Date string
	Time string
	A    *Candle
	B    *Candle
}

// FetchUpbitPair15m 은 두 마켓의 15분봉을 시각 정렬해 반환한다.
// 한쪽이 없는 시각은 스킵 (페어 동시 평가 가능한 시점만 사용).
func FetchUpbitPair15m(db *sql.DB, marketA, marketB string, limit int) ([]PairedCandle, error) {
	candlesA, err := FetchUpbitCandles15m(db, marketA, limit)
	if err != nil {
		return nil, fmt.Errorf("페어 A(%s) 로드 실패: %w", marketA, err)
	}
	candlesB, err := FetchUpbitCandles15m(db, marketB, limit)
	if err != nil {
		return nil, fmt.Errorf("페어 B(%s) 로드 실패: %w", marketB, err)
	}

	// 시간 키 -> 캔들 인덱스 맵
	keyOf := func(c *Candle) string { return c.Date + c.Time }
	indexB := make(map[string]*Candle, len(candlesB))
	for _, c := range candlesB {
		indexB[keyOf(c)] = c
	}

	var paired []PairedCandle
	for _, a := range candlesA {
		k := keyOf(a)
		b, ok := indexB[k]
		if !ok {
			continue
		}
		if a.CloseOrigin <= 0 || b.CloseOrigin <= 0 {
			continue
		}
		paired = append(paired, PairedCandle{
			Date: a.Date,
			Time: a.Time,
			A:    a,
			B:    b,
		})
	}
	return paired, nil
}
