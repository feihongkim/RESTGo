package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func at(hm string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04", "2026-07-30 "+hm, loc)
	return t
}

func TestInTimeWindow(t *testing.T) {
	cases := []struct {
		now  string
		want bool
	}{
		{"16:29", false}, // 이르면 안 됨
		{"16:30", true},  // 정각
		{"16:31", true},  // 30초 tick이 놓쳐도 다음 tick에 잡히도록 2분 창
		{"16:32", false}, // 창 종료
		{"17:00", false},
	}
	for _, c := range cases {
		if got := inTimeWindow("16:30", at(c.now)); got != c.want {
			t.Errorf("inTimeWindow(16:30, %s) = %v, want %v", c.now, got, c.want)
		}
	}
}

func TestIsPastTime(t *testing.T) {
	if isPastTime("16:30", at("16:29")) {
		t.Error("예정 시각 전인데 past로 판정")
	}
	// DependsOn 대기는 창이 닫히면 안 된다 — 선행 작업이 늦어도 따라 실행돼야 한다
	for _, hm := range []string{"16:30", "18:00", "23:59"} {
		if !isPastTime("16:30", at(hm)) {
			t.Errorf("isPastTime(16:30, %s) = false, want true", hm)
		}
	}
}

func TestParseHMRejectsBadInput(t *testing.T) {
	for _, bad := range []string{"", "1630", "24:00", "16:60", "-1:00", "aa:bb", "16:30:00"} {
		if _, _, err := parseHM(bad); err == nil {
			t.Errorf("parseHM(%q) 가 통과됨 — 거부해야 함", bad)
		}
	}
	h, m, err := parseHM("16:30")
	if err != nil || h != 16 || m != 30 {
		t.Errorf("parseHM(16:30) = (%d,%d,%v)", h, m, err)
	}
}

// 휴장일·주말에는 실행하지 않아야 한다.
func TestIsTradingDay(t *testing.T) {
	holidaySet = map[string]bool{"20260817": true} // 광복절 대체
	defer func() { holidaySet = nil }()

	cases := []struct {
		date string
		want bool
		why  string
	}{
		{"2026-07-30", true, "목요일 평일"},
		{"2026-08-01", false, "토요일"},
		{"2026-08-02", false, "일요일"},
		{"2026-08-17", false, "월요일이지만 KRX 휴장일"},
		{"2026-08-18", true, "화요일 평일"},
	}
	for _, c := range cases {
		d, _ := time.ParseInLocation("2006-01-02", c.date, loc)
		if got := isTradingDay(d); got != c.want {
			t.Errorf("isTradingDay(%s) = %v, want %v (%s)", c.date, got, c.want, c.why)
		}
	}
}

// 휴장일 목록이 없으면 요일 판정만 하고, 휴장일을 막지는 못한다 (degrade 동작 고정).
func TestIsTradingDayWithoutHolidayList(t *testing.T) {
	holidaySet = nil
	d, _ := time.ParseInLocation("2006-01-02", "2026-08-17", loc)
	if !isTradingDay(d) {
		t.Error("목록 미로드 시 평일은 거래일로 봐야 한다 (스케줄러가 멈추면 안 됨)")
	}
}

func TestLoadKRXHolidays(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "h.json")
	year := time.Now().In(loc).Year()
	body := `{"market":"KR","year":` + itoa(year) + `,"holidays":["2026-01-01","2026-08-17"]}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RESTGO_KRX_HOLIDAYS", path)
	defer func() { holidaySet, holidayYear = nil, 0 }()

	info, err := LoadKRXHolidays()
	if err != nil {
		t.Fatalf("LoadKRXHolidays: %v", err)
	}
	if info == "" {
		t.Error("요약 문자열이 비었다")
	}
	if len(holidaySet) != 2 || !holidaySet["20260817"] {
		t.Errorf("휴장일 파싱 실패: %v", holidaySet)
	}
}

// 연도가 어긋나면 에러로 드러나야 한다 — 단일 연도 파일이라 해가 바뀌면 무용지물이 된다.
func TestLoadKRXHolidaysStaleYear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "h.json")
	os.WriteFile(path, []byte(`{"market":"KR","year":1999,"holidays":["1999-01-01"]}`), 0644)
	t.Setenv("RESTGO_KRX_HOLIDAYS", path)
	defer func() { holidaySet, holidayYear = nil, 0 }()

	if _, err := LoadKRXHolidays(); err == nil {
		t.Error("연도 불일치가 에러로 보고되지 않음")
	}
	// 에러여도 목록 자체는 적재돼야 한다 (부분 동작 유지)
	if len(holidaySet) != 1 {
		t.Errorf("연도 불일치 시에도 목록은 적재돼야 한다: %v", holidaySet)
	}
}

func TestLoadKRXHolidaysMissingFile(t *testing.T) {
	t.Setenv("RESTGO_KRX_HOLIDAYS", filepath.Join(t.TempDir(), "없음.json"))
	defer func() { holidaySet, holidayYear = nil, 0 }()
	if _, err := LoadKRXHolidays(); err == nil {
		t.Error("없는 파일인데 에러가 없다")
	}
}

// validateSchedule 은 스크립트 누락·권한·중복 라벨을 기동 시점에 잡아야 한다.
// 이 검증이 없어 cron이 없는 경로를 가리킨 채 2주간 조용히 실패했다.
func TestValidateSchedule(t *testing.T) {
	root := t.TempDir()
	ok := filepath.Join(root, "ok.sh")
	os.WriteFile(ok, []byte("#!/bin/sh\n"), 0755)
	noexec := filepath.Join(root, "noexec.sh")
	os.WriteFile(noexec, []byte("#!/bin/sh\n"), 0644)

	good := []Task{{Label: "a", Time: "16:30", Script: "ok.sh"}}
	if err := validateSchedule(good, root); err != nil {
		t.Errorf("정상 스케줄이 거부됨: %v", err)
	}

	bad := []struct {
		name  string
		tasks []Task
	}{
		{"스크립트 없음", []Task{{Label: "a", Time: "16:30", Script: "없다.sh"}}},
		{"실행권한 없음", []Task{{Label: "a", Time: "16:30", Script: "noexec.sh"}}},
		{"라벨 중복", []Task{
			{Label: "a", Time: "16:30", Script: "ok.sh"},
			{Label: "a", Time: "17:00", Script: "ok.sh"}}},
		{"시각 형식 오류", []Task{{Label: "a", Time: "25:00", Script: "ok.sh"}}},
		{"라벨 없음", []Task{{Time: "16:30", Script: "ok.sh"}}},
		{"스크립트 미지정", []Task{{Label: "a", Time: "16:30"}}},
		{"없는 선행작업", []Task{{Label: "a", Time: "16:30", Script: "ok.sh", DependsOn: "b"}}},
	}
	for _, c := range bad {
		if err := validateSchedule(c.tasks, root); err == nil {
			t.Errorf("%s: 통과됨 — 거부해야 함", c.name)
		}
	}
}

// 실제 스케줄이 저장소 상태와 맞는지 확인한다 (스크립트 이름이 바뀌면 여기서 깨진다).
func TestBuildScheduleIsValid(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("저장소 루트를 찾지 못함: %v", err)
	}
	if err := validateSchedule(BuildSchedule(), root); err != nil {
		t.Errorf("실제 스케줄 검증 실패: %v", err)
	}
}

// repoRoot 는 go.mod가 있는 디렉토리까지 거슬러 올라간다.
// 테스트 작업 디렉토리 가정에 기대지 않기 위해서다.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// 하루 1회 보장 + 비거래일 스킵.
func TestShouldRunOncePerDay(t *testing.T) {
	holidaySet = nil
	s := &Scheduler{
		tasks:     []Task{{Label: "a", Time: "16:30", Script: "x.sh", Condition: isTradingDay}},
		completed: map[string]time.Time{},
		today:     "20260730",
	}
	task := s.tasks[0]
	now := at("16:30")

	if !s.shouldRun(task, now) {
		t.Fatal("예정 시각인데 실행되지 않음")
	}
	s.completed[s.today+":a"] = now
	if s.shouldRun(task, at("16:31")) {
		t.Error("이미 완료된 작업이 재실행됨")
	}

	// 토요일: 조건 미충족 → 완료로 찍고 스킵
	sat := &Scheduler{tasks: s.tasks, completed: map[string]time.Time{}, today: "20260801"}
	satNow, _ := time.ParseInLocation("2006-01-02 15:04", "2026-08-01 16:30", loc)
	if sat.shouldRun(task, satNow) {
		t.Error("토요일에 실행됨")
	}
	if _, marked := sat.completed["20260801:a"]; !marked {
		t.Error("조건 미충족 작업이 완료로 기록되지 않아 매 tick 재평가된다")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// 월간 리포트는 1일에만, 요일·휴장일과 무관하게 실행돼야 한다.
// 거래일 조건을 걸면 1일이 주말인 달은 리포트가 통째로 빠진다.
func TestIsFirstOfMonth(t *testing.T) {
	holidaySet = map[string]bool{"20260101": true} // 신정 — 휴장일이지만 리포트는 돌아야 함
	defer func() { holidaySet = nil }()

	cases := []struct {
		date string
		want bool
		why  string
	}{
		{"2026-08-01", true, "토요일 1일"},
		{"2026-11-01", true, "일요일 1일"},
		{"2026-01-01", true, "휴장일(신정) 1일"},
		{"2026-09-01", true, "화요일 1일"},
		{"2026-07-31", false, "말일"},
		{"2026-08-02", false, "2일"},
	}
	for _, c := range cases {
		d, _ := time.ParseInLocation("2006-01-02", c.date, loc)
		if got := isFirstOfMonth(d); got != c.want {
			t.Errorf("isFirstOfMonth(%s) = %v, want %v (%s)", c.date, got, c.want, c.why)
		}
	}
}
