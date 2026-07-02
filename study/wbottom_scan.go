package study

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/cond"
	"RESTGo/indicator"
	"RESTGo/stg"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
)

// WBottomExample 은 W바텀 패턴 발화 사례 1건
type WBottomExample struct {
	Market    string `json:"market"`
	Shcode    string `json:"shcode"`
	EntryDate string `json:"entry_date"`
	P1Date    string `json:"p1_date"`
	P2Date    string `json:"p2_date"`
}

// HandleWBottomScan 은 각 시장에서 W바텀 패턴 예시를 수집한다.
// 사용법: ./RESTGo stock wbottom_scan [--foreign-jp|--foreign-cn|--foreign-hk] [n]
// 출력: zpicture/wbottom_examples.json
func HandleWBottomScan(args []string) {
	mode := "hannam"
	maxExamples := 2
	for _, a := range args {
		switch a {
		case "--foreign-jp":
			mode = "foreign-jp"
		case "--foreign-cn":
			mode = "foreign-cn"
		case "--foreign-hk":
			mode = "foreign-hk"
		}
	}

	db, err := console.MsConn.GetDB("KIS2")
	if mode == "hannam" {
		db, err = console.MsConn.GetDB("han")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[wbscan] DB 연결 오류: %v\n", err)
		return
	}

	// 종목 목록
	var stocks []string
	switch mode {
	case "hannam":
		stocks, err = box.FetchHannamStockList(db)
	case "foreign-jp":
		stocks, err = box.FetchForeignStockList(db, []string{"DTS"})
	case "foreign-cn":
		stocks, err = box.FetchForeignStockList(db, []string{"DSZ", "DSH"})
	case "foreign-hk":
		stocks, err = box.FetchForeignStockList(db, []string{"DHK"})
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[wbscan] 종목 목록 오류: %v\n", err)
		return
	}

	market := map[string]string{
		"hannam":     "KR",
		"foreign-jp": "JP",
		"foreign-cn": "CN",
		"foreign-hk": "HK",
	}[mode]

	fmt.Printf("[wbscan] %s 모드 %d개 종목 스캔\n", market, len(stocks))

	s := stg.DefaultSettings()
	var mu sync.Mutex
	var examples []WBottomExample
	var scanned atomic.Int64

	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for _, code := range stocks {
		if func() bool {
			mu.Lock()
			defer mu.Unlock()
			return len(examples) >= maxExamples
		}() {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(shcode string) {
			defer wg.Done()
			defer func() { <-sem }()

			var candles []*box.Candle
			var fetchErr error
			if mode == "hannam" {
				candles, fetchErr = box.FetchCandlesHannam(db, shcode, 300)
			} else {
				candles, fetchErr = box.FetchCandlesForeign(db, shcode, 300)
			}
			if fetchErr != nil || len(candles) < 60 {
				return
			}
			indicator.PrepareCandles(candles)

			// W바텀 패턴 탐색 (DefBox 없이 순수 BB 조건만)
			ctx := &box.TradingContext{CandleList: candles}
			ex := findWBottomInCandles(ctx, s, market, shcode)
			if ex != nil {
				mu.Lock()
				if len(examples) < maxExamples {
					examples = append(examples, *ex)
					fmt.Printf("[wbscan] 발견: %s %s (P1:%s P2:%s entry:%s)\n",
						market, shcode, ex.P1Date, ex.P2Date, ex.EntryDate)
				}
				mu.Unlock()
			}
			scanned.Add(1)
		}(code)
	}
	wg.Wait()

	// JSON 저장
	outPath := "zpicture/wbottom_examples.json"
	type output struct {
		Market   string          `json:"market"`
		Examples []WBottomExample `json:"examples"`
	}
	data, _ := json.MarshalIndent(output{Market: market, Examples: examples}, "", "  ")
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[wbscan] 저장 실패: %v\n", err)
		return
	}
	fmt.Printf("[wbscan] 완료: %s (%d개)\n", outPath, len(examples))
}

// findWBottomInCandles 는 캔들 배열에서 가장 최근 W바텀 패턴을 탐색한다.
// DefBox 없이 순수하게 BB 조건만 사용 (시각화 목적).
func findWBottomInCandles(ctx *box.TradingContext, s stg.Settings, market, shcode string) *WBottomExample {
	candles := ctx.CandleList
	n := len(candles)
	lookback := s.BBWBottomLookback

	// 뒤에서부터 찾아 가장 최근 패턴 반환
	for pos := n - 1; pos >= lookback+20; pos-- {
		ctx.Position = pos
		cur := candles[pos]
		if cur.BollingerUpper == 0 || cur.BollingerUpper <= cur.BollingerLower {
			continue
		}

		// P1: BB 하단 터치
		start := pos - lookback
		if start < 20 {
			start = 20
		}
		p1Pos := -1
		p1PB := 1.0
		for i := start; i < pos-5; i++ {
			c := candles[i]
			if c.BollingerUpper <= c.BollingerLower {
				continue
			}
			if c.Low <= c.BollingerLower && c.BBPercent < p1PB {
				p1Pos = i
				p1PB = c.BBPercent
			}
		}
		if p1Pos < 0 {
			continue
		}

		// 중간 반등
		recovPos := -1
		for i := p1Pos + 1; i < pos-2; i++ {
			c := candles[i]
			if c.BollingerUpper > c.BollingerLower && c.BBPercent >= 0.4 {
				recovPos = i
				break
			}
		}
		if recovPos < 0 {
			continue
		}

		// P2: BB하단 위, %B < 0.5
		p2Pos := -1
		for i := recovPos + 1; i < pos; i++ {
			c := candles[i]
			if c.BollingerUpper <= c.BollingerLower {
				continue
			}
			if c.BBPercent < 0.5 && c.Low > c.BollingerLower {
				p2Pos = i
				break
			}
		}
		if p2Pos < 0 {
			continue
		}

		// 추가 필터: 현재 %B > 0.3 (회복 중)
		if cur.BBPercent < 0.3 {
			continue
		}

		// 추가 필터: IsVolumeBreakout 없이 단순 거래량 확인
		if !cond.IsVolumeBreakout(cur, 2.0) {
			continue
		}

		return &WBottomExample{
			Market:    market,
			Shcode:    shcode,
			EntryDate: cur.Date,
			P1Date:    candles[p1Pos].Date,
			P2Date:    candles[p2Pos].Date,
		}
	}
	return nil
}
