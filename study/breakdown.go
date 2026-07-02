package study

import (
	"RESTGo/box"
	"RESTGo/cond"
	"RESTGo/console"
	"RESTGo/indicator"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// DefBox 하향 돌파 이벤트 스터디 — 운용 룰이 아닌 사후 가격 분포 측정용.
// 2026-06-17 사용자 요청.
//
// 알고리즘:
//   1. KOSPI 전 종목 일봉 BP_PeriodPrice 조회
//   2. 종목 × 봉 순회하며 cond.IsDefBoxBreakdown 발화 검사
//   3. 발화 t → t+1/5/10/20봉 종가 수익률 측정 (왕복 비용 0.4% 차감 — 한국주 수수료+거래세)
//   4. 베이스라인: 무조건 진입 (모든 봉의 같은 호라이즌 수익률 평균)
//   5. 엣지 = 발화 시 평균 − 베이스라인 평균, Welch t-stat
//   6. 결과 JSON

const (
	bsRoundTripCost = 0.004 // 0.4% 왕복 (수수료 0.015%×2 + 거래세 0.18% + 슬리피지)
	bsWarmupBars    = 60    // DefBox 형성에 필요한 워밍업
	bsDaysFetch     = 2500  // 종목당 일봉 봉수 (충분히 길게)
)

var bsHorizons = []int{1, 5, 10, 20}

// BreakdownEventStats 는 호라이즌별 통계
type BreakdownEventStats struct {
	Horizon       int     `json:"horizon"`
	FireCount     int     `json:"fire_count"`
	MeanNetReturn float64 `json:"mean_net_return_pct"`
	MedianNet     float64 `json:"median_net_return_pct"`
	StdNet        float64 `json:"std_net_return_pct"`
	WinRate       float64 `json:"win_rate"`
	BaselineMean  float64 `json:"baseline_mean_net_pct"`
	BaselineN     int     `json:"baseline_n"`
	Edge          float64 `json:"edge_pct"`
	TStat         float64 `json:"t_stat"`
}

// YearStats 는 연도별 호라이즌 통계
type YearStats struct {
	Year      int     `json:"year"`
	Horizon   int     `json:"horizon"`
	FireCount int     `json:"fire_count"`
	Mean      float64 `json:"mean_net_return_pct"`
	Median    float64 `json:"median_net_return_pct"`
	WinRate   float64 `json:"win_rate"`
}

// BreakdownStudyOutput 는 전체 결과
type BreakdownStudyOutput struct {
	GeneratedAt    string                `json:"generated_at"`
	Mode           string                `json:"mode"` // "breakdown" | "breakout" | "recovery"
	StockCount     int                   `json:"stock_count"`
	CandleCount    int64                 `json:"total_candle_count"`
	Horizons       []int                 `json:"horizons"`
	RoundTripCost  float64               `json:"round_trip_cost_pct"`
	RecoveryWindow int                   `json:"recovery_window_bars"` // recovery 모드에서만
	Stats          []BreakdownEventStats `json:"stats"`
	ByYear         []YearStats           `json:"by_year"`
	NetReturnDist  map[string][]float64  `json:"net_return_distribution"`
}

// HandleBreakdownStudy 는 "stock breakdown_study [breakout|breakdown|recovery] [out]" 명령 진입점.
// - "breakdown" (기본): IsDefBoxBreakdown 발화 봉 측정
// - "breakout": IsDefBoxBreakout 발화 봉 측정
// - "recovery": IsDefBoxBreakdown 발화 후 N봉(기본 5) 내 IsDefBoxBreakout 발화한 봉 측정 (재상승 돌파 매수)
func HandleBreakdownStudy(args []string) {
	mode := "breakdown"
	outputPath := "zpicture/breakdown_study.json"
	if len(args) >= 1 && (args[0] == "breakout" || args[0] == "breakdown" || args[0] == "recovery") {
		mode = args[0]
		outputPath = fmt.Sprintf("zpicture/%s_study.json", mode)
		args = args[1:]
	}
	if len(args) >= 1 && args[0] != "" {
		outputPath = args[0]
	}
	runEventStudy(mode, outputPath)
}

// recoveryWindow 는 하향 돌파 후 재상승 돌파 탐색 윈도우 (봉 수)
const recoveryWindow = 5

func runEventStudy(mode string, outputPath string) {
	db, err := console.MsConn.GetDB("KIS2")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[bs] KIS2 DB 연결 실패: %v\n", err)
		return
	}

	rows, err := db.Query(`
		SELECT p.stck_shrn_iscd
		FROM (SELECT DISTINCT stck_shrn_iscd FROM DM.BP_PeriodPrice WHERE period_type='D') p
		ORDER BY p.stck_shrn_iscd
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[bs] 종목 조회 실패: %v\n", err)
		return
	}
	var stocks []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err == nil {
			stocks = append(stocks, s)
		}
	}
	rows.Close()
	modeLabel := mode
	switch mode {
	case "breakdown":
		modeLabel = "Breakdown(하향)"
	case "breakout":
		modeLabel = "Breakout(상향)"
	case "recovery":
		modeLabel = fmt.Sprintf("Recovery(하향→%d봉내 재상향)", recoveryWindow)
	}
	fmt.Printf("[bs] 모드: %s  종목: %d개  호라이즌: %v  비용: %.1f%%\n", modeLabel, len(stocks), bsHorizons, bsRoundTripCost*100)

	// 호라이즌별 수익률 누적
	type horizonAcc struct {
		fireReturns []float64
		baseReturns []float64
	}
	accMap := make(map[int]*horizonAcc)
	yearAcc := make(map[int]map[int][]float64) // [horizon][year] -> returns
	for _, h := range bsHorizons {
		accMap[h] = &horizonAcc{}
		yearAcc[h] = make(map[int][]float64)
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 20)
	var processed int32
	var totalCandles int64

	for _, shcode := range stocks {
		wg.Add(1)
		go func(sh string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			candles, err := box.FetchCandles(db, sh, bsDaysFetch)
			if err != nil || len(candles) < bsWarmupBars+bsHorizons[len(bsHorizons)-1]+2 {
				atomic.AddInt32(&processed, 1)
				return
			}
			indicator.PrepareCandles(candles)

			// 종목별 임시 누적 (마지막에 mu lock으로 합산)
			localFire := make(map[int][]float64)
			localBase := make(map[int][]float64)
			// by_year: [horizon][year] -> returns
			localByYearFire := make(map[int]map[int][]float64)
			for _, h := range bsHorizons {
				localFire[h] = nil
				localBase[h] = nil
				localByYearFire[h] = make(map[int][]float64)
			}
			// recovery 모드: 최근 하향 돌파 봉 인덱스 추적 (-1 = 비활성)
			lastBreakdownIdx := -1

			// DefBox 추적용 임시 컨텍스트
			ctx := box.NewTradingContext(candles, []*box.Box{})

			endIdx := len(candles) - bsHorizons[len(bsHorizons)-1] - 1
			for i := 5; i < endIdx; i++ {
				ctx.Position = i
				// 첫 5봉 곡률 초기화 (analyzeInternal와 동일)
				if i == 5 {
					if candles[i].Gradient >= 0 {
						candles[i].Curvekey = 1
					} else {
						candles[i].Curvekey = -1
					}
					continue
				}
				// Box 구조 생성 (DefBox 추적)
				box.CheckAndCreateDefBox(ctx, 0)
				candles[i].Curvekey = box.AnalyzeCurvature(ctx)

				// DefBox 업데이트 — DefChecker 검사 추가 (analyzeInternal과 동일)
				if ctx.DefChecker != 0 {
					if tempIdx := findLastDefBoxIndexLocal(ctx.BoxList); tempIdx != -1 {
						if ctx.DefboxIndex != tempIdx {
							ctx.DefboxIndex = tempIdx
							ctx.UpdateBoxInfo()
						}
					}
				}

				// 베이스라인 — 모든 유효 봉의 t+H 수익률
				if i+bsHorizons[len(bsHorizons)-1] >= len(candles) {
					continue
				}
				entry := candles[i].CloseOrigin
				if entry <= 0 {
					continue
				}
				for _, h := range bsHorizons {
					if i+h >= len(candles) {
						continue
					}
					exit := candles[i+h].CloseOrigin
					if exit <= 0 {
						continue
					}
					ret := (exit-entry)/entry*100.0 - bsRoundTripCost*100.0
					localBase[h] = append(localBase[h], ret)
				}

				// 발화 검사 (모드별)
				fired := false
				switch mode {
				case "breakout":
					fired = cond.IsDefBoxBreakout(ctx)
				case "recovery":
					// 1) 하향 돌파 발생 → 마커 갱신
					if cond.IsDefBoxBreakdown(ctx) {
						lastBreakdownIdx = i
					}
					// 2) 상향 돌파 발생 + 최근 recoveryWindow 봉 내 하향 돌파 마커 존재
					if cond.IsDefBoxBreakout(ctx) && lastBreakdownIdx >= 0 && i-lastBreakdownIdx <= recoveryWindow {
						fired = true
						lastBreakdownIdx = -1 // 한 번 사용 후 비활성 (중복 진입 방지)
					}
				default: // breakdown
					fired = cond.IsDefBoxBreakdown(ctx)
				}
				if !fired {
					continue
				}
				// 발화 — 같은 호라이즌 수익률 기록
				year := 0
				if len(candles[i].Date) >= 4 {
					fmt.Sscanf(candles[i].Date[:4], "%d", &year)
				}
				for _, h := range bsHorizons {
					if i+h >= len(candles) {
						continue
					}
					exit := candles[i+h].CloseOrigin
					if exit <= 0 {
						continue
					}
					ret := (exit-entry)/entry*100.0 - bsRoundTripCost*100.0
					localFire[h] = append(localFire[h], ret)
					if year > 0 {
						localByYearFire[h][year] = append(localByYearFire[h][year], ret)
					}
				}
			}

			mu.Lock()
			for _, h := range bsHorizons {
				accMap[h].fireReturns = append(accMap[h].fireReturns, localFire[h]...)
				accMap[h].baseReturns = append(accMap[h].baseReturns, localBase[h]...)
				for y, rs := range localByYearFire[h] {
					yearAcc[h][y] = append(yearAcc[h][y], rs...)
				}
			}
			atomic.AddInt64(&totalCandles, int64(len(candles)))
			mu.Unlock()

			n := atomic.AddInt32(&processed, 1)
			if n%100 == 0 {
				fmt.Printf("[bs] %d/%d 처리...\n", n, len(stocks))
			}
		}(shcode)
	}
	wg.Wait()

	// 통계 계산
	out := BreakdownStudyOutput{
		GeneratedAt:   time.Now().Format(time.RFC3339),
		Mode:          mode,
		StockCount:    len(stocks),
		CandleCount:   totalCandles,
		Horizons:      bsHorizons,
		RoundTripCost: bsRoundTripCost * 100,
		NetReturnDist: make(map[string][]float64),
	}
	if mode == "recovery" {
		out.RecoveryWindow = recoveryWindow
	}
	for _, h := range bsHorizons {
		fireR := accMap[h].fireReturns
		baseR := accMap[h].baseReturns
		stat := BreakdownEventStats{Horizon: h, FireCount: len(fireR), BaselineN: len(baseR)}
		if len(fireR) > 0 {
			stat.MeanNetReturn = meanFloats(fireR)
			stat.MedianNet = medianFloats(fireR)
			stat.StdNet = stdFloats(fireR)
			wins := 0
			for _, r := range fireR {
				if r > 0 {
					wins++
				}
			}
			stat.WinRate = float64(wins) / float64(len(fireR)) * 100
		}
		if len(baseR) > 0 {
			stat.BaselineMean = meanFloats(baseR)
		}
		stat.Edge = stat.MeanNetReturn - stat.BaselineMean
		stat.TStat = welchTStatFloats(fireR, baseR)
		out.Stats = append(out.Stats, stat)
		// 분포 raw (히스토그램용, 최대 50,000 샘플)
		key := fmt.Sprintf("h%d", h)
		if len(fireR) > 50000 {
			out.NetReturnDist[key] = fireR[:50000]
		} else {
			out.NetReturnDist[key] = fireR
		}
		fmt.Printf("[bs] h=%d  fire=%d  mean=%+.3f%%  median=%+.3f%%  win=%.1f%%  edge=%+.3f%%p  t=%.2f\n",
			h, stat.FireCount, stat.MeanNetReturn, stat.MedianNet, stat.WinRate, stat.Edge, stat.TStat)
		// 연도별 집계 (5건 미만 연도는 스킵)
		years := make([]int, 0, len(yearAcc[h]))
		for y := range yearAcc[h] {
			years = append(years, y)
		}
		// 정렬 (삽입 정렬, 연도 수 소량)
		for ii := 1; ii < len(years); ii++ {
			for jj := ii; jj > 0 && years[jj] < years[jj-1]; jj-- {
				years[jj], years[jj-1] = years[jj-1], years[jj]
			}
		}
		for _, y := range years {
			rs := yearAcc[h][y]
			if len(rs) < 5 {
				continue
			}
			wins := 0
			for _, r := range rs {
				if r > 0 {
					wins++
				}
			}
			out.ByYear = append(out.ByYear, YearStats{
				Year:      y,
				Horizon:   h,
				FireCount: len(rs),
				Mean:      meanFloats(rs),
				Median:    medianFloats(rs),
				WinRate:   float64(wins) / float64(len(rs)) * 100,
			})
		}
	}

	if err := os.MkdirAll(dirOf(outputPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[bs] 디렉토리 생성 실패: %v\n", err)
		return
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[bs] JSON 직렬화 실패: %v\n", err)
		return
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[bs] 파일 저장 실패: %v\n", err)
		return
	}
	fmt.Printf("[bs] 완료: %s (총 %d 종목 / %d 캔들)\n", outputPath, len(stocks), totalCandles)
}

func findLastDefBoxIndexLocal(boxList []*box.Box) int {
	for i := len(boxList) - 1; i >= 0; i-- {
		if boxList[i].KindOfBox == box.KindDefBox {
			return i
		}
	}
	return -1
}
