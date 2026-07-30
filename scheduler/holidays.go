package scheduler

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// krxHolidayFile 은 KRX 휴장일 목록의 기본 경로.
//
// LS 프로젝트(ls-scheduler)가 KRX 공식 캘린더를 보고 갱신하는 파일을 그대로 읽는다.
// 복사본을 두면 해가 바뀌거나 임시 휴장이 생겼을 때 한쪽만 갱신돼 조용히 어긋난다.
// RESTGO_KRX_HOLIDAYS 로 경로를 바꿀 수 있다 (테스트·이관 대비).
const krxHolidayFile = "/home/feihong/code/LS/ls-scheduler/holidays_krx.json"

type krxHolidayDoc struct {
	Market   string   `json:"market"`
	Year     int      `json:"year"`
	Holidays []string `json:"holidays"` // "YYYY-MM-DD"
}

// holidaySet 은 "YYYYMMDD" 집합. 미로드 상태면 nil이고, 그때는 요일만으로 판정한다.
var holidaySet map[string]bool

// holidayYear 는 로드된 목록이 커버하는 연도 (0이면 미로드).
var holidayYear int

// LoadKRXHolidays 는 휴장일 목록을 읽어 전역에 적재한다.
// 반환 문자열은 기동 로그용 요약이다. 파일이 없거나 깨져도 에러만 돌려주고
// 스케줄러는 계속 뜬다 — 휴장일에 배치가 한 번 더 도는 것은 무해하지만
// (data_date 커버리지 규약상 직전 완전일로 떨어지고 멱등), 스케줄러가 아예
// 안 뜨면 모든 배치가 멈춘다. 다만 경고는 반드시 눈에 띄게 남긴다.
func LoadKRXHolidays() (string, error) {
	path := krxHolidayFile
	if p := os.Getenv("RESTGO_KRX_HOLIDAYS"); p != "" {
		path = p
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("휴장일 목록 읽기 실패 (%s): %w", path, err)
	}
	var doc krxHolidayDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("휴장일 목록 파싱 실패 (%s): %w", path, err)
	}
	set := make(map[string]bool, len(doc.Holidays))
	for _, d := range doc.Holidays {
		t, perr := time.Parse("2006-01-02", d)
		if perr != nil {
			return "", fmt.Errorf("휴장일 날짜 형식 오류 %q: %w", d, perr)
		}
		set[t.Format("20060102")] = true
	}
	holidaySet = set
	holidayYear = doc.Year

	summary := fmt.Sprintf("%s %d년 휴장일 %d일 (%s)", doc.Market, doc.Year, len(set), path)
	// 단일 연도 파일이라 해가 바뀌면 조용히 무용지물이 된다. 기동 시 드러나게 한다.
	if y := time.Now().In(loc).Year(); doc.Year != y {
		return summary, fmt.Errorf("휴장일 목록이 %d년치인데 현재는 %d년 — LS ls-scheduler/holidays_krx.json 갱신 필요",
			doc.Year, y)
	}
	return summary, nil
}

// isKRXHoliday 는 해당 일자가 KRX 휴장일인지 반환한다. 목록 미로드 시 항상 false.
func isKRXHoliday(t time.Time) bool {
	if holidaySet == nil {
		return false
	}
	return holidaySet[t.Format("20060102")]
}
