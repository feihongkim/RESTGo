package study

// volume_wave_matrix.go — VW1/VW2 돌파 추격 대조군과 "첫 눌림→반등→다음 시가" 재진입의
// 소거/파라미터 매트릭스. 비용 차감, 고정 IS/OOS 분리, chase 대비 개선폭을 함께 기록한다.

import (
	"RESTGo/box"
	"RESTGo/cond"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
)

var volumeWaveMatrixHorizons = []int{1, 5, 10, 20}

type volumeWaveVariant struct {
	ID     string                       `json:"id"`
	Group  string                       `json:"group"` // chase / ablation / matrix
	Label  string                       `json:"label"`
	Chase  bool                         `json:"chase"`
	Config stg.VolumeWavePullbackConfig `json:"config"`
}

type volumeWaveMatrixRecord struct {
	variant int
	period  string
	returns map[int]float64
}

type volumeWaveVariantAggregate struct {
	returns map[string][]float64 // period|horizon
}

type VolumeWaveMatrixResult struct {
	VariantID    string                       `json:"variant_id"`
	Group        string                       `json:"group"`
	Label        string                       `json:"label"`
	Config       stg.VolumeWavePullbackConfig `json:"config"`
	Period       string                       `json:"period"`
	Horizon      int                          `json:"horizon"`
	SourceN      int                          `json:"source_n"`
	EntryN       int                          `json:"entry_n"`
	EntryRate    float64                      `json:"entry_rate_pct"`
	MeanNet      float64                      `json:"mean_net_pct"`
	MedianNet    float64                      `json:"median_net_pct"`
	WinRate      float64                      `json:"win_rate_pct"`
	ProfitFactor float64                      `json:"profit_factor"`
	ChaseMeanNet float64                      `json:"chase_mean_net_pct"`
	Improvement  float64                      `json:"improvement_vs_chase_pctp"`
	TStatVsChase float64                      `json:"t_stat_vs_chase"`
}

type VolumeWaveMatrixTop struct {
	VariantID    string                       `json:"variant_id"`
	Label        string                       `json:"label"`
	Config       stg.VolumeWavePullbackConfig `json:"config"`
	ISN          int                          `json:"is_n"`
	ISMeanNet    float64                      `json:"is_mean_net_pct"`
	OOSN         int                          `json:"oos_n"`
	OOSMeanNet   float64                      `json:"oos_mean_net_pct"`
	OOSMedianNet float64                      `json:"oos_median_net_pct"`
	OOSPF        float64                      `json:"oos_profit_factor"`
	OOSImprove   float64                      `json:"oos_improvement_vs_chase_pctp"`
	RobustScore  float64                      `json:"robust_score"` // min(IS/OOS mean, IS/OOS chase 개선폭)
}

// HandleVolumeWaveMatrix 사용법:
// ./RESTGo stock volume_wave_matrix [--max N] [--candles N] [--out path]
//
//	[--oos-date YYYYMMDD] [--cost-bp-per-side N]
func HandleVolumeWaveMatrix(args []string) {
	maxStocks := 0
	candleCount := 4200
	outPath := "zpicture/volume_wave_pullback_matrix.json"
	oosDate := "20220101"
	costBPPerSide := 15.0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--max":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &maxStocks)
				i++
			}
		case "--candles":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &candleCount)
				i++
			}
		case "--out":
			if i+1 < len(args) {
				outPath = args[i+1]
				i++
			}
		case "--oos-date":
			if i+1 < len(args) {
				oosDate = args[i+1]
				i++
			}
		case "--cost-bp-per-side":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%f", &costBPPerSide)
				i++
			}
		}
	}
	if len(oosDate) != 8 || costBPPerSide < 0 {
		fmt.Fprintln(os.Stderr, "[volume_wave_matrix] 잘못된 oos-date 또는 비용")
		return
	}

	db, err := console.MsConn.GetDB("han")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[volume_wave_matrix] han DB 연결 오류: %v\n", err)
		return
	}
	stocks, err := box.FetchHannamStockList(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[volume_wave_matrix] 종목 목록 오류: %v\n", err)
		return
	}
	if maxStocks > 0 && len(stocks) > maxStocks {
		stocks = stocks[:maxStocks]
	}
	variants := buildVolumeWaveVariants()
	waveCfg := cond.DefaultVolumeWaveConfig()
	roundTripCostPct := costBPPerSide * 2 / 100 // 1bp=0.01%
	fmt.Printf("[volume_wave_matrix] KR %d종목 × %d변형, OOS=%s, 비용=왕복 %.3f%%\n",
		len(stocks), len(variants), oosDate, roundTripCostPct)

	aggs := make([]volumeWaveVariantAggregate, len(variants))
	for i := range aggs {
		aggs[i] = volumeWaveVariantAggregate{returns: map[string][]float64{}}
	}
	sourceN := map[string]int{} // stage|period
	var mu sync.Mutex
	var scanned, failed atomic.Int64
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup

	for _, code := range stocks {
		wg.Add(1)
		sem <- struct{}{}
		go func(shcode string) {
			defer wg.Done()
			defer func() { <-sem }()
			candles, fetchErr := box.FetchCandlesHannam(db, shcode, candleCount)
			if fetchErr != nil || len(candles) < waveCfg.AccumulationMaxLead+waveCfg.VolumeWindow+20 {
				failed.Add(1)
				return
			}
			indicator.PrepareCandles(candles)
			waves := stg.VolumeWaveAnalyze(candles, waveCfg)

			localSource := map[string]int{}
			for _, source := range waves {
				if source.Stage != 1 && source.Stage != 2 {
					continue
				}
				period := volumeWavePeriod(source.Date, oosDate)
				localSource[fmt.Sprintf("%d|ALL", source.Stage)]++
				localSource[fmt.Sprintf("%d|%s", source.Stage, period)]++
			}

			var records []volumeWaveMatrixRecord
			for variantIdx, variant := range variants {
				if variant.Chase {
					for _, source := range waves {
						if source.Stage != variant.Config.SourceStage || source.Pos+1 >= len(candles) {
							continue
						}
						entryPos := source.Pos + 1
						entryPrice := candles[entryPos].OpenOrigin
						if entryPrice <= 0 {
							continue
						}
						if rs := volumeWaveEntryReturns(candles, entryPos, entryPrice, roundTripCostPct); len(rs) > 0 {
							records = append(records, volumeWaveMatrixRecord{variant: variantIdx, period: volumeWavePeriod(candles[entryPos].Date, oosDate), returns: rs})
						}
					}
					continue
				}
				entries := stg.VolumeWavePullbackAnalyze(candles, waves, variant.Config)
				for _, entry := range entries {
					if rs := volumeWaveEntryReturns(candles, entry.EntryPos, entry.EntryPriceOrigin, roundTripCostPct); len(rs) > 0 {
						records = append(records, volumeWaveMatrixRecord{variant: variantIdx, period: volumeWavePeriod(entry.EntryDate, oosDate), returns: rs})
					}
				}
			}

			mu.Lock()
			for key, n := range localSource {
				sourceN[key] += n
			}
			for _, record := range records {
				agg := &aggs[record.variant]
				for h, r := range record.returns {
					agg.returns[volumeWaveReturnKey("ALL", h)] = append(agg.returns[volumeWaveReturnKey("ALL", h)], r)
					agg.returns[volumeWaveReturnKey(record.period, h)] = append(agg.returns[volumeWaveReturnKey(record.period, h)], r)
				}
			}
			mu.Unlock()
			if n := scanned.Add(1); n%500 == 0 {
				fmt.Printf("[volume_wave_matrix] 진행 %d/%d\n", n, len(stocks))
			}
		}(code)
	}
	wg.Wait()

	chaseIndex := map[int]int{}
	for i, variant := range variants {
		if variant.Chase {
			chaseIndex[variant.Config.SourceStage] = i
		}
	}
	var results []VolumeWaveMatrixResult
	resultLookup := map[string]VolumeWaveMatrixResult{}
	for variantIdx, variant := range variants {
		for _, period := range []string{"ALL", "IS", "OOS"} {
			for _, h := range volumeWaveMatrixHorizons {
				values := aggs[variantIdx].returns[volumeWaveReturnKey(period, h)]
				if len(values) == 0 {
					continue
				}
				chaseValues := aggs[chaseIndex[variant.Config.SourceStage]].returns[volumeWaveReturnKey(period, h)]
				mean := meanFloats(values)
				chaseMean := meanFloats(chaseValues)
				wins := 0
				profit, loss := 0.0, 0.0
				for _, r := range values {
					if r > 0 {
						wins++
						profit += r
					} else if r < 0 {
						loss -= r
					}
				}
				pf := 0.0
				if loss > 0 {
					pf = profit / loss
				}
				sn := sourceN[fmt.Sprintf("%d|%s", variant.Config.SourceStage, period)]
				result := VolumeWaveMatrixResult{
					VariantID: variant.ID, Group: variant.Group, Label: variant.Label, Config: variant.Config,
					Period: period, Horizon: h, SourceN: sn, EntryN: len(values),
					MeanNet: mean, MedianNet: medianFloats(values),
					WinRate: 100 * float64(wins) / float64(len(values)), ProfitFactor: pf,
					ChaseMeanNet: chaseMean, Improvement: mean - chaseMean,
					TStatVsChase: welchTStatFloats(values, chaseValues),
				}
				if sn > 0 {
					result.EntryRate = 100 * float64(len(values)) / float64(sn)
				}
				results = append(results, result)
				resultLookup[volumeWaveResultKey(variant.ID, period, h)] = result
			}
		}
	}

	buildTop := func(horizon int) ([]VolumeWaveMatrixTop, int) {
		var ranked []VolumeWaveMatrixTop
		for _, variant := range variants {
			if variant.Group != "matrix" {
				continue
			}
			is, isOK := resultLookup[volumeWaveResultKey(variant.ID, "IS", horizon)]
			oos, oosOK := resultLookup[volumeWaveResultKey(variant.ID, "OOS", horizon)]
			if !isOK || !oosOK || is.EntryN < 100 || oos.EntryN < 100 ||
				is.MeanNet <= 0 || oos.MeanNet <= 0 || is.Improvement <= 0 || oos.Improvement <= 0 {
				continue
			}
			ranked = append(ranked, VolumeWaveMatrixTop{
				VariantID: variant.ID, Label: variant.Label, Config: variant.Config,
				ISN: is.EntryN, ISMeanNet: is.MeanNet,
				OOSN: oos.EntryN, OOSMeanNet: oos.MeanNet, OOSMedianNet: oos.MedianNet,
				OOSPF: oos.ProfitFactor, OOSImprove: oos.Improvement,
				RobustScore: math.Min(math.Min(is.MeanNet, oos.MeanNet), math.Min(is.Improvement, oos.Improvement)),
			})
		}
		sort.Slice(ranked, func(i, j int) bool {
			if ranked[i].RobustScore != ranked[j].RobustScore {
				return ranked[i].RobustScore > ranked[j].RobustScore
			}
			return ranked[i].OOSMeanNet > ranked[j].OOSMeanNet
		})
		total := len(ranked)
		if len(ranked) > 20 {
			ranked = ranked[:20]
		}
		return ranked, total
	}
	topByHorizon := map[int][]VolumeWaveMatrixTop{}
	robustCounts := map[int]int{}
	for _, h := range volumeWaveMatrixHorizons {
		topByHorizon[h], robustCounts[h] = buildTop(h)
	}

	out := struct {
		Title            string                        `json:"title"`
		GeneratedRule    string                        `json:"generated_rule"`
		StockCount       int                           `json:"stock_count"`
		ScannedCount     int64                         `json:"scanned_count"`
		FailedCount      int64                         `json:"failed_count"`
		CandleCount      int                           `json:"candle_count"`
		OOSDate          string                        `json:"oos_date"`
		CostBPPerSide    float64                       `json:"cost_bp_per_side"`
		PrimaryHorizon   int                           `json:"primary_horizon"`
		VariantCount     int                           `json:"variant_count"`
		MultipleTestNote string                        `json:"multiple_test_note"`
		Variants         []volumeWaveVariant           `json:"variants"`
		Results          []VolumeWaveMatrixResult      `json:"results"`
		RobustCounts     map[int]int                   `json:"robust_positive_counts_by_horizon"`
		TopByHorizon     map[int][]VolumeWaveMatrixTop `json:"top_robust_by_horizon"`
	}{
		Title:         "VW1/VW2 첫 눌림 진입 소거·매트릭스",
		GeneratedRule: "breakout day chase 금지; pullback close 확인 후 next-open fill",
		StockCount:    len(stocks), ScannedCount: scanned.Load(), FailedCount: failed.Load(),
		CandleCount: candleCount, OOSDate: oosDate, CostBPPerSide: costBPPerSide,
		PrimaryHorizon: 5, VariantCount: len(variants),
		MultipleTestNote: "robust=IS/OOS 비용후 평균과 chase 개선폭 모두 양수·각 n≥100. 순위는 가설 생성용이며 채택 전 walk-forward/DSR 별도 필요",
		Variants:         variants, Results: results,
		RobustCounts: robustCounts, TopByHorizon: topByHorizon,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[volume_wave_matrix] JSON 실패: %v\n", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[volume_wave_matrix] 출력 디렉토리 실패: %v\n", err)
		return
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "[volume_wave_matrix] 저장 실패: %v\n", err)
		return
	}

	fmt.Printf("[volume_wave_matrix] 완료: 스캔 %d 실패 %d 변형 %d → %s\n", scanned.Load(), failed.Load(), len(variants), outPath)
	fmt.Println("[volume_wave_matrix] 소거군 OOS h5 (비용후):")
	for _, variant := range variants {
		if variant.Group != "chase" && variant.Group != "ablation" {
			continue
		}
		if r, ok := resultLookup[volumeWaveResultKey(variant.ID, "OOS", 5)]; ok {
			fmt.Printf("  %-22s n=%-6d mean=%+.3f%% med=%+.3f%% PF=%.2f chase대비=%+.3f%%p\n",
				variant.ID, r.EntryN, r.MeanNet, r.MedianNet, r.ProfitFactor, r.Improvement)
		}
	}
	for _, h := range []int{5, 20} {
		fmt.Printf("[volume_wave_matrix] IS/OOS 비용후·chase개선 모두 양수·각 n≥100 (h%d): %d개\n", h, robustCounts[h])
		for i, r := range topByHorizon[h] {
			if i >= 5 {
				break
			}
			fmt.Printf("  #%d %s  IS n=%d %+.3f%% / OOS n=%d %+.3f%% med=%+.3f%% PF=%.2f improve=%+.3f%%p\n",
				i+1, r.VariantID, r.ISN, r.ISMeanNet, r.OOSN, r.OOSMeanNet, r.OOSMedianNet, r.OOSPF, r.OOSImprove)
		}
	}
}

func buildVolumeWaveVariants() []volumeWaveVariant {
	var out []volumeWaveVariant
	for _, stage := range []int{1, 2} {
		out = append(out, volumeWaveVariant{
			ID: fmt.Sprintf("CHASE_S%d", stage), Group: "chase", Label: "돌파 다음 봉 시가 추격 대조군", Chase: true,
			Config: stg.VolumeWavePullbackConfig{SourceStage: stage},
		})
		base := stg.VolumeWavePullbackConfig{
			SourceStage: stage, MinWaitBars: 1, MaxWaitBars: 10, MinDepth: 0, MaxDepth: 1,
			Confirmation: stg.VolumeWaveConfirmCloseUp, Structure: stg.VolumeWaveStructureNone,
		}
		out = append(out, volumeWaveVariant{ID: fmt.Sprintf("A1_DELAY_S%d", stage), Group: "ablation", Label: "첫 눌림+약반등만", Config: base})
		depth := base
		depth.MinDepth, depth.MaxDepth = 0.02, 0.15
		out = append(out, volumeWaveVariant{ID: fmt.Sprintf("A2_DEPTH_S%d", stage), Group: "ablation", Label: "+눌림 깊이 2~15%", Config: depth})
		volume := depth
		volume.MaxEntryVolumeRatio, volume.MaxPullbackAverageVolumeRatio = 0.8, 0.8
		out = append(out, volumeWaveVariant{ID: fmt.Sprintf("A3_VOLUME_S%d", stage), Group: "ablation", Label: "+거래량 수축", Config: volume})
		structure := volume
		structure.Structure, structure.StructureTolerance = stg.VolumeWaveStructureBreakoutFloor, 0.02
		out = append(out, volumeWaveVariant{ID: fmt.Sprintf("A4_STRUCTURE_S%d", stage), Group: "ablation", Label: "+돌파 전 종가 지지", Config: structure})
		confirm := structure
		confirm.Confirmation = stg.VolumeWaveConfirmPrevHigh
		out = append(out, volumeWaveVariant{ID: fmt.Sprintf("A5_CONFIRM_S%d", stage), Group: "ablation", Label: "+전일 고가 돌파 확인", Config: confirm})
	}

	depths := []struct {
		name     string
		min, max float64
	}{
		{"D01_08", 0.01, 0.08}, {"D02_15", 0.02, 0.15}, {"D01_20", 0.01, 0.20},
	}
	structures := []struct{ code, value string }{
		{"N", stg.VolumeWaveStructureNone}, {"F", stg.VolumeWaveStructureBreakoutFloor}, {"M", stg.VolumeWaveStructureMA20},
	}
	confirms := []struct{ code, value string }{
		{"CU", stg.VolumeWaveConfirmCloseUp}, {"PH", stg.VolumeWaveConfirmPrevHigh},
	}
	for _, stage := range []int{1, 2} {
		for _, wait := range []int{5, 10, 15} {
			for _, depth := range depths {
				for _, volume := range []float64{0.5, 0.7, 0.9} {
					for _, confirm := range confirms {
						for _, structure := range structures {
							id := fmt.Sprintf("M_S%d_W%02d_%s_V%02d_%s_%s", stage, wait, depth.name, int(volume*10), confirm.code, structure.code)
							out = append(out, volumeWaveVariant{
								ID: id, Group: "matrix", Label: "첫 눌림 매트릭스",
								Config: stg.VolumeWavePullbackConfig{
									SourceStage: stage, MinWaitBars: 1, MaxWaitBars: wait,
									MinDepth: depth.min, MaxDepth: depth.max,
									MaxEntryVolumeRatio: volume, MaxPullbackAverageVolumeRatio: volume,
									Confirmation: confirm.value, Structure: structure.value, StructureTolerance: 0.02,
								},
							})
						}
					}
				}
			}
		}
	}
	return out
}

func volumeWaveEntryReturns(candles []*box.Candle, entryPos int, entryPrice, costPct float64) map[int]float64 {
	out := map[int]float64{}
	if entryPos < 0 || entryPos >= len(candles) || entryPrice <= 0 {
		return out
	}
	for _, h := range volumeWaveMatrixHorizons {
		exitPos := entryPos + h
		if exitPos < len(candles) && candles[exitPos].CloseOrigin > 0 {
			out[h] = (candles[exitPos].CloseOrigin-entryPrice)/entryPrice*100 - costPct
		}
	}
	return out
}

func volumeWavePeriod(date, oosDate string) string {
	if date >= oosDate {
		return "OOS"
	}
	return "IS"
}

func volumeWaveReturnKey(period string, horizon int) string {
	return fmt.Sprintf("%s|%d", period, horizon)
}
func volumeWaveResultKey(variant, period string, horizon int) string {
	return fmt.Sprintf("%s|%s|%d", variant, period, horizon)
}
