package console

import (
	"database/sql"
	"fmt"
	"strings"
)

// RunSQLQuery 는 지정된 DB에서 SQL 쿼리를 실행하고 결과를 콘솔에 출력합니다.
// dbName: "key", "han", "var", "KIS2" 중 하나
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
			if val == nil {
				row[i] = "NULL"
			} else {
				row[i] = fmt.Sprintf("%v", val)
			}
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
