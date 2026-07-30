package console

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// RunSQLQuery 는 지정된 DB에서 SQL 쿼리를 실행하고 결과를 콘솔에 출력합니다.
// dbName: "key", "han", "var", "KIS2", "tuf", "LS" 중 하나
func RunSQLQuery(dbName string, query string) error {
	db, err := MsConn.GetDB(dbName)
	if err != nil {
		return fmt.Errorf("DB '%s' 연결 가져오기 실패: %w", dbName, err)
	}

	queryUpper := strings.TrimSpace(strings.ToUpper(query))

	// SELECT 쿼리인 경우 결과 출력
	if strings.HasPrefix(queryUpper, "SELECT") ||
		strings.HasPrefix(queryUpper, "WITH") ||
		strings.HasPrefix(queryUpper, "EXEC") ||
		strings.HasPrefix(queryUpper, "SP_") {
		return runSelectQuery(db, query)
	}

	// INSERT, UPDATE, DELETE 등 실행 쿼리
	result, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("쿼리 실행 실패: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("실행 완료. 영향받은 행: %d\n", rowsAffected)
	return nil
}

// runSelectQuery 는 SELECT 쿼리 결과를 테이블 형태로 출력합니다.
func runSelectQuery(db *sql.DB, query string) error {
	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("쿼리 실행 실패: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("컬럼 정보 가져오기 실패: %w", err)
	}

	// 결과 데이터 수집
	var allRows [][]string
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return fmt.Errorf("row 스캔 실패: %w", err)
		}

		row := make([]string, len(columns))
		for i, val := range values {
			row[i] = formatValue(val)
		}
		allRows = append(allRows, row)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("row 반복 오류: %w", err)
	}

	if len(allRows) == 0 {
		fmt.Println("(결과 없음)")
		return nil
	}

	// 각 컬럼의 최대 너비 계산
	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = len(col)
	}
	for _, row := range allRows {
		for i, val := range row {
			if len(val) > widths[i] {
				widths[i] = len(val)
			}
			// 최대 너비 제한
			if widths[i] > 50 {
				widths[i] = 50
			}
		}
	}

	// 구분선 생성
	separator := "+"
	for _, w := range widths {
		separator += strings.Repeat("-", w+2) + "+"
	}

	// 헤더 출력
	fmt.Println(separator)
	header := "|"
	for i, col := range columns {
		header += fmt.Sprintf(" %-*s |", widths[i], truncate(col, widths[i]))
	}
	fmt.Println(header)
	fmt.Println(separator)

	// 데이터 출력
	for _, row := range allRows {
		line := "|"
		for i, val := range row {
			line += fmt.Sprintf(" %-*s |", widths[i], truncate(val, widths[i]))
		}
		fmt.Println(line)
	}
	fmt.Println(separator)

	fmt.Printf("총 %d 행\n", len(allRows))
	return nil
}

// formatValue 는 DB 스캔 값을 표시용 문자열로 바꾼다.
//
// %v를 그대로 쓰면 안 되는 이유: go-mssqldb는 DECIMAL/NUMERIC을 []byte로 돌려주는데,
// %v는 이를 바이트 배열로 찍는다 — 종가 5060이 "[53 48 54 48 46 48 48 48 48]"로 나온다.
// MONEY·VARBINARY·UNIQUEIDENTIFIER도 같은 경로다.
func formatValue(val interface{}) string {
	switch v := val.(type) {
	case nil:
		return "NULL"
	case []byte:
		// DECIMAL·MONEY는 ASCII 숫자열이므로 그대로 문자열이 된다.
		// 진짜 바이너리(VARBINARY 등)는 깨진 글자 대신 16진수로 보여준다.
		if isPrintable(v) {
			return string(v)
		}
		return "0x" + hex.EncodeToString(v)
	case time.Time:
		// 자정이면 날짜만 — DATE 컬럼이 "00:00:00"을 달고 나오는 것을 막는다.
		if v.Hour() == 0 && v.Minute() == 0 && v.Second() == 0 && v.Nanosecond() == 0 {
			return v.Format("2006-01-02")
		}
		return v.Format("2006-01-02 15:04:05")
	case float64:
		// %v는 큰 수를 지수 표기로 바꾼다 (26804038 → 2.6804038e+07).
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// isPrintable 은 출력 가능한 텍스트인지 본다 (탭·개행은 허용).
//
// UTF-8 검증을 먼저 한다 — string(b) 순회는 잘못된 바이트를 U+FFFD로 바꿔
// 넘겨주므로, 이 검사 없이는 임의의 바이너리도 "출력 가능"으로 통과한다.
func isPrintable(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, r := range string(b) {
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// truncate 문자열을 최대 길이로 자릅니다
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// PrintAvailableDBs 사용 가능한 DB 목록을 출력합니다
func PrintAvailableDBs() {
	fmt.Println("사용 가능한 DB:")
	MsConn.lock.RLock()
	defer MsConn.lock.RUnlock()
	for name := range MsConn.db {
		fmt.Printf("  - %s\n", name)
	}
}
