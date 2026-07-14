package study

// volume_wave_strict_study.go — 단발 거래량 대신 N봉 누적거래량+OBV 매집을 쓰고,
// VW1 종가 유지율·10~12% 고저폭·전후반 거래량 감소로 고가놀이를 좁힌 소거/매트릭스.

import (
	"RESTGo/box"
	"RESTGo/cond"
	"RESTGo/console"
	"RESTGo/indicator"
	"RESTGo/stg"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type volumeWaveStrictVariant struct {
	ID, Group, Label string
	Config           cond.VolumeWaveConfig
}
type volumeWaveStrictAgg struct {
	source  map[string]int
	returns map[string][]float64
}
type VolumeWaveStrictResult struct {
	VariantID       string                `json:"variant_id"`
	Group           string                `json:"group"`
	Label           string                `json:"label"`
	Config          cond.VolumeWaveConfig `json:"config"`
	Period          string                `json:"period"`
	Horizon         int                   `json:"horizon"`
	SourceN         int                   `json:"vw2_source_n"`
	SourceRetention float64               `json:"source_retention_pct"`
	EntryN          int                   `json:"entry_n"`
	EntryRate       float64               `json:"entry_rate_pct"`
	MeanNet         float64               `json:"mean_net_pct"`
	TrimmedMeanNet  float64               `json:"trimmed_mean_5pct"`
	MedianNet       float64               `json:"median_net_pct"`
	P90Net          float64               `json:"p90_net_pct"`
	MaxNet          float64               `json:"max_net_pct"`
	WinRate         float64               `json:"win_rate_pct"`
	PF              float64               `json:"profit_factor"`
}
type VolumeWaveStrictTop struct {
	VariantID string                `json:"variant_id"`
	Config    cond.VolumeWaveConfig `json:"config"`
	ISN       int                   `json:"is_n"`
	ISMean    float64               `json:"is_mean_net_pct"`
	OOSN      int                   `json:"oos_n"`
	OOSMean   float64               `json:"oos_mean_net_pct"`
	OOSMedian float64               `json:"oos_median_net_pct"`
	OOSPF     float64               `json:"oos_profit_factor"`
	Score     float64               `json:"score"`
}

func buildVolumeWaveStrictVariants() []volumeWaveStrictVariant {
	base := cond.DefaultVolumeWaveConfig()
	mk := func(id, group, label string, c cond.VolumeWaveConfig) volumeWaveStrictVariant {
		return volumeWaveStrictVariant{id, group, label, c}
	}
	v := []volumeWaveStrictVariant{mk("BASE", "baseline", "기존 단발 spike + 느슨한 고가놀이", base)}
	acc := base
	acc.AccumulationMode = cond.VolumeWaveAccumulationCumulativeOBV
	acc.AccumulationWindow = 10
	acc.AccumulationVolumeRatio = 1.2
	v = append(v, mk("A_ACC", "ablation", "10봉 누적거래량 1.2x + OBV 상승", acc))
	ret := base
	ret.BaseMinCloseRetention = .8
	v = append(v, mk("A_RET", "ablation", "VW1 종가 위 80% 유지", ret))
	width := base
	width.BaseMaxHighLowRange = .12
	v = append(v, mk("A_WIDTH", "ablation", "고저폭 12% 이하", width))
	decay := base
	decay.BaseLateEarlyVolRatio = .9
	v = append(v, mk("A_DECAY", "ablation", "후반 평균거래량≤전반 90%", decay))
	strict := acc
	strict.BaseMinCloseRetention = .8
	strict.BaseMaxHighLowRange = .12
	strict.BaseLateEarlyVolRatio = .9
	v = append(v, mk("STRICT", "combined", "이미지 정합 기본 결합", strict))
	for _, n := range []int{5, 10, 20} {
		for _, vr := range []float64{1.15, 1.30} {
			for _, rr := range []float64{.7, .85} {
				for _, w := range []float64{.10, .12} {
					for _, d := range []float64{.8, 1.0} {
						c := base
						c.AccumulationMode = cond.VolumeWaveAccumulationCumulativeOBV
						c.AccumulationWindow = n
						c.AccumulationVolumeRatio = vr
						c.BaseMinCloseRetention = rr
						c.BaseMaxHighLowRange = w
						c.BaseLateEarlyVolRatio = d
						id := fmt.Sprintf("M_N%02d_V%03.0f_R%02.0f_W%02.0f_D%03.0f", n, vr*100, rr*100, w*100, d*100)
						v = append(v, mk(id, "matrix", "누적OBV+좁은 고가놀이", c))
					}
				}
			}
		}
	}
	return v
}

// HandleVolumeWaveStrictStudy 사용법: ./RESTGo stock volume_wave_strict_study [--max N] [--candles N] [--out path]
func HandleVolumeWaveStrictStudy(args []string) {
	maxStocks, candleCount := 0, 4200
	outPath, oosDate := "zpicture/volume_wave_strict_study.json", "20220101"
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
		}
	}
	db, err := console.MsConn.GetDB("han")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	stocks, err := box.FetchHannamStockList(db)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if maxStocks > 0 && len(stocks) > maxStocks {
		stocks = stocks[:maxStocks]
	}
	variants := buildVolumeWaveStrictVariants()
	aggs := make([]volumeWaveStrictAgg, len(variants))
	for i := range aggs {
		aggs[i] = volumeWaveStrictAgg{map[string]int{}, map[string][]float64{}}
	}
	entryCfg := selectedVolumeWavePullbackConfig()
	var mu sync.Mutex
	var scanned, failed atomic.Int64
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	fmt.Printf("[volume_wave_strict_study] KR %d종목 × %d변형\n", len(stocks), len(variants))
	for _, code := range stocks {
		wg.Add(1)
		sem <- struct{}{}
		go func(shcode string) {
			defer wg.Done()
			defer func() { <-sem }()
			candles, e := box.FetchCandlesHannam(db, shcode, candleCount)
			if e != nil || len(candles) < 120 {
				failed.Add(1)
				return
			}
			indicator.PrepareCandles(candles)
			local := make([]volumeWaveStrictAgg, len(variants))
			for vi, v := range variants {
				local[vi] = volumeWaveStrictAgg{map[string]int{}, map[string][]float64{}}
				waves := stg.VolumeWaveAnalyze(candles, v.Config)
				for _, s := range waves {
					if s.Stage == 2 {
						p := volumeWavePeriod(s.Date, oosDate)
						local[vi].source["ALL"]++
						local[vi].source[p]++
					}
				}
				for _, e := range stg.VolumeWavePullbackAnalyze(candles, waves, entryCfg) {
					// hannam 원시 일봉의 액면분할/병합 비조정 점프를 신호 및 수익에서 제거한다.
					if !volumeWavePricePathClean(candles, e.Cycle.AccumulationPos, e.EntryPos) {
						continue
					}
					p := volumeWavePeriod(e.EntryDate, oosDate)
					for h, r := range volumeWaveEntryReturnsClean(candles, e.EntryPos, e.EntryPriceOrigin, .30) {
						local[vi].returns[volumeWaveReturnKey("ALL", h)] = append(local[vi].returns[volumeWaveReturnKey("ALL", h)], r)
						local[vi].returns[volumeWaveReturnKey(p, h)] = append(local[vi].returns[volumeWaveReturnKey(p, h)], r)
					}
				}
			}
			mu.Lock()
			for i := range variants {
				for k, n := range local[i].source {
					aggs[i].source[k] += n
				}
				for k, x := range local[i].returns {
					aggs[i].returns[k] = append(aggs[i].returns[k], x...)
				}
			}
			mu.Unlock()
			if n := scanned.Add(1); n%500 == 0 {
				fmt.Printf("[volume_wave_strict_study] %d/%d\n", n, len(stocks))
			}
		}(code)
	}
	wg.Wait()
	var results []VolumeWaveStrictResult
	lookup := map[string]VolumeWaveStrictResult{}
	for vi, v := range variants {
		for _, p := range []string{"ALL", "IS", "OOS"} {
			for _, h := range volumeWaveMatrixHorizons {
				x := aggs[vi].returns[volumeWaveReturnKey(p, h)]
				r := VolumeWaveStrictResult{
					VariantID: v.ID, Group: v.Group, Label: v.Label, Config: v.Config,
					Period: p, Horizon: h, SourceN: aggs[vi].source[p], EntryN: len(x),
					MeanNet: meanFloats(x), TrimmedMeanNet: trimmedMeanFloats(x, 0.05),
					MedianNet: medianFloats(x), P90Net: percentileFloats(x, 0.90), MaxNet: percentileFloats(x, 1.0),
				}
				if aggs[0].source[p] > 0 {
					r.SourceRetention = 100 * float64(r.SourceN) / float64(aggs[0].source[p])
				}
				if r.SourceN > 0 {
					r.EntryRate = 100 * float64(r.EntryN) / float64(r.SourceN)
				}
				wins := 0
				profit, loss := 0.0, 0.0
				for _, z := range x {
					if z > 0 {
						wins++
						profit += z
					} else if z < 0 {
						loss -= z
					}
				}
				if len(x) > 0 {
					r.WinRate = 100 * float64(wins) / float64(len(x))
				}
				if loss > 0 {
					r.PF = profit / loss
				}
				results = append(results, r)
				lookup[fmt.Sprintf("%s|%s|%d", v.ID, p, h)] = r
			}
		}
	}
	var top5, top20 []VolumeWaveStrictTop
	for _, v := range variants {
		if v.Group != "matrix" {
			continue
		}
		for _, pair := range []struct {
			h   int
			dst *[]VolumeWaveStrictTop
		}{{5, &top5}, {20, &top20}} {
			is := lookup[fmt.Sprintf("%s|IS|%d", v.ID, pair.h)]
			oo := lookup[fmt.Sprintf("%s|OOS|%d", v.ID, pair.h)]
			if is.EntryN >= 100 && oo.EntryN >= 100 && is.MeanNet > 0 && oo.MeanNet > 0 {
				*pair.dst = append(*pair.dst, VolumeWaveStrictTop{v.ID, v.Config, is.EntryN, is.MeanNet, oo.EntryN, oo.MeanNet, oo.MedianNet, oo.PF, minFloat(is.MeanNet, oo.MeanNet)})
			}
		}
	}
	sortTop := func(x []VolumeWaveStrictTop) { sort.Slice(x, func(i, j int) bool { return x[i].Score > x[j].Score }) }
	sortTop(top5)
	sortTop(top20)
	if len(top5) > 20 {
		top5 = top5[:20]
	}
	if len(top20) > 20 {
		top20 = top20[:20]
	}
	out := struct {
		GeneratedAt, OOSDate string
		CostPct              float64
		Scanned, Failed      int64
		Variants             []volumeWaveStrictVariant
		Results              []VolumeWaveStrictResult
		TopH5, TopH20        []VolumeWaveStrictTop
	}{time.Now().Format(time.RFC3339), oosDate, .30, scanned.Load(), failed.Load(), variants, results, top5, top20}
	data, _ := json.MarshalIndent(out, "", "  ")
	os.MkdirAll(filepath.Dir(outPath), 0755)
	if err = os.WriteFile(outPath, data, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("[volume_wave_strict_study] 완료 %d/%d → %s robust h5=%d h20=%d\n", scanned.Load(), failed.Load(), outPath, len(top5), len(top20))
	for _, r := range results {
		if r.Period == "OOS" && r.Horizon == 20 && (r.Group != "matrix") {
			fmt.Printf("  %-8s src=%d(%.1f%%) n=%d mean=%+.3f%% med=%+.3f%% PF=%.2f\n", r.VariantID, r.SourceN, r.SourceRetention, r.EntryN, r.MeanNet, r.MedianNet, r.PF)
		}
	}
}

func volumeWavePricePathClean(candles []*box.Candle, start, end int) bool {
	if start < 1 {
		start = 1
	}
	if end >= len(candles) {
		end = len(candles) - 1
	}
	for i := start; i <= end; i++ {
		prev, cur := candles[i-1].CloseOrigin, candles[i].CloseOrigin
		if prev <= 0 || cur <= 0 {
			return false
		}
		ratio := cur / prev
		if ratio < 0.5 || ratio > 1.5 {
			return false
		} // KRX ±30% 제한 밖은 기업행위/데이터 단절
	}
	return true
}

func volumeWaveEntryReturnsClean(candles []*box.Candle, entryPos int, entryPrice, costPct float64) map[int]float64 {
	out := map[int]float64{}
	for _, h := range volumeWaveMatrixHorizons {
		end := entryPos + h
		if end >= len(candles) || !volumeWavePricePathClean(candles, entryPos+1, end) {
			continue
		}
		if candles[end].CloseOrigin > 0 {
			out[h] = (candles[end].CloseOrigin-entryPrice)/entryPrice*100 - costPct
		}
	}
	return out
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func percentileFloats(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	x := append([]float64(nil), values...)
	sort.Float64s(x)
	idx := int(float64(len(x)-1)*q + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(x) {
		idx = len(x) - 1
	}
	return x[idx]
}

func trimmedMeanFloats(values []float64, fraction float64) float64 {
	if len(values) == 0 {
		return 0
	}
	x := append([]float64(nil), values...)
	sort.Float64s(x)
	cut := int(float64(len(x)) * fraction)
	if cut*2 >= len(x) {
		return meanFloats(x)
	}
	return meanFloats(x[cut : len(x)-cut])
}
