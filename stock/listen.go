package stock

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// handleListen 은 "stock listen" — 실시간 큐 소비 모드 (2026-07-09).
//
// 외부 프로그램이 장 마감 동시호가 전(~15:17)에 "과거 249봉 + 가상 금일봉 = 250봉 전체"를
// 종목당 1메시지 JSON으로 발행하면(기본 큐 CMST_st2), 이 데몬이 수신 즉시 두 운용 전략 쌍
// (s03s23 / wdefbox)을 평가해 "금일" 매수·매도 이벤트만 추출한다.
//
// 출력: ① 콘솔 ② 결과 큐(JSON, 기본 CMST_st2_result) ③ han.StrategyTradeLog 건별 적재(멱등).
// W중력 매수에는 밀도 게이트 판정(기동 시 1회 — 게이트는 당일 제외 설계)을 함께 실어 보낸다.
// 장 마감 후 daily_batch.sh가 확정 일봉으로 같은 날짜를 멱등 재적재하므로 잠정→확정 교체는 자동.
//
// 사용:
//
//	./RESTGo stock listen [--queue CMST_st2] [--out-queue CMST_st2_result] [--peek N] [--no-db] [--no-out]
//
// --peek N: 메시지 N건을 소비해 구조만 출력하고 종료 (스키마 검증용 — auto-ack라 소비됨에 유의)
func handleListen(args []string) {
	queue := "CMST_st2"
	outQueue := "CMST_st2_result"
	peek := 0
	useDB, useOut := true, true
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--queue":
			if i+1 < len(args) {
				queue = args[i+1]
				i++
			}
		case "--out-queue":
			if i+1 < len(args) {
				outQueue = args[i+1]
				i++
			}
		case "--peek":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil {
					peek = n
				}
				i++
			}
		case "--no-db":
			useDB = false
		case "--no-out":
			useOut = false
		}
	}

	if err := console.RabbitMQSession.AddConsumerChannel(queue); err != nil {
		fmt.Fprintf(os.Stderr, "[listen] 큐 연결 실패: %v\n", err)
		return
	}
	msgChan := make(chan []byte, 4096)
	if err := console.RabbitMQSession.Receive(queue, msgChan); err != nil {
		fmt.Fprintf(os.Stderr, "[listen] 컨슈머 등록 실패: %v\n", err)
		return
	}
	fmt.Printf("[listen] 큐 %s 소비 시작 (peek=%d)\n", queue, peek)

	// ── peek 모드: 구조 검증만 ─────────────────────────────────────────
	if peek > 0 {
		for n := 0; n < peek; n++ {
			select {
			case body := <-msgChan:
				peekPrint(n+1, body)
			case <-time.After(30 * time.Second):
				fmt.Printf("[listen] 30초 내 메시지 없음 (수신 %d건에서 종료)\n", n)
				return
			}
		}
		return
	}

	// ── 전략 쌍 로드 (매수 룰은 파싱해 보관, 매도는 메시지마다 전역 스위치) ──
	type pair struct {
		label    string // DB StrategyTradeLog.strategy 및 daily_batch 컨벤션과 동일
		buyPath  string
		sellPath string
		rules    []stg.RuleConfig
		settings stg.Settings
	}
	pairs := []*pair{
		{label: "S1_S03S23", buyPath: "rules/strategy1_s03s23.yaml", sellPath: "rules/sell_s03s23.yaml"},
		{label: "W_DefBoxGravity", buyPath: "rules/buy_wdefbox.yaml", sellPath: "rules/sell_wdefbox.yaml"},
	}
	for _, p := range pairs {
		rules, settings, err := stg.LoadRulesWithSettings(p.buyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[listen] 매수 전략 로드 실패 (%s): %v\n", p.buyPath, err)
			return
		}
		p.rules, p.settings = rules, settings
		fmt.Printf("[listen] 전략 %s: 매수 %s (%d룰) + 매도 %s\n", p.label, p.buyPath, len(rules), p.sellPath)
	}

	// ── 밀도 게이트 (W중력 전용) — 당일 제외 설계이므로 기동 시 1회 판정 ──
	today := time.Now().Format("20060102")
	gateNote := "게이트 판정 불가"
	gatePass := false
	if cfg, err := stg.LoadOverlayConfig("rules/overlay_wdefbox.yaml"); err == nil {
		if hanDB, err := console.MsConn.GetDB("han"); err == nil {
			if hist, err := stg.FetchSignalDailyCounts(hanDB, cfg.Strategies); err == nil {
				if gate, err := stg.NewDensityGate(cfg.GateConfig(), hist); err == nil {
					if dec, err := gate.Evaluate(today); err == nil {
						gatePass = dec.Pass
						verdict := "HOLD"
						if dec.Pass {
							verdict = "PASS"
						}
						gateNote = fmt.Sprintf("%s (밀도 %d / 임계 %d)", verdict, dec.Density, dec.Threshold)
					}
				}
			}
		}
	}
	fmt.Printf("[listen] W중력 밀도 게이트 (%s): %s\n", today, gateNote)

	if useOut {
		if err := console.RabbitMQSession.AddChannelAndQueue(outQueue); err != nil {
			fmt.Fprintf(os.Stderr, "[listen] 결과 큐 %s 생성 실패: %v — 큐 출력 비활성\n", outQueue, err)
			useOut = false
		}
	}
	var hanDB = func() *dbHandle {
		if !useDB {
			return nil
		}
		db, err := console.MsConn.GetDB("han")
		if err != nil {
			fmt.Fprintf(os.Stderr, "[listen] han DB 연결 실패: %v — DB 적재 비활성\n", err)
			return nil
		}
		return &dbHandle{db}
	}()

	// ── 이벤트 방출 ───────────────────────────────────────────────────
	type outEvent struct {
		Type      string  `json:"type"` // BUY | SELL
		Strategy  string  `json:"strategy"`
		Shcode    string  `json:"shcode"`
		Hname     string  `json:"hname"`
		TradeDate string  `json:"trade_date"`
		Reason    string  `json:"reason"`
		Weight    float64 `json:"weight"`
		NetReturn float64 `json:"net_return_pct,omitempty"`
		BuyDate   string  `json:"buy_date,omitempty"`
		Gate      string  `json:"gate,omitempty"` // W중력 BUY에만
		AsOf      string  `json:"as_of"`
	}
	nEvents := 0
	emit := func(e outEvent) {
		nEvents++
		gateTag := ""
		if e.Gate != "" {
			gateTag = "  게이트 " + e.Gate
		}
		fmt.Printf("[listen] %s %-15s %s %s (%s) w=%.2f%s\n", e.Type, e.Strategy, e.Shcode, e.Hname, e.Reason, e.Weight, gateTag)
		if useOut {
			_ = console.SendJson(outQueue, e)
		}
		if hanDB != nil {
			hanDB.upsertTradeLog(e.Strategy, e.Shcode, e.Hname, e.Type, e.TradeDate, e.Reason, e.Weight, e.NetReturn, e.BuyDate)
		}
	}

	// ── 메인 루프 ─────────────────────────────────────────────────────
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	connClosed := console.RabbitMQSession.NotifyConnClose() // 끊기면 즉시 종료 → 슈퍼바이저 재기동
	nMsg, nSkip := 0, 0
	fmt.Printf("[listen] 대기 중 — 결과: 콘솔%s%s (Ctrl+C 종료)\n",
		map[bool]string{true: " + 큐 " + outQueue, false: ""}[useOut],
		map[bool]string{true: " + StrategyTradeLog", false: ""}[hanDB != nil])

	for {
		select {
		case <-stop:
			fmt.Printf("\n[listen] 종료 — 메시지 %d건 처리 (스킵 %d), 이벤트 %d건\n", nMsg, nSkip, nEvents)
			return
		case amqpErr := <-connClosed:
			// 소비자 채널은 자동 복구가 안 되므로 좀비로 남지 않게 비정상 종료 (systemd Restart가 재기동)
			fmt.Fprintf(os.Stderr, "[listen] RabbitMQ 연결 끊김: %v — 재기동 필요, 종료(1)\n", amqpErr)
			os.Exit(1)
		case body := <-msgChan:
			msg, candles, err := parseVirtualMsg(body)
			if err != nil {
				nSkip++
				fmt.Fprintf(os.Stderr, "[listen] 메시지 파싱 실패: %v\n", err)
				continue
			}
			if len(candles) < 130 { // MA120 워밍업 불가
				nSkip++
				continue
			}
			indicator.PrepareCandles(candles)
			lastDate := candles[len(candles)-1].Date

			for _, p := range pairs {
				if err := stg.LoadSellStrategyFile(p.sellPath); err != nil {
					fmt.Fprintf(os.Stderr, "[listen] 매도 로드 실패 (%s): %v\n", p.sellPath, err)
					continue
				}
				result := stg.AnalyzeWithRules(candles, p.rules, p.settings)
				for _, sig := range result.BuySignals {
					if sig.Date != lastDate {
						continue
					}
					e := outEvent{Type: "BUY", Strategy: p.label, Shcode: msg.Shcode, Hname: msg.Hname,
						TradeDate: lastDate, Reason: sig.Reason, Weight: 1.0, AsOf: msg.AsOf}
					if p.label == "W_DefBoxGravity" {
						e.Gate = gateNote
						_ = gatePass
					}
					emit(e)
				}
				for _, pos := range result.Positions {
					for _, ex := range pos.SellExecutions {
						if ex.SellDate != lastDate {
							continue
						}
						emit(outEvent{Type: "SELL", Strategy: p.label, Shcode: msg.Shcode, Hname: msg.Hname,
							TradeDate: lastDate, Reason: ex.SellReason, Weight: ex.Weight,
							NetReturn: ex.NetPartialReturn, BuyDate: pos.BuyDate, AsOf: msg.AsOf})
					}
				}
			}
			nMsg++
			if nMsg%200 == 0 {
				fmt.Printf("[listen] %d건 처리 (스킵 %d, 이벤트 %d)\n", nMsg, nSkip, nEvents)
			}
		}
	}
}

// virtualMsg 는 발행측 확정 스키마 (2026-07-09):
// {v, shcode, hname, as_of, bars: [["YYYYMMDD",시,고,저,종,거래량] × ~250]} — 마지막 bar가 가상 금일봉.
type virtualMsg struct {
	V      int               `json:"v"`
	Shcode string            `json:"shcode"`
	Hname  string            `json:"hname"`
	AsOf   string            `json:"as_of"`
	RawBar []json.RawMessage `json:"bars"`
}

func parseVirtualMsg(body []byte) (*virtualMsg, []*box.Candle, error) {
	var m virtualMsg
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, nil, fmt.Errorf("JSON 아님: %w", err)
	}
	if m.Shcode == "" || len(m.RawBar) == 0 {
		return nil, nil, fmt.Errorf("필수 필드 누락 (shcode=%q bars=%d)", m.Shcode, len(m.RawBar))
	}
	candles := make([]*box.Candle, 0, len(m.RawBar))
	for i, raw := range m.RawBar {
		// bar = [date문자열, 시, 고, 저, 종, 거래량] — 혼합 타입이라 개별 디코드
		var arr []interface{}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&arr); err != nil || len(arr) < 6 {
			return nil, nil, fmt.Errorf("bar[%d] 형식 오류 (%v, len=%d)", i, err, len(arr))
		}
		date, ok := arr[0].(string)
		if !ok || len(date) != 8 {
			return nil, nil, fmt.Errorf("bar[%d] 날짜 형식 오류: %v", i, arr[0])
		}
		nums := make([]float64, 5)
		for j := 1; j <= 5; j++ {
			n, ok := arr[j].(json.Number)
			if !ok {
				return nil, nil, fmt.Errorf("bar[%d][%d] 숫자 아님: %v", i, j, arr[j])
			}
			f, err := n.Float64()
			if err != nil {
				return nil, nil, err
			}
			nums[j-1] = f
		}
		candles = append(candles, &box.Candle{
			Shcode: m.Shcode, Date: date,
			OpenOrigin: nums[0], HighOrigin: nums[1], LowOrigin: nums[2], CloseOrigin: nums[3],
			Volume: nums[4],
		})
	}
	return &m, candles, nil
}

func peekPrint(n int, body []byte) {
	m, candles, err := parseVirtualMsg(body)
	if err != nil {
		fmt.Printf("[peek %d] 파싱 실패: %v — 원문 앞부분: %.200s\n", n, err, string(body))
		return
	}
	first, last := candles[0], candles[len(candles)-1]
	fmt.Printf("[peek %d] v=%d %s %s as_of=%s bars=%d\n", n, m.V, m.Shcode, m.Hname, m.AsOf, len(candles))
	fmt.Printf("         첫봉 %s O%.0f H%.0f L%.0f C%.0f V%.0f\n", first.Date, first.OpenOrigin, first.HighOrigin, first.LowOrigin, first.CloseOrigin, first.Volume)
	fmt.Printf("         끝봉 %s O%.0f H%.0f L%.0f C%.0f V%.0f (가상 금일봉)\n", last.Date, last.OpenOrigin, last.HighOrigin, last.LowOrigin, last.CloseOrigin, last.Volume)
}

// dbHandle 은 listen 모드의 han DB 적재 헬퍼.
type dbHandle struct{ db *sql.DB }

// upsertTradeLog 는 StrategyTradeLog에 이벤트를 source='LIVE'로 멱등 적재한다 (재전송 시 동일 키 삭제 후 삽입).
//
// source 규약 (2026-07-09, 사용자 설계): LIVE = 15:17 가상봉 기반 실시간 신호 — **실제 매매의 근거이므로
// 마감 후 확정 재계산(daily_batch, source='EOD')이 절대 덮어쓰지 않는다.** 같은 날짜에 LIVE/EOD 두 벌이
// 공존하며, 둘의 차이가 곧 "가상봉 vs 확정봉 신호 괴리" 모니터링 지표가 된다.
func (h *dbHandle) upsertTradeLog(strategy, shcode, hname, eventType, tradeDate, reason string, weight, netReturn float64, buyDate string) {
	_, _ = h.db.Exec(`DELETE FROM StrategyTradeLog WHERE strategy=@p1 AND shcode=@p2 AND event_type=@p3 AND trade_date=@p4 AND reason=@p5 AND source='LIVE'`,
		strategy, shcode, eventType, tradeDate, reason)
	var nr, bd interface{}
	if eventType == "SELL" {
		nr, bd = netReturn, buyDate
	}
	_, err := h.db.Exec(`INSERT INTO StrategyTradeLog (strategy, shcode, hname, event_type, trade_date, reason, weight, net_return_pct, buy_date, source)
		VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9, 'LIVE')`,
		strategy, shcode, hname, eventType, tradeDate, reason, weight, nr, bd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[listen] TradeLog 적재 실패 (%s %s): %v\n", shcode, eventType, err)
	}
}

// handleFeed 는 "stock feed" — 발신측과 동일한 스키마로 KIS2 일봉을 큐에 발행한다 (2026-07-09).
// 용도: ① 발신 프로그램 없이 셀프 테스트/리허설 ② listen과의 패리티 검증 (확정 일봉을 그대로 발행하면
// listen의 결과가 daily_batch와 일치해야 한다) ③ 다른 소비자 개발 시 테스트 데이터 공급.
//
// 사용: ./RESTGo stock feed [--queue CMST_st2] [--days 250] [--max N]
func handleFeed(args []string) {
	queue := "CMST_st2"
	days := 250
	maxN := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--queue":
			if i+1 < len(args) {
				queue = args[i+1]
				i++
			}
		case "--days":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
					days = n
				}
				i++
			}
		case "--max":
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
					maxN = n
				}
				i++
			}
		}
	}
	// 국내 일봉 소스: hannam (2026-07-09 사용자 지시)
	db, err := console.MsConn.GetDB("han")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[feed] han DB 연결 실패: %v\n", err)
		return
	}
	if err := console.RabbitMQSession.AddConsumerChannel(queue); err != nil {
		// 큐가 없으면 생성 시도
		if err2 := console.RabbitMQSession.AddChannelAndQueue(queue); err2 != nil {
			fmt.Fprintf(os.Stderr, "[feed] 큐 준비 실패: %v / %v\n", err, err2)
			return
		}
	}
	codes, err := box.FetchHannamStockList(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[feed] 종목 조회 실패: %v\n", err)
		return
	}
	names := fetchStockNames()
	type stockInfo struct{ Shcode, Hname string }
	stocks := make([]stockInfo, 0, len(codes))
	for _, c := range codes {
		nm := names[c]
		if nm == "" {
			nm = c
		}
		stocks = append(stocks, stockInfo{c, nm})
	}
	if maxN > 0 && len(stocks) > maxN {
		stocks = stocks[:maxN]
	}
	asOf := time.Now().Format("20060102 15:04:05")
	sent, skip := 0, 0
	for _, s := range stocks {
		candles, err := box.FetchCandlesHannam(db, s.Shcode, days)
		if err != nil || len(candles) < 130 {
			skip++
			continue
		}
		bars := make([][]interface{}, 0, len(candles))
		for _, c := range candles {
			bars = append(bars, []interface{}{c.Date, c.OpenOrigin, c.HighOrigin, c.LowOrigin, c.CloseOrigin, c.Volume})
		}
		msg := map[string]interface{}{"v": 1, "shcode": s.Shcode, "hname": s.Hname, "as_of": asOf, "bars": bars}
		if err := console.SendJson(queue, msg); err != nil {
			fmt.Fprintf(os.Stderr, "[feed] 발행 실패 (%s): %v\n", s.Shcode, err)
			skip++
			continue
		}
		sent++
		if sent%500 == 0 {
			fmt.Printf("[feed] %d건 발행...\n", sent)
		}
	}
	fmt.Printf("[feed] 완료: 큐 %s 로 %d건 발행 (스킵 %d)\n", queue, sent, skip)
}
