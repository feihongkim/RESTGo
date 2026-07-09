package box

import (
	"database/sql"
	"fmt"
	"time"
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

// FetchCandlesHannam 는 hannam DB stock_price_kor_d001 에서 일봉 데이터를 조회합니다.
// KIS2의 BP_PeriodPrice(1.5년)와 달리 약 16년치 데이터 보유.
// 컬럼명 OPEN/CLOSE/HIGH/LOW가 SQL 예약어라 대괄호 escape 필요.
func FetchCandlesHannam(db *sql.DB, shcode string, days int) ([]*Candle, error) {
	query := fmt.Sprintf(`
		SELECT DATE, [OPEN], [HIGH], [LOW], [CLOSE], CAST(VOLUME AS FLOAT)
		FROM (
			SELECT TOP %d DATE, [OPEN], [HIGH], [LOW], [CLOSE], VOLUME
			FROM stock_price_kor_d001
			WHERE SHCODE = '%s' AND STICK_TYPE = 'D001'
			ORDER BY DATE DESC
		) t
		ORDER BY DATE ASC
	`, days, shcode)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("hannam 캔들 조회 실패 (%s): %w", shcode, err)
	}
	defer rows.Close()

	var candles []*Candle
	for rows.Next() {
		c := &Candle{Shcode: shcode}
		if err := rows.Scan(&c.Date, &c.OpenOrigin, &c.HighOrigin, &c.LowOrigin, &c.CloseOrigin, &c.Volume); err != nil {
			return nil, fmt.Errorf("hannam row 스캔 실패: %w", err)
		}
		candles = append(candles, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("hannam row 반복 오류: %w", err)
	}
	return candles, nil
}

// FetchCandlesForeign 는 KIS2 FG.BP_PeriodPrice (외국 일봉)에서 종목 일봉을 조회한다.
// rsym 형식: 'DNYAAPL' (DNY=NYSE), 'DNAMSFT' (DNA=NASDAQ) 등.
// 가격 컬럼은 byte 배열로 저장돼 있어 CAST VARCHAR 후 Go에서 ParseFloat.
func FetchCandlesForeign(db *sql.DB, rsym string, days int) ([]*Candle, error) {
	query := fmt.Sprintf(`
		SELECT xymd,
		       CAST([open] AS VARCHAR(20)),
		       CAST(high AS VARCHAR(20)),
		       CAST([low] AS VARCHAR(20)),
		       CAST(clos AS VARCHAR(20)),
		       CAST(tvol AS FLOAT)
		FROM (
			SELECT TOP %d xymd, [open], high, [low], clos, tvol
			FROM FG.BP_PeriodPrice
			WHERE rsym = '%s'
			ORDER BY xymd DESC
		) t
		ORDER BY xymd ASC
	`, days, rsym)
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("foreign 캔들 조회 실패 (%s): %w", rsym, err)
	}
	defer rows.Close()
	var candles []*Candle
	for rows.Next() {
		c := &Candle{Shcode: rsym}
		var oS, hS, lS, cS string
		if err := rows.Scan(&c.Date, &oS, &hS, &lS, &cS, &c.Volume); err != nil {
			return nil, fmt.Errorf("foreign row 스캔 실패: %w", err)
		}
		c.OpenOrigin = parseForeignPrice(oS)
		c.HighOrigin = parseForeignPrice(hS)
		c.LowOrigin = parseForeignPrice(lS)
		c.CloseOrigin = parseForeignPrice(cS)
		candles = append(candles, c)
	}
	return candles, nil
}

// parseForeignPrice — KIS 외국 가격 문자열 파싱 (음수 포함, dash prefix)
func parseForeignPrice(s string) float64 {
	if s == "" {
		return 0
	}
	v := 0.0
	fmt.Sscanf(s, "%f", &v)
	return v
}

// FetchForeignStockList 는 FG.BP_PeriodPrice에서 prefix(시장)별 활성 종목 목록을 가져온다.
// prefix 예: "DNY"(NYSE), "DNA"(NASDAQ). 최근 1개월 내 거래 있는 종목만.
func FetchForeignStockList(db *sql.DB, prefixes []string) ([]string, error) {
	// 가장 활성 (최근 30일에 데이터 있는) 종목만
	prefCond := ""
	for i, p := range prefixes {
		if i > 0 {
			prefCond += " OR "
		}
		prefCond += fmt.Sprintf("LEFT(rsym,3)='%s'", p)
	}
	if prefCond == "" {
		prefCond = "1=1"
	}
	query := fmt.Sprintf(`
		SELECT DISTINCT rsym
		FROM FG.BP_PeriodPrice
		WHERE xymd = '20220315' AND (%s)
		ORDER BY rsym
	`, prefCond)
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("foreign 종목 목록 조회 실패: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err == nil && s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

// FetchHannamStockList 는 hannam DB에서 KOR 종목 목록을 가져옵니다.
// stock_price_kor_d001은 SHCODE 인덱스가 없을 수 있으므로 별도 종목 마스터 테이블에서 조회 권장이나,
// 폴백으로 BP_PeriodPrice 기반 종목 목록을 사용해도 됨 (KIS2 종목과 동일).
func FetchHannamStockList(db *sql.DB) ([]string, error) {
	// stock_price_kor_d001에서 DISTINCT SHCODE는 큰 테이블 인덱스 부재로 timeout 위험.
	// 최근 35일 범위로 좁혀서 가져옴 (2026-07-09: 하드코딩 날짜 → 롤링 윈도우 수정 — 운용 전환).
	to := time.Now().Format("20060102")
	from := time.Now().AddDate(0, 0, -35).Format("20060102")
	rows, err := db.Query(fmt.Sprintf(`
		SELECT DISTINCT SHCODE
		FROM stock_price_kor_d001
		WHERE DATE BETWEEN '%s' AND '%s' AND STICK_TYPE = 'D001'
		ORDER BY SHCODE
	`, from, to))
	if err != nil {
		return nil, fmt.Errorf("hannam 종목 목록 조회 실패: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err == nil && s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}
