package stock

import (
	"RESTGo/box"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// 15분봉 기준: 1일 = 96봉, 1개월 ≈ 2,880봉
const (
	wfBarsPerMonth = 2880
	wfISBars       = 6 * wfBarsPerMonth // 17,280
	wfOOSBars      = 2 * wfBarsPerMonth // 5,760
	wfStrideBars   = wfOOSBars          // OOS 길이만큼 시프트
	wfWarmup       = 100
)

// WFWindow 는 하나의 IS+OOS 윈도우 결과
type WFWindow struct {
	Index     int     `json:"index"`
	ISFrom    string  `json:"is_from"`
	ISTo      string  `json:"is_to"`
	OOSFrom   string  `json:"oos_from"`
	OOSTo     string  `json:"oos_to"`
	ISTrades  int     `json:"is_trades"`
	ISWinRate float64 `json:"is_win_rate"`
	ISAvgNet  float64 `json:"is_avg_net_return_pct"`
	ISPF      float64 `json:"is_profit_factor"`
	ISMDD     float64 `json:"is_max_drawdown_pct"`
	OOSTrades int     `json:"oos_trades"`
	OOSWinRate float64 `json:"oos_win_rate"`
	OOSAvgNet float64 `json:"oos_avg_net_return_pct"`
	OOSPF     float64 `json:"oos_profit_factor"`
	OOSMDD    float64 `json:"oos_max_drawdown_pct"`
	// 성과비: OOS / IS (avgNet, PF). 분모 0/음수면 nil
	RatioAvgNet *float64 `json:"ratio_avg_net"`
	RatioPF     *float64 `json:"ratio_pf"`
}

// WFOutput 은 walk-forward 결과 전체
type WFOutput struct {
	GeneratedAt   string     `json:"generated_at"`
	Strategy      string     `json:"strategy"`
	Market        string     `json:"market"`
	TotalCandles  int        `json:"total_candles"`
	ISBars        int        `json:"is_bars"`
	OOSBars       int        `json:"oos_bars"`
	StrideBars    int        `json:"stride_bars"`
	Windows       []WFWindow `json:"windows"`
	// 종합 통계: 윈도우당 OOS/IS 성과비의 중앙값·평균
	MedianRatioPF     float64 `json:"median_ratio_pf"`
	MeanRatioPF       float64 `json:"mean_ratio_pf"`
	MedianRatioAvgNet float64 `json:"median_ratio_avg_net"`
	MeanRatioAvgNet   float64 `json:"mean_ratio_avg_net"`
	WindowsPassed     int     `json:"windows_passed_oos_gate"` // OOS PF ≥ 1.3 ∧ avgNet ≥ 0.30
}

// handleWalkForward 는 "stock walkforward" 명령 진입점
//
// 사용법: ./RESTGo stock walkforward <market> [output_json] [strategy_yaml]
// 예) ./RESTGo stock walkforward KRW-ETH zpicture/wf_eth_t11d.json rules/strategy3.yaml
func handleWalkForward(args []string) {
	if len(args) == 0 {
		fmt.Println("사용법: ./RESTGo stock walkforward <market> [output_json] [strategy_yaml]")
		return
	}
	market := args[0]
	outputPath := "zpicture/wf_" + strings.ToLower(strings.ReplaceAll(market, "-", "_")) + ".json"
	stratPath := "rules/strategy3.yaml"
	if len(args) >= 2 && args[1] != "" {
		outputPath = args[1]
	}
	if len(args) >= 3 && args[2] != "" {
		stratPath = args[2]
	}
	runWalkForward(market, stratPath, outputPath)
}

func runWalkForward(market, stratPath, outputPath string) {
	db, err := console.MsConn.GetDB("tuf")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[wf] tuf DB 연결 실패: %v\n", err)
		return
	}

	rules, settings, err := stg.LoadRulesWithSettings(stratPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[wf] 전략 로드 실패: %v\n", err)
		return
	}
	fmt.Printf("[wf] 전략: %s  룰: %d개  마켓: %s\n", stratPath, len(rules), market)

	candles, err := box.FetchUpbitCandles15m(db, market, 400000)
	if err != nil || len(candles) < wfWarmup+wfISBars+wfOOSBars {
		fmt.Fprintf(os.Stderr, "[wf] %s 캔들 부족 (%d봉, 최소 %d 필요)\n", market, len(candles), wfWarmup+wfISBars+wfOOSBars)
		return
	}
	indicator.PrepareCandles(candles)
	fmt.Printf("[wf] 캔들 %d봉 로드, IS %d봉 + OOS %d봉, stride %d봉\n", len(candles), wfISBars, wfOOSBars, wfStrideBars)

	var windows []WFWindow
	idx := 0
	// 첫 윈도우 시작: warmup 이후
	for isStart := wfWarmup; isStart+wfISBars+wfOOSBars <= len(candles); isStart += wfStrideBars {
		isEnd := isStart + wfISBars
		oosEnd := isEnd + wfOOSBars

		isSlice := candles[isStart:isEnd]
		oosSlice := candles[isEnd:oosEnd]

		isResult := stg.AnalyzeWithRules(isSlice, rules, settings)
		oosResult := stg.AnalyzeWithRules(oosSlice, rules, settings)

		isStats := computeWFStats(isResult.Positions)
		oosStats := computeWFStats(oosResult.Positions)

		w := WFWindow{
			Index:      idx,
			ISFrom:     candles[isStart].Date,
			ISTo:       candles[isEnd-1].Date,
			OOSFrom:    candles[isEnd].Date,
			OOSTo:      candles[oosEnd-1].Date,
			ISTrades:   isStats.Trades,
			ISWinRate:  isStats.WinRate,
			ISAvgNet:   isStats.AvgNet,
			ISPF:       isStats.PF,
			ISMDD:      isStats.MDD,
			OOSTrades:  oosStats.Trades,
			OOSWinRate: oosStats.WinRate,
			OOSAvgNet:  oosStats.AvgNet,
			OOSPF:      oosStats.PF,
			OOSMDD:     oosStats.MDD,
		}
		if isStats.AvgNet > 0 {
			r := oosStats.AvgNet / isStats.AvgNet
			w.RatioAvgNet = &r
		}
		if isStats.PF > 0 {
			r := oosStats.PF / isStats.PF
			w.RatioPF = &r
		}
		windows = append(windows, w)
		fmt.Printf("[wf] win#%d IS %s~%s (n=%d PF=%.2f avgNet=%.3f) → OOS %s~%s (n=%d PF=%.2f avgNet=%.3f)\n",
			idx, w.ISFrom, w.ISTo, w.ISTrades, w.ISPF, w.ISAvgNet,
			w.OOSFrom, w.OOSTo, w.OOSTrades, w.OOSPF, w.OOSAvgNet)
		idx++
	}

	if len(windows) == 0 {
		fmt.Fprintf(os.Stderr, "[wf] 유효 윈도우 없음 (캔들 %d봉)\n", len(candles))
		return
	}

	// 종합 통계
	var pfRatios, avgRatios []float64
	passCount := 0
	for _, w := range windows {
		if w.RatioPF != nil {
			pfRatios = append(pfRatios, *w.RatioPF)
		}
		if w.RatioAvgNet != nil {
			avgRatios = append(avgRatios, *w.RatioAvgNet)
		}
		if w.OOSPF >= 1.3 && w.OOSAvgNet >= 0.30 {
			passCount++
		}
	}

	out := WFOutput{
		GeneratedAt:       time.Now().Format(time.RFC3339),
		Strategy:          stratPath,
		Market:            market,
		TotalCandles:      len(candles),
		ISBars:            wfISBars,
		OOSBars:           wfOOSBars,
		StrideBars:        wfStrideBars,
		Windows:           windows,
		MedianRatioPF:     medianOf(pfRatios),
		MeanRatioPF:       meanOf(pfRatios),
		MedianRatioAvgNet: medianOf(avgRatios),
		MeanRatioAvgNet:   meanOf(avgRatios),
		WindowsPassed:     passCount,
	}

	if err := os.MkdirAll(dirOf(outputPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[wf] 디렉토리 생성 실패: %v\n", err)
		return
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[wf] JSON 직렬화 실패: %v\n", err)
		return
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[wf] 파일 저장 실패: %v\n", err)
		return
	}

	fmt.Printf("\n[wf] 완료: %s\n", outputPath)
	fmt.Printf("  윈도우 수: %d, OOS 게이트 통과: %d (%.1f%%)\n", len(windows), passCount, float64(passCount)/float64(len(windows))*100)
	fmt.Printf("  PF 성과비 (OOS/IS) 중앙값: %.3f, 평균: %.3f\n", out.MedianRatioPF, out.MeanRatioPF)
	fmt.Printf("  avgNet 성과비 (OOS/IS) 중앙값: %.3f, 평균: %.3f\n", out.MedianRatioAvgNet, out.MeanRatioAvgNet)
}

type wfStats struct {
	Trades  int
	WinRate float64
	AvgNet  float64
	PF      float64
	MDD     float64
}

// computeWFStats 는 청산 완료 포지션 목록에서 메트릭을 계산한다.
func computeWFStats(positions []*box.TradePosition) wfStats {
	var trades []box.TradePosition
	for _, p := range positions {
		if p.IsActive || len(p.SellExecutions) == 0 {
			continue
		}
		trades = append(trades, *p)
	}
	st := wfStats{Trades: len(trades)}
	if len(trades) == 0 {
		return st
	}
	var wins int
	var grossWin, grossLoss, cumRet float64
	equity, peak, maxDD := 1.0, 1.0, 0.0
	for _, t := range trades {
		net := t.NetReturnRate
		if net == 0 {
			net = t.ReturnRate
		}
		cumRet += net
		if net > 0 {
			wins++
			grossWin += net
		} else {
			grossLoss += -net
		}
		equity *= 1.0 + net/100.0
		if equity <= 0 {
			equity = 0
			maxDD = 1.0
		} else {
			if equity > peak {
				peak = equity
			}
			if dd := (peak - equity) / peak; dd > maxDD {
				maxDD = dd
			}
		}
	}
	st.WinRate = float64(wins) / float64(len(trades)) * 100
	st.AvgNet = cumRet / float64(len(trades))
	if grossLoss > 0 {
		st.PF = grossWin / grossLoss
	}
	st.MDD = maxDD * 100
	return st
}

// medianOf 는 슬라이스의 중앙값을 반환한다 (정렬 부수효과).
func medianOf(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	// 삽입 정렬 — 윈도우 수 소량
	cp := make([]float64, len(xs))
	copy(cp, xs)
	for i := 1; i < len(cp); i++ {
		for j := i; j > 0 && cp[j] < cp[j-1]; j-- {
			cp[j], cp[j-1] = cp[j-1], cp[j]
		}
	}
	n := len(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}
