package stock

import (
	"RESTGo/box"
	"RESTGo/console"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CMST_st2 발행기 — LS DB(white MSSQL `LS`) 기반 두 가지 모드.
//
//	publish-hist : 역사적 재생. ST.t8410 일봉을 메모리에 통째로 올리고 창을 밀며 발행 (mode=eod)
//	publish-live : 실전. 어제까지 ST.t8410 + 오늘 ST.t8407 현재가를 합성한 가상 금일봉 (mode 생략 = LIVE)
//
// 스키마 정본: /docs/03-reference/cmst-st2-publisher-spec.md
// 소비자: stock/listen.go (parseVirtualMsg) — 130봉 미만은 조용히 스킵하므로 발행측이 세어야 한다.
//
// 두 모드 모두 일봉 소스는 LS 고정이다 (2026-07-31 사용자 확정). LS는 수정주가 기준이
// KIS2/hannam과 다른 종목이 있으므로(예: 000040) 다른 소스로 만든 백테스트 결과와 직접
// 비교하지 말 것.

const (
	publishSchemaVersion = 1
	minBarsForConsumer   = 130 // listen.go가 이 미만을 스킵한다 (MA120 워밍업 불가)
)

// publishOpts 는 두 모드가 공유하는 설정.
type publishOpts struct {
	queue   string
	days    int
	maxN    int
	codes   []string
	dryRun  bool
	sleepMs int
	verbose bool
}

// publishMsg 는 발행 스키마 v1.
type publishMsg struct {
	V      int             `json:"v"`
	Shcode string          `json:"shcode"`
	Hname  string          `json:"hname"`
	AsOf   string          `json:"as_of"`
	Mode   string          `json:"mode,omitempty"` // "eod"만 의미가 있다. 생략하면 소비자가 LIVE로 태깅
	Bars   [][]interface{} `json:"bars"`
}

// barsToJSON 은 LSBar 슬라이스를 스키마의 위치 배열로 바꾼다.
//
// 날짜는 반드시 문자열, 가격·거래량은 반드시 숫자여야 한다. 하나라도 어기면 소비자가
// 메시지 전체를 버린다 (spec §2 "bar 배열 규격", 2026-07-31 실측 확인).
func barsToJSON(bars []box.LSBar) [][]interface{} {
	out := make([][]interface{}, 0, len(bars))
	for _, b := range bars {
		out = append(out, []interface{}{b.Date, b.Open, b.High, b.Low, b.Close, b.Volume})
	}
	return out
}

// publisher 는 큐 준비와 전송을 감싼다.
type publisher struct {
	queue  string
	dryRun bool
	sent   int
	bytes  int
}

func newPublisher(queue string, dryRun bool) (*publisher, error) {
	if !dryRun {
		// 소비자와 동일하게 durable=false 로 선언한다. 다르면 PRECONDITION_FAILED로 채널이 죽는다.
		if err := console.RabbitMQSession.AddChannelAndQueue(queue); err != nil {
			return nil, fmt.Errorf("큐 %s 준비 실패: %w", queue, err)
		}
	}
	return &publisher{queue: queue, dryRun: dryRun}, nil
}

func (p *publisher) send(msg publishMsg) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("JSON 직렬화 실패 (%s): %w", msg.Shcode, err)
	}
	p.bytes += len(body)
	if p.dryRun {
		p.sent++
		return nil
	}
	// console.SendJson 은 메시지 전문을 로그 큐로 흘린다. 250봉 × 수천 종목이면 로그가
	// 수백 MB가 되므로 여기서는 조용한 경로(Send)를 직접 쓴다.
	if err := console.RabbitMQSession.Send(p.queue, body); err != nil {
		return fmt.Errorf("발행 실패 (%s): %w", msg.Shcode, err)
	}
	p.sent++
	return nil
}

// ── 공통 인수 파싱 ────────────────────────────────────────────────────────

func parsePublishOpts(args []string, extra func(i int, args []string) int) (publishOpts, error) {
	o := publishOpts{queue: "CMST_st2", days: 250}
	for i := 0; i < len(args); i++ {
		consumed := 0
		if extra != nil {
			consumed = extra(i, args)
		}
		if consumed > 0 {
			i += consumed - 1
			continue
		}
		switch args[i] {
		case "--queue":
			if i+1 >= len(args) {
				return o, fmt.Errorf("--queue 뒤에 큐 이름이 필요합니다")
			}
			o.queue = args[i+1]
			i++
		case "--days":
			if i+1 >= len(args) {
				return o, fmt.Errorf("--days 뒤에 숫자가 필요합니다")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < minBarsForConsumer {
				return o, fmt.Errorf("--days 는 %d 이상이어야 합니다 (소비자가 그 미만을 스킵): %s", minBarsForConsumer, args[i+1])
			}
			o.days = n
			i++
		case "--max":
			if i+1 >= len(args) {
				return o, fmt.Errorf("--max 뒤에 숫자가 필요합니다")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n <= 0 {
				return o, fmt.Errorf("--max 는 양수여야 합니다: %s", args[i+1])
			}
			o.maxN = n
			i++
		case "--codes":
			if i+1 >= len(args) {
				return o, fmt.Errorf("--codes 뒤에 종목코드 목록이 필요합니다")
			}
			for _, c := range strings.Split(args[i+1], ",") {
				if c = strings.TrimSpace(c); c != "" {
					o.codes = append(o.codes, c)
				}
			}
			i++
		case "--sleep-ms":
			if i+1 >= len(args) {
				return o, fmt.Errorf("--sleep-ms 뒤에 숫자가 필요합니다")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 0 {
				return o, fmt.Errorf("--sleep-ms 는 0 이상이어야 합니다: %s", args[i+1])
			}
			o.sleepMs = n
			i++
		case "--dry-run":
			o.dryRun = true
		case "--verbose":
			o.verbose = true
		default:
			return o, fmt.Errorf("알 수 없는 인수: %s", args[i])
		}
	}
	return o, nil
}

// selectCodes 는 대상 종목을 정렬·필터링한다.
func selectCodes(all map[string][]box.LSBar, o publishOpts) []string {
	var codes []string
	if len(o.codes) > 0 {
		for _, c := range o.codes {
			if _, ok := all[c]; ok {
				codes = append(codes, c)
			} else {
				fmt.Fprintf(os.Stderr, "[publish] 경고: %s 는 LS 일봉에 없습니다 — 건너뜁니다\n", c)
			}
		}
	} else {
		codes = make([]string, 0, len(all))
		for c := range all {
			codes = append(codes, c)
		}
	}
	sort.Strings(codes)
	if o.maxN > 0 && len(codes) > o.maxN {
		codes = codes[:o.maxN]
	}
	return codes
}

// openLS 는 LS DB 핸들과 종목명 맵을 준비한다.
func openLS() (*sql.DB, map[string]string, error) {
	db, err := console.MsConn.GetDB("LS")
	if err != nil {
		return nil, nil, fmt.Errorf("LS DB 연결 실패: %w", err)
	}
	names, nerr := box.FetchLSStockNames(db)
	if nerr != nil {
		// 종목명은 없어도 발행은 가능하다 (소비자는 hname을 그대로 흘려보낼 뿐).
		fmt.Fprintf(os.Stderr, "[publish] 경고: 종목명 조회 실패 — 코드로 대체합니다: %v\n", nerr)
	}
	return db, names, nil
}

func nameOf(names map[string]string, code string) string {
	if n := names[code]; n != "" {
		return n
	}
	return code
}

// ── ① 역사적 발행기 (백테스트 재생) ──────────────────────────────────────

// handlePublishHist 는 LS 일봉을 메모리에 올려 과거 창을 재생 발행한다.
//
//	./RESTGo stock publish-hist [--queue CMST_st2] [--days 250]
//	    [--as-of YYYYMMDD]            창 끝 날짜 하나만 발행 (기본: 데이터 최신일)
//	    [--from YYYYMMDD --to YYYYMMDD]  거래일마다 창을 밀며 연속 발행
//	    [--codes 005930,000660] [--max N] [--dry-run] [--sleep-ms N]
//
// 전 종목 일봉을 한 번만 읽어 재사용하므로, 여러 날짜를 재생해도 DB 조회는 1회다.
// 모든 메시지에 mode=eod 를 붙인다 — 확정봉이므로 소비자가 EOD로 태깅해야
// daily_batch 결과와 멱등 수렴한다.
func handlePublishHist(args []string) {
	var asOf, from, to string
	o, err := parsePublishOpts(args, func(i int, args []string) int {
		switch args[i] {
		case "--as-of":
			if i+1 < len(args) {
				asOf = args[i+1]
				return 2
			}
		case "--from":
			if i+1 < len(args) {
				from = args[i+1]
				return 2
			}
		case "--to":
			if i+1 < len(args) {
				to = args[i+1]
				return 2
			}
		}
		return 0
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		printPublishHistUsage()
		return
	}
	for _, d := range []string{asOf, from, to} {
		if d != "" && len(d) != 8 {
			fmt.Fprintf(os.Stderr, "오류: 날짜는 YYYYMMDD 8자리여야 합니다: %s\n", d)
			return
		}
	}
	if asOf != "" && (from != "" || to != "") {
		fmt.Fprintln(os.Stderr, "오류: --as-of 와 --from/--to 는 함께 쓸 수 없습니다")
		return
	}

	db, names, err := openLS()
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}

	// 재생 구간의 가장 이른 날짜에서도 days 개 창이 나오도록 넉넉히 당겨 읽는다.
	// (거래일 기준이므로 달력일이 아니라 거래일 수로 계산한다.)
	windowEnd := asOf
	if windowEnd == "" {
		windowEnd = to
	}
	lookback := o.days
	if from != "" {
		lookback = o.days + tradingDaySpan(db, from, windowEnd)
	}
	cutoff, err := box.LSCutoffDate(db, lookback, windowEnd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[publish-hist] LS 일봉 메모리 적재 중 (cutoff=%s, 창 %d봉)...\n", cutoff, o.days)
	started := time.Now()
	all, err := box.FetchAllBarsLS(db, cutoff, windowEnd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}
	nBars := 0
	for _, v := range all {
		nBars += len(v)
	}
	fmt.Printf("[publish-hist] 적재 완료: %d종목 / %d봉 (%.1fs)\n",
		len(all), nBars, time.Since(started).Seconds())

	codes := selectCodes(all, o)
	if len(codes) == 0 {
		fmt.Fprintln(os.Stderr, "오류: 대상 종목이 없습니다")
		return
	}

	// 재생할 날짜 목록 = 데이터에 실제로 존재하는 거래일.
	replayDates := resolveReplayDates(all, asOf, from, to)
	if len(replayDates) == 0 {
		fmt.Fprintln(os.Stderr, "오류: 재생할 거래일이 없습니다 (--as-of/--from/--to 범위 확인)")
		return
	}

	pub, err := newPublisher(o.queue, o.dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[publish-hist] 큐 %s 로 %d종목 × %d거래일 발행 시작%s\n",
		o.queue, len(codes), len(replayDates), map[bool]string{true: " (DRY RUN — 전송 안 함)"}[o.dryRun])

	skipShort, skipNoBar := 0, 0
	for _, d := range replayDates {
		asOfStamp := d + " 15:30:00" // 확정봉이므로 장 마감 시각으로 표기
		for _, code := range codes {
			w := windowEndingAt(all[code], d, o.days)
			if len(w) == 0 {
				skipNoBar++
				continue
			}
			// 창의 마지막 봉이 재생일과 다르면 그 종목은 그날 거래가 없었다는 뜻이다.
			// 소비자는 날짜를 검사하지 않으므로 발행측이 걸러야 한다.
			if w[len(w)-1].Date != d {
				skipNoBar++
				continue
			}
			if len(w) < minBarsForConsumer {
				skipShort++
				continue
			}
			msg := publishMsg{V: publishSchemaVersion, Shcode: code, Hname: nameOf(names, code),
				AsOf: asOfStamp, Mode: "eod", Bars: barsToJSON(w)}
			if err := pub.send(msg); err != nil {
				fmt.Fprintf(os.Stderr, "[publish-hist] %v\n", err)
				continue
			}
			if o.sleepMs > 0 {
				time.Sleep(time.Duration(o.sleepMs) * time.Millisecond)
			}
		}
		fmt.Printf("[publish-hist] %s 완료 — 누적 발행 %d건\n", d, pub.sent)
	}
	fmt.Printf("[publish-hist] 종료: %d건 발행 (%.1f MB), 스킵 %d건 (130봉 미만 %d / 해당일 봉 없음 %d)\n",
		pub.sent, float64(pub.bytes)/1024/1024, skipShort+skipNoBar, skipShort, skipNoBar)
}

// resolveReplayDates 는 실제 데이터에 존재하는 거래일 중 재생 대상을 고른다.
func resolveReplayDates(all map[string][]box.LSBar, asOf, from, to string) []string {
	seen := map[string]struct{}{}
	for _, bars := range all {
		for _, b := range bars {
			seen[b.Date] = struct{}{}
		}
	}
	dates := make([]string, 0, len(seen))
	for d := range seen {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	if len(dates) == 0 {
		return nil
	}
	if from == "" && to == "" {
		target := asOf
		if target == "" {
			return dates[len(dates)-1:] // 최신 거래일 하나
		}
		// as-of 이하의 가장 가까운 거래일
		for i := len(dates) - 1; i >= 0; i-- {
			if dates[i] <= target {
				return dates[i : i+1]
			}
		}
		return nil
	}
	var out []string
	for _, d := range dates {
		if from != "" && d < from {
			continue
		}
		if to != "" && d > to {
			continue
		}
		out = append(out, d)
	}
	return out
}

// windowEndingAt 은 date 이하의 마지막 n봉을 돌려준다 (원본 슬라이스를 복사하지 않는다).
func windowEndingAt(bars []box.LSBar, date string, n int) []box.LSBar {
	if len(bars) == 0 {
		return nil
	}
	// bars 는 날짜 오름차순이므로 이진탐색으로 date 이하의 마지막 인덱스를 찾는다.
	hi := sort.Search(len(bars), func(i int) bool { return bars[i].Date > date })
	if hi == 0 {
		return nil
	}
	lo := hi - n
	if lo < 0 {
		lo = 0
	}
	return bars[lo:hi]
}

// tradingDaySpan 은 from~to 사이 거래일 수를 센다 (없으면 0).
func tradingDaySpan(db *sql.DB, from, to string) int {
	const q = `SELECT COUNT(DISTINCT date) FROM ST.t8410
	           WHERE in_gubun='2' AND in_sujung='Y' AND date >= @p1 AND (@p2 = '' OR date <= @p2)`
	var n int
	if err := db.QueryRow(q, from, to).Scan(&n); err != nil {
		return 0
	}
	return n
}

func printPublishHistUsage() {
	fmt.Println("사용법:")
	fmt.Println("  ./RESTGo stock publish-hist [--queue CMST_st2] [--days 250]")
	fmt.Println("      [--as-of YYYYMMDD]                 창 끝 날짜 하나만 발행 (기본: 최신 거래일)")
	fmt.Println("      [--from YYYYMMDD --to YYYYMMDD]    거래일마다 창을 밀며 연속 재생")
	fmt.Println("      [--codes 005930,000660] [--max N] [--sleep-ms N] [--dry-run]")
}

// ── ② 실전 발행기 (장 마감 직전 가상 금일봉) ────────────────────────────

// handlePublishLive 는 어제까지의 LS 일봉에 오늘 현재가를 붙여 발행한다.
//
//	./RESTGo stock publish-live [--queue CMST_st2] [--days 250]
//	    [--date YYYYMMDD]          금일봉 날짜 (기본: 오늘)
//	    [--max-quote-age-min N]    현재가 스냅샷 허용 나이, 기본 30분
//	    [--allow-gap]              과거봉 끝이 직전 거래일이 아니어도 발행 (기본: 스킵)
//	    [--codes ...] [--max N] [--dry-run] [--sleep-ms N]
//
// mode 를 붙이지 않으므로 소비자가 마지막 봉을 LIVE로 태깅한다 (실매매 근거 = 불가침 행).
//
// 갭 가드: 과거봉의 마지막 날짜가 "직전 거래일"이 아니면 그 종목을 건너뛴다.
// LS 일봉은 주간 배치라 종목별로 며칠씩 밀릴 수 있는데, 구멍이 있는 채로 250봉을 만들면
// MA·ATR이 조용히 틀어져 신호가 그럴듯하게 잘못 나온다. 발행 안 하는 쪽이 안전하다.
func handlePublishLive(args []string) {
	today := time.Now().Format("20060102")
	maxAgeMin := 30
	allowGap := false
	o, err := parsePublishOpts(args, func(i int, args []string) int {
		switch args[i] {
		case "--date":
			if i+1 < len(args) {
				today = args[i+1]
				return 2
			}
		case "--max-quote-age-min":
			if i+1 < len(args) {
				if n, e := strconv.Atoi(args[i+1]); e == nil && n > 0 {
					maxAgeMin = n
				}
				return 2
			}
		case "--allow-gap":
			allowGap = true
			return 1
		}
		return 0
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		printPublishLiveUsage()
		return
	}
	if len(today) != 8 {
		fmt.Fprintf(os.Stderr, "오류: --date 는 YYYYMMDD 8자리여야 합니다: %s\n", today)
		return
	}

	db, names, err := openLS()
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}

	// ① 오늘 현재가 스냅샷 (ST.t8407 — 50종목/콜로 수집된 것)
	quotes, err := box.FetchLSQuotes(db, today)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}
	if len(quotes) == 0 {
		fmt.Fprintf(os.Stderr, "오류: %s 자 ST.t8407 현재가가 없습니다 — 멀티 현재가 수집이 먼저 돌아야 합니다\n", today)
		os.Exit(1)
	}
	newest, oldest := quoteAgeRange(quotes)
	ageMin := time.Since(oldest).Minutes()
	fmt.Printf("[publish-live] 현재가 %d종목 (수집 %s ~ %s, 가장 오래된 것 %.1f분 전)\n",
		len(quotes), oldest.Format("15:04:05"), newest.Format("15:04:05"), ageMin)
	if ageMin > float64(maxAgeMin) {
		fmt.Fprintf(os.Stderr, "오류: 현재가 스냅샷이 %.1f분 전으로 허용치(%d분)를 넘었습니다 — 수집을 다시 돌리세요\n",
			ageMin, maxAgeMin)
		os.Exit(1)
	}

	// ② 과거봉: 오늘 이전 거래일까지. days-1 개만 있으면 되지만 여유를 두고 읽는다.
	prevDay, err := lsPrevTradingDate(db, today)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}
	cutoff, err := box.LSCutoffDate(db, o.days, prevDay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[publish-live] LS 일봉 메모리 적재 중 (직전 거래일=%s, cutoff=%s)...\n", prevDay, cutoff)
	all, err := box.FetchAllBarsLS(db, cutoff, prevDay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[publish-live] 적재 완료: %d종목\n", len(all))

	codes := selectCodes(all, o)
	pub, err := newPublisher(o.queue, o.dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[publish-live] 큐 %s 로 발행 시작 — 금일봉 %s%s\n",
		o.queue, today, map[bool]string{true: " (DRY RUN — 전송 안 함)"}[o.dryRun])

	asOfStamp := time.Now().Format("20060102 15:04:05")
	var skipNoQuote, skipGap, skipShort int
	for _, code := range codes {
		q, ok := quotes[code]
		if !ok {
			skipNoQuote++
			continue
		}
		hist := windowEndingAt(all[code], prevDay, o.days-1)
		if len(hist) == 0 {
			skipShort++
			continue
		}
		if last := hist[len(hist)-1].Date; last != prevDay {
			// 이 종목의 LS 일봉이 밀려 있다 — 붙이면 시계열에 구멍이 생긴다.
			if !allowGap {
				skipGap++
				continue
			}
			if o.verbose {
				fmt.Fprintf(os.Stderr, "[publish-live] 경고: %s 과거봉 끝 %s ≠ 직전 거래일 %s (--allow-gap)\n", code, last, prevDay)
			}
		}
		bars := make([]box.LSBar, 0, len(hist)+1)
		bars = append(bars, hist...)
		bars = append(bars, q.VirtualBar(today))
		if len(bars) < minBarsForConsumer {
			skipShort++
			continue
		}
		hname := q.Hname
		if hname == "" {
			hname = nameOf(names, code)
		}
		// mode 를 넣지 않는다 → 소비자가 마지막 봉을 LIVE로 태깅 (spec §4)
		msg := publishMsg{V: publishSchemaVersion, Shcode: code, Hname: hname,
			AsOf: asOfStamp, Bars: barsToJSON(bars)}
		if err := pub.send(msg); err != nil {
			fmt.Fprintf(os.Stderr, "[publish-live] %v\n", err)
			continue
		}
		if o.sleepMs > 0 {
			time.Sleep(time.Duration(o.sleepMs) * time.Millisecond)
		}
	}
	fmt.Printf("[publish-live] 종료: %d건 발행 (%.1f MB)\n", pub.sent, float64(pub.bytes)/1024/1024)
	fmt.Printf("[publish-live] 스킵: 현재가 없음 %d / 일봉 갭 %d / 봉 부족 %d\n", skipNoQuote, skipGap, skipShort)
	if skipGap > 0 {
		fmt.Printf("[publish-live] ★ 일봉 갭 %d종목 — LS 일봉(stock-daily)이 %s까지 적재됐는지 확인하세요. "+
			"의도적으로 무시하려면 --allow-gap\n", skipGap, prevDay)
	}
}

// quoteAgeRange 는 현재가 스냅샷의 최신·최고령 수집 시각을 돌려준다.
func quoteAgeRange(quotes map[string]box.LSQuote) (newest, oldest time.Time) {
	for _, q := range quotes {
		if newest.IsZero() || q.RegAt.After(newest) {
			newest = q.RegAt
		}
		if oldest.IsZero() || q.RegAt.Before(oldest) {
			oldest = q.RegAt
		}
	}
	return
}

// lsPrevTradingDate 는 date 직전의 거래일(= LS 일봉에 존재하는 가장 최근 날짜)을 돌려준다.
func lsPrevTradingDate(db *sql.DB, date string) (string, error) {
	var d sql.NullString
	err := db.QueryRow(`SELECT MAX(date) FROM ST.t8410
	                    WHERE in_gubun='2' AND in_sujung='Y' AND date < @p1`, date).Scan(&d)
	if err != nil {
		return "", fmt.Errorf("직전 거래일 조회 실패: %w", err)
	}
	if !d.Valid {
		return "", fmt.Errorf("%s 이전 LS 일봉이 없습니다", date)
	}
	return d.String, nil
}

func printPublishLiveUsage() {
	fmt.Println("사용법:")
	fmt.Println("  ./RESTGo stock publish-live [--queue CMST_st2] [--days 250]")
	fmt.Println("      [--date YYYYMMDD]          금일봉 날짜 (기본: 오늘)")
	fmt.Println("      [--max-quote-age-min 30]   현재가 스냅샷 허용 나이")
	fmt.Println("      [--allow-gap]              과거봉 끝이 직전 거래일이 아니어도 발행")
	fmt.Println("      [--codes 005930,000660] [--max N] [--sleep-ms N] [--dry-run] [--verbose]")
}
