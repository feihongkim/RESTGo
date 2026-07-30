package scheduler

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"RESTGo/console"
)

var loc *time.Location

func init() {
	var err error
	loc, err = time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.FixedZone("KST", 9*3600)
	}
}

const pidFile = "zpicture/scheduler.pid"

// Scheduler 는 스케줄 상태를 관리한다.
type Scheduler struct {
	tasks     []Task
	completed map[string]time.Time // "YYYYMMDD:label" → 완료 시각
	today     string
	root      string // 프로젝트 루트 (스크립트 실행 디렉토리)
	stopCh    chan struct{}
}

// Handle 은 "scheduler [status|stop]" 서브커맨드를 처리한다.
func Handle(args []string) {
	if len(args) > 0 {
		switch args[0] {
		case "status":
			status()
			return
		case "stop":
			stop()
			return
		case "list":
			listSchedule()
			return
		default:
			fmt.Printf("알 수 없는 scheduler 명령: %s\n", args[0])
			fmt.Println("사용법: ./RESTGo scheduler [status|stop|list]")
			return
		}
	}
	run()
}

func run() {
	root, err := projectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[scheduler] 프로젝트 루트 확인 실패: %v\n", err)
		os.Exit(1)
	}

	s := &Scheduler{
		tasks:     BuildSchedule(),
		completed: make(map[string]time.Time),
		today:     time.Now().In(loc).Format("20060102"),
		root:      root,
		stopCh:    make(chan struct{}),
	}

	// 스크립트 존재를 기동 즉시 확인한다. 이 검증이 없어서 cron이 없는 경로를 가리킨 채
	// 2주간 조용히 실패했다 (2026-07-17~30). 발동 시각까지 기다렸다 실패하면 늦다.
	if err := validateSchedule(s.tasks, root); err != nil {
		fmt.Fprintf(os.Stderr, "[scheduler] 스케줄 구성 오류: %v\n", err)
		os.Exit(1)
	}
	if isRunning() {
		fmt.Fprintln(os.Stderr, "[scheduler] 이미 실행 중입니다 (./RESTGo scheduler status)")
		os.Exit(1)
	}

	holidayInfo, hErr := LoadKRXHolidays()
	writePID()
	defer os.Remove(pidFile)

	logf("스케줄러 시작 (PID %d, root %s)", os.Getpid(), root)
	if hErr != nil {
		logf("★경고: 휴장일 처리 제한 — %v", hErr)
		notify("⚠️ *RESTGo 스케줄러* 휴장일 목록 문제\n%v", hErr)
	}
	if holidayInfo != "" {
		logf("휴장일: %s", holidayInfo)
	}
	for _, t := range s.tasks {
		logf("  - %-12s %s  %s", t.Label, t.Time, t.Script)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		logf("종료 시그널 수신")
		close(s.stopCh)
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	s.tick()
	for {
		select {
		case <-s.stopCh:
			logf("스케줄러 종료")
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	now := time.Now().In(loc)
	if day := now.Format("20060102"); day != s.today {
		s.completed = make(map[string]time.Time)
		s.today = day
		logf("날짜 변경: %s", day)
	}
	for _, t := range s.tasks {
		if s.shouldRun(t, now) {
			s.dispatch(t, now)
		}
	}
}

func (s *Scheduler) shouldRun(task Task, now time.Time) bool {
	key := s.today + ":" + task.Label
	if _, done := s.completed[key]; done {
		return false
	}
	// 조건 미충족(휴장일 등)은 그날 완료로 찍어 매 tick 재평가를 막는다.
	if task.Condition != nil && !task.Condition(now) {
		s.completed[key] = now
		logf("%s 스킵 — 비거래일 (%s)", task.Label, s.today)
		return false
	}
	if task.DependsOn != "" {
		// 선행 작업이 늦어질 수 있으므로 예정 시각 이후면 계속 대기·재시도한다.
		if !isPastTime(task.Time, now) {
			return false
		}
		if _, done := s.completed[s.today+":"+task.DependsOn]; !done {
			return false
		}
		return true
	}
	return inTimeWindow(task.Time, now)
}

// execScript 는 테스트에서 교체하는 seam.
var execScript = (*Scheduler).runScript

func (s *Scheduler) dispatch(task Task, now time.Time) {
	s.completed[s.today+":"+task.Label] = now
	logf("▶ %s 시작 (%s)", task.Label, task.Script)
	go func() {
		defer recoverTask(task)
		start := time.Now()
		err := execScript(s, task)
		elapsed := time.Since(start).Round(time.Second)
		if err != nil {
			logf("✗ %s 실패 (%s): %v", task.Label, elapsed, err)
			// cron은 실패를 알려주지 않아 2주간 방치됐다. 스케줄러는 반드시 알린다.
			notify("🔥 *RESTGo 배치 실패*\n작업: %s\n소요: %s\n%v\n로그: %s",
				task.Label, elapsed, err, task.LogFile)
			return
		}
		logf("✓ %s 완료 (%s)", task.Label, elapsed)
	}()
}

func (s *Scheduler) runScript(task Task) error {
	cmd := exec.Command("./"+task.Script, task.Args...)
	cmd.Dir = s.root
	out := os.Stdout
	if task.LogFile != "" {
		path := filepath.Join(s.root, task.LogFile)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("로그 디렉토리 생성 실패: %w", err)
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("로그 파일 열기 실패: %w", err)
		}
		defer f.Close()
		fmt.Fprintf(f, "\n===== %s 시작 %s =====\n", task.Label,
			time.Now().In(loc).Format("2006-01-02 15:04:05"))
		out = f
	}
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// recoverTask 는 작업 goroutine 패닉을 복구한다. 한 작업의 패닉으로 스케줄러
// 프로세스 전체가 죽으면 나머지 배치도 함께 멈춘다.
func recoverTask(task Task) {
	r := recover()
	if r == nil {
		return
	}
	logf("%s 패닉: %v\n%s", task.Label, r, debug.Stack())
	notify("🔥 *RESTGo 스케줄러* %s 작업 패닉\n%v", task.Label, r)
}

// validateSchedule 은 라벨 중복·시각 형식·스크립트 존재를 기동 시점에 확인한다.
func validateSchedule(tasks []Task, root string) error {
	seen := map[string]bool{}
	for _, t := range tasks {
		if t.Label == "" {
			return fmt.Errorf("라벨 없는 태스크가 있습니다")
		}
		if seen[t.Label] {
			return fmt.Errorf("라벨 중복: %q (완료 기록이 겹쳐 하나만 실행됩니다)", t.Label)
		}
		seen[t.Label] = true
		if _, _, err := parseHM(t.Time); err != nil {
			return fmt.Errorf("태스크 %q 의 시각 %q: %w", t.Label, t.Time, err)
		}
		if t.Script == "" {
			return fmt.Errorf("태스크 %q 에 스크립트가 없습니다", t.Label)
		}
		path := filepath.Join(root, t.Script)
		st, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("태스크 %q 의 스크립트를 찾을 수 없습니다: %s", t.Label, path)
		}
		if st.Mode()&0111 == 0 {
			return fmt.Errorf("태스크 %q 의 스크립트에 실행 권한이 없습니다: %s", t.Label, path)
		}
	}
	for _, t := range tasks {
		if t.DependsOn != "" && !seen[t.DependsOn] {
			return fmt.Errorf("태스크 %q 의 선행 작업 %q 가 스케줄에 없습니다", t.Label, t.DependsOn)
		}
	}
	return nil
}

// ── 시각 판정 ────────────────────────────────────────────────────────────

func parseHM(hm string) (int, int, error) {
	parts := strings.Split(hm, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("형식은 HH:MM")
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0, fmt.Errorf("시(hour)가 0~23이 아님")
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("분(minute)이 0~59가 아님")
	}
	return h, m, nil
}

// inTimeWindow 는 예정 시각부터 2분 이내인지 판정한다 (tick 30초 간격 대비 여유).
func inTimeWindow(target string, now time.Time) bool {
	h, m, err := parseHM(target)
	if err != nil {
		return false
	}
	t := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, loc)
	diff := now.Sub(t)
	return diff >= 0 && diff < 2*time.Minute
}

// isPastTime 은 예정 시각을 지났는지 판정한다 (DependsOn 대기용).
func isPastTime(target string, now time.Time) bool {
	h, m, err := parseHM(target)
	if err != nil {
		return false
	}
	return !now.Before(time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, loc))
}

// ── 프로세스 제어 ────────────────────────────────────────────────────────

func projectRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

func writePID() {
	_ = os.MkdirAll(filepath.Dir(pidFile), 0755)
	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0644)
}

func readPID() (int, error) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func isRunning() bool {
	pid, err := readPID()
	return err == nil && processAlive(pid)
}

func status() {
	pid, err := readPID()
	if err != nil {
		fmt.Println("[scheduler] 중지됨")
		return
	}
	if processAlive(pid) {
		fmt.Printf("[scheduler] 실행 중 (PID %d)\n", pid)
	} else {
		fmt.Printf("[scheduler] 중지됨 (stale PID %d 정리)\n", pid)
		os.Remove(pidFile)
	}
}

func stop() {
	pid, err := readPID()
	if err != nil || !processAlive(pid) {
		fmt.Println("[scheduler] 실행 중이 아닙니다")
		os.Remove(pidFile)
		return
	}
	proc, _ := os.FindProcess(pid)
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "[scheduler] 종료 신호 전송 실패: %v\n", err)
		return
	}
	fmt.Printf("[scheduler] 종료 신호 전송 (PID %d)\n", pid)
}

// listSchedule 은 등록된 스케줄과 검증 결과를 출력한다 (기동 전 점검용).
func listSchedule() {
	root, err := projectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "프로젝트 루트 확인 실패: %v\n", err)
		return
	}
	info, hErr := LoadKRXHolidays()
	if info != "" {
		fmt.Printf("휴장일: %s\n", info)
	}
	if hErr != nil {
		fmt.Printf("★경고: %v\n", hErr)
	}
	tasks := BuildSchedule()
	fmt.Printf("\n%-14s %-6s %-22s %s\n", "라벨", "시각", "스크립트", "로그")
	fmt.Println(strings.Repeat("-", 78))
	for _, t := range tasks {
		fmt.Printf("%-14s %-6s %-22s %s\n", t.Label, t.Time, t.Script, t.LogFile)
	}
	now := time.Now().In(loc)
	fmt.Printf("\n오늘(%s) 거래일 여부: %v\n", now.Format("2006-01-02"), isTradingDay(now))
	if err := validateSchedule(tasks, root); err != nil {
		fmt.Printf("검증: ✗ %v\n", err)
		return
	}
	fmt.Println("검증: ✓ 전 태스크 스크립트 존재·실행권한 확인")
}

// ── 로그·알림 ────────────────────────────────────────────────────────────

func logf(format string, v ...interface{}) {
	fmt.Printf("[%s] [scheduler] %s\n",
		time.Now().In(loc).Format("2006-01-02 15:04:05"), fmt.Sprintf(format, v...))
}

// notify 는 테스트에서 교체하는 seam.
var notify = func(format string, v ...interface{}) {
	console.Tele(format, v...)
}
