package box

import (
	"database/sql"
	"fmt"
	"time"
)

// LS DB (white MSSQL `LS`) 로더 — XingAPI 수집 파이프라인이 적재한 원본 테이블을 읽는다.
//
// 일봉 정본은 ST.t8410 (API전용주식챠트 일주월년). 수집 시 InBlock이 그대로 컬럼으로
// 남으므로 반드시 in_gubun='2'(일봉) AND in_sujung='Y'(수정주가)로 걸러야 한다.
// 걸지 않으면 주/월/년봉이 섞여 시계열이 깨진다.
//
// (in_shcode, date) 조합은 유일하다 — LsSaver가 같은 키를 덮어쓰므로 주간 재수집으로
// 구간이 겹쳐도 중복 행이 생기지 않는다 (2026-07-31 실측: 중복 0건).

// LSBar 는 발행기용 경량 일봉이다.
//
// box.Candle 은 지표 슬롯(Ma5~Ma200 등)까지 포함해 행당 수백 바이트라, 전 종목
// 전 기간(약 130만 행)을 메모리에 올리면 수백 MB가 된다. 발행기는 OHLCV만 쓰므로
// 이 구조체로 담아 약 77MB에 맞춘다.
type LSBar struct {
	Date   string
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// LSQuote 는 ST.t8407(주식 멀티 현재가)의 한 종목 스냅샷이다.
// 장중 조회이므로 open/high/low는 당일 누적, price는 조회 시점 현재가, volume은 누적거래량.
type LSQuote struct {
	Shcode    string
	Hname     string
	Price     float64
	Open      float64
	High      float64
	Low       float64
	Volume    float64
	PrevClose float64
	QryID     int64
	RegAt     time.Time
}

// VirtualBar 는 현재가 스냅샷을 "가상 금일봉"으로 환산한다.
//
// 발행 스펙(cmst-st2-publisher-spec.md §4)의 마지막 봉 규약: 고가·저가는 당일 누적,
// 종가는 현재가, 거래량은 누적. 동시호가 전 스냅샷이라 확정봉과는 다를 수 있다.
func (q LSQuote) VirtualBar(date string) LSBar {
	high, low := q.High, q.Low
	// 장 초반이나 거래 부진 종목은 고/저가 0으로 오는 경우가 있다. 현재가로 보정하지
	// 않으면 low=0이 되어 지표(ATR·%B 등)가 통째로 망가진다.
	if high <= 0 || high < q.Price {
		high = q.Price
	}
	if low <= 0 || low > q.Price {
		low = q.Price
	}
	open := q.Open
	if open <= 0 {
		open = q.Price
	}
	return LSBar{Date: date, Open: open, High: high, Low: low, Close: q.Price, Volume: q.Volume}
}

// lsDailyWhere 는 일봉 필터의 정본. 다른 gubun/sujung 조합이 섞이지 않도록 한 곳에 둔다.
const lsDailyWhere = `in_gubun = '2' AND in_sujung = 'Y'`

// LSCutoffDate 는 "최근 n 거래일" 창의 시작 날짜를 돌려준다.
//
// 전 종목 일괄 로드 시 WHERE date >= cutoff 로 행 수를 줄이는 용도다. 종목별로 상장일이
// 달라도 거래일 달력은 공통이므로 전체 distinct date 기준으로 뽑으면 된다.
func LSCutoffDate(db *sql.DB, tradingDays int, endDate string) (string, error) {
	if tradingDays <= 0 {
		return "", fmt.Errorf("tradingDays는 양수여야 함: %d", tradingDays)
	}
	q := fmt.Sprintf(`
		SELECT MIN(date) FROM (
			SELECT DISTINCT TOP (%d) date FROM ST.t8410
			WHERE %s AND (@p1 = '' OR date <= @p1)
			ORDER BY date DESC
		) t`, tradingDays, lsDailyWhere)
	var cut sql.NullString
	if err := db.QueryRow(q, endDate).Scan(&cut); err != nil {
		return "", fmt.Errorf("LS 컷오프 날짜 조회 실패: %w", err)
	}
	if !cut.Valid {
		return "", fmt.Errorf("LS 일봉이 비어 있음 (ST.t8410, %s)", lsDailyWhere)
	}
	return cut.String, nil
}

// FetchAllBarsLS 는 cutoff 이후 전 종목 일봉을 한 번의 쿼리로 메모리에 올린다.
//
// 종목당 개별 쿼리(4,300회)를 돌리는 대신 단일 스트림으로 받는다 — 백테스트 재생처럼
// 같은 데이터를 창만 바꿔 여러 번 쓰는 경우 이 차이가 크다.
// 반환 맵의 각 슬라이스는 날짜 오름차순이다.
func FetchAllBarsLS(db *sql.DB, cutoff, endDate string) (map[string][]LSBar, error) {
	q := fmt.Sprintf(`
		SELECT in_shcode, date, [open], high, low, [close], CAST(jdiff_vol AS FLOAT)
		FROM ST.t8410
		WHERE %s AND date >= @p1 AND (@p2 = '' OR date <= @p2)
		ORDER BY in_shcode, date`, lsDailyWhere)

	rows, err := db.Query(q, cutoff, endDate)
	if err != nil {
		return nil, fmt.Errorf("LS 전종목 일봉 조회 실패: %w", err)
	}
	defer rows.Close()

	out := make(map[string][]LSBar, 4500)
	for rows.Next() {
		var code string
		var b LSBar
		if err := rows.Scan(&code, &b.Date, &b.Open, &b.High, &b.Low, &b.Close, &b.Volume); err != nil {
			return nil, fmt.Errorf("LS 일봉 스캔 실패: %w", err)
		}
		if code == "" || b.Date == "" {
			continue
		}
		out[code] = append(out[code], b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("LS 일봉 반복 오류: %w", err)
	}
	return out, nil
}

// FetchCandlesLS 는 단일 종목 일봉을 box.Candle 로 조회한다 (analyze/batch 호환 경로).
func FetchCandlesLS(db *sql.DB, shcode string, days int) ([]*Candle, error) {
	q := fmt.Sprintf(`
		SELECT date, [open], high, low, [close], CAST(jdiff_vol AS FLOAT)
		FROM (
			SELECT TOP %d date, [open], high, low, [close], jdiff_vol
			FROM ST.t8410
			WHERE %s AND in_shcode = @p1
			ORDER BY date DESC
		) t
		ORDER BY date ASC`, days, lsDailyWhere)

	rows, err := db.Query(q, shcode)
	if err != nil {
		return nil, fmt.Errorf("LS 캔들 조회 실패 (%s): %w", shcode, err)
	}
	defer rows.Close()

	var candles []*Candle
	for rows.Next() {
		c := &Candle{Shcode: shcode}
		if err := rows.Scan(&c.Date, &c.OpenOrigin, &c.HighOrigin, &c.LowOrigin, &c.CloseOrigin, &c.Volume); err != nil {
			return nil, fmt.Errorf("LS row 스캔 실패: %w", err)
		}
		candles = append(candles, c)
	}
	return candles, rows.Err()
}

// FetchLSStockList 는 최근 35일 안에 봉이 있는 종목 목록을 돌려준다.
// (hannam/KIS2 로더와 같은 규약 — 상장폐지 종목이 자동으로 빠진다.)
func FetchLSStockList(db *sql.DB) ([]string, error) {
	to := time.Now().Format("20060102")
	from := time.Now().AddDate(0, 0, -35).Format("20060102")
	q := fmt.Sprintf(`
		SELECT DISTINCT in_shcode FROM ST.t8410
		WHERE %s AND date BETWEEN @p1 AND @p2
		ORDER BY in_shcode`, lsDailyWhere)
	rows, err := db.Query(q, from, to)
	if err != nil {
		return nil, fmt.Errorf("LS 종목 목록 조회 실패: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err == nil && s != "" {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}

// FetchLSStockNames 는 MS.t8436(주식 종목 마스터, 매일 08:30 수집)에서 종목명 맵을 만든다.
// 실패해도 발행은 계속돼야 하므로 호출측에서 빈 맵을 허용한다.
func FetchLSStockNames(db *sql.DB) (map[string]string, error) {
	m := map[string]string{}
	rows, err := db.Query(`SELECT shcode, RTRIM(hname) FROM MS.t8436 WHERE shcode <> ''`)
	if err != nil {
		return m, fmt.Errorf("LS 종목명 조회 실패: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c, n string
		if err := rows.Scan(&c, &n); err == nil && c != "" {
			m[c] = n
		}
	}
	return m, rows.Err()
}

// FetchLSQuotes 는 ST.t8407(멀티 현재가)에서 종목별 최신 스냅샷을 돌려준다.
//
// 하나의 수집 사이클이 50종목씩 여러 번(전종목이면 86회) 나뉘어 들어오므로 qry_id가
// 청크마다 다르다. 따라서 "가장 큰 qry_id 하나"가 아니라 **종목별 최신 행**을 잡아야
// 전 종목이 모인다. qry_date로 당일분만 본다.
func FetchLSQuotes(db *sql.DB, qryDate string) (map[string]LSQuote, error) {
	q := `
		SELECT t.shcode, RTRIM(t.hname), t.price, t.[open], t.high, t.low,
		       CAST(t.volume AS FLOAT), t.jnilclose, t.qry_id, t.DT_REG
		FROM ST.t8407 t
		JOIN (
			SELECT shcode, MAX(qry_id) AS qry_id
			FROM ST.t8407
			WHERE qry_date = @p1 AND shcode <> ''
			GROUP BY shcode
		) m ON m.shcode = t.shcode AND m.qry_id = t.qry_id
		WHERE t.qry_date = @p1 AND t.shcode <> ''`

	rows, err := db.Query(q, qryDate)
	if err != nil {
		return nil, fmt.Errorf("LS 멀티 현재가 조회 실패: %w", err)
	}
	defer rows.Close()

	out := make(map[string]LSQuote, 4500)
	for rows.Next() {
		var v LSQuote
		var prev sql.NullFloat64
		if err := rows.Scan(&v.Shcode, &v.Hname, &v.Price, &v.Open, &v.High, &v.Low,
			&v.Volume, &prev, &v.QryID, &v.RegAt); err != nil {
			return nil, fmt.Errorf("LS 현재가 스캔 실패: %w", err)
		}
		if prev.Valid {
			v.PrevClose = prev.Float64
		}
		// 현재가 0은 미체결/거래정지이거나 빈 응답이다. 종가 0인 봉을 붙이면
		// 지표가 0으로 붕괴하므로 여기서 버린다.
		if v.Shcode == "" || v.Price <= 0 {
			continue
		}
		out[v.Shcode] = v
	}
	return out, rows.Err()
}
