// Package scheduler 는 RESTGo 정기 배치를 단일 상주 프로세스로 관리한다.
//
// crontab 대신 쓰는 이유 (2026-07-30 도입 배경):
//
//	기존 cron 항목이 존재하지 않는 경로(`cd /home/feihong/code/REST/RESTGo`)를 가리켜
//	`&&` 단락으로 2주간(20260717~) 조용히 실행되지 않았다. cron은 실패를 알려주지 않는다.
//	스케줄러는 ① 기동 즉시 스크립트 존재를 검증하고 ② 실패 시 Telegram으로 알린다.
//
// 구조는 KIS2(cmd/scheduler)·MakeSQL(scheduler) 컨벤션을 따른다.
package scheduler

import "time"

// Task 는 정해진 시각에 실행할 배치 하나를 정의한다.
type Task struct {
	Label     string               // 완료 기록·로그 식별자 (하루 1회 보장 키)
	Time      string               // "HH:MM" (KST) 실행 시각
	Script    string               // 프로젝트 루트 기준 스크립트 경로
	Args      []string             // 스크립트 인수
	LogFile   string               // 출력 append 경로 (빈 값이면 스케줄러 로그로)
	DependsOn string               // 이 라벨이 완료돼야 실행 (지연 대응 — 예정시각 이후 대기)
	Condition func(time.Time) bool // 실행 조건 (nil이면 항상)
}

// BuildSchedule 은 전체 스케줄을 반환한다.
//
// 설계 원칙:
//   - 국내 일봉(hannam) 적재 완료 후 실행한다. 적재가 지연돼도 배치는 안전하다 —
//     data_date 커버리지 규약이 직전 완전일을 기준으로 삼는다 (stock/handler.go).
//   - 거래일(월~금 중 KRX 휴장일 제외)만 실행한다.
//   - paper_wd는 daily_batch와 독립이다. 데이터 경로가 달라 의존을 걸지 않는다.
func BuildSchedule() []Task {
	return []Task{
		{
			Label: "daily_batch", Time: "16:30",
			Script: "daily_batch.sh", LogFile: "zpicture/daily_batch.log",
			Condition: isTradingDay,
		},
		{
			Label: "paper_wd", Time: "16:45",
			Script: "paper_wd_daily.sh", LogFile: "zpicture/paper_wd/daily.log",
			Condition: isTradingDay,
		},
		{
			// 월간 리포트는 원장 JSON만 읽으므로 거래일일 필요가 없다.
			// 거래일 조건을 걸면 1일이 주말·휴장일인 달은 리포트가 통째로 누락된다.
			Label: "paper_wd_report", Time: "09:00",
			Script: "paper_wd_report_monthly.sh", LogFile: "zpicture/paper_wd/report.log",
			Condition: isFirstOfMonth,
		},
	}
}

// isTradingDay 는 국내 거래일(월~금 중 KRX 휴장일 제외)인지 반환한다.
func isTradingDay(t time.Time) bool {
	d := t.Weekday()
	if d < time.Monday || d > time.Friday {
		return false
	}
	return !isKRXHoliday(t)
}

// isFirstOfMonth 는 매달 1일인지 반환한다 (요일·휴장일 무관).
func isFirstOfMonth(t time.Time) bool {
	return t.Day() == 1
}
