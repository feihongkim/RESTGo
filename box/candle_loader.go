package box

import (
	"database/sql"
	"fmt"
)

// FetchCandles 는 KIS2 DB DM.BP_PeriodPrice 에서 종목 일봉 데이터를 조회합니다.
// 결과는 날짜 오름차순(오래된 것 먼저)으로 반환됩니다.
func FetchCandles(db *sql.DB, shcode string, days int) ([]*Candle, error) {
	query := fmt.Sprintf(`
		SELECT stck_bsop_date, stck_oprc, stck_hgpr, stck_lwpr, stck_clpr, CAST(acml_vol AS FLOAT)
		FROM (
			SELECT TOP %d stck_bsop_date, stck_oprc, stck_hgpr, stck_lwpr, stck_clpr, acml_vol
			FROM DM.BP_PeriodPrice
			WHERE stck_shrn_iscd = '%s' AND period_type = 'D'
			ORDER BY stck_bsop_date DESC
		) t
		ORDER BY stck_bsop_date ASC
	`, days, shcode)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("캔들 조회 실패 (%s): %w", shcode, err)
	}
	defer rows.Close()

	var candles []*Candle
	for rows.Next() {
		c := &Candle{Shcode: shcode}
		if err := rows.Scan(&c.Date, &c.OpenOrigin, &c.HighOrigin, &c.LowOrigin, &c.CloseOrigin, &c.Volume); err != nil {
			return nil, fmt.Errorf("row 스캔 실패: %w", err)
		}
		candles = append(candles, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row 반복 오류: %w", err)
	}

	return candles, nil
}
