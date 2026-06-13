package box

import (
	"database/sql"
	"fmt"
	"time"
)

// FetchUpbitCandles15m loads 15-minute candles for a crypto market from TUF DB.
// market: e.g. "KRW-BTC", limit: number of candles to fetch (equivalent to 'days' for daily).
// Results are returned in ascending order (oldest first).
//
// Fill assumption: same-candle fill — the signal candle is also the fill candle.
// Next-candle open fill is a future enhancement.
func FetchUpbitCandles15m(db *sql.DB, market string, limit int) ([]*Candle, error) {
	query := fmt.Sprintf(`
		SELECT *
		FROM (
			SELECT TOP %d
				market,
				REPLACE(CONVERT(VARCHAR(10), CAST(candle_date_time_kst AS DATETIME), 112), '-', '') AS [date],
				CONVERT(VARCHAR(8), CAST(candle_date_time_kst AS DATETIME), 108) AS [time],
				opening_price,
				high_price,
				low_price,
				trade_price,
				candle_acc_trade_volume
			FROM candles_15m
			WHERE market = '%s'
			ORDER BY candle_date_time_kst DESC
		) t
		ORDER BY t.[date] ASC, t.[time] ASC
	`, limit, market)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("Upbit 캔들 조회 실패 (%s): %w", market, err)
	}
	defer rows.Close()

	var candles []*Candle
	for rows.Next() {
		c := &Candle{Shcode: market, Hname: market}
		if err := rows.Scan(&c.Shcode, &c.Date, &c.Time,
			&c.OpenOrigin, &c.HighOrigin, &c.LowOrigin, &c.CloseOrigin, &c.Volume); err != nil {
			return nil, fmt.Errorf("Upbit row 스캔 실패: %w", err)
		}
		candles = append(candles, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("Upbit row 반복 오류: %w", err)
	}

	// Gap detection: log if >1 expected 15m slot is missing
	logUpbitGaps(candles)

	return candles, nil
}

// logUpbitGaps logs consecutive 15m candles where the gap exceeds 15 minutes.
// Upbit runs 24/7 so any gap > 16 minutes (1 min slack) is noteworthy.
func logUpbitGaps(candles []*Candle) {
	if len(candles) < 2 {
		return
	}
	for i := 1; i < len(candles); i++ {
		prev := candles[i-1]
		curr := candles[i]
		prevDT := prev.Date + prev.Time
		currDT := curr.Date + curr.Time
		t1, err1 := time.Parse("20060102150405", prevDT)
		t2, err2 := time.Parse("20060102150405", currDT)
		if err1 != nil || err2 != nil {
			continue
		}
		gap := t2.Sub(t1)
		if gap > 16*time.Minute { // allow 1 min slack
			fmt.Printf("[Upbit] 갭 감지: %s %s → %s %s (%.0f분)\n",
				prev.Date, prev.Time, curr.Date, curr.Time, gap.Minutes())
		}
	}
}
