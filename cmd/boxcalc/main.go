// boxcalc — 캔들 JSON(stdin) → Box 분석 JSON(stdout) 독립 바이너리.
//
// Box 로직의 단일 소스(box/·stg/)를 외부 소비자(makesql_chart, py 차트)에
// 노출하기 위한 최소 래퍼다. DB·RabbitMQ·config.yaml 등 인프라 무의존이라
// CGO_ENABLED=0 정적 빌드로 어느 컨테이너에나 복사해 쓸 수 있다.
//
// 빌드:  CGO_ENABLED=0 go build -ldflags="-s -w" -o boxcalc ./cmd/boxcalc
// 사용:  ./RESTGo stock candlesjson 005930 250 | ./boxcalc
//
// 입력 (stdin):
//
//	{"shcode":"005930","candles":[{"date":"20260102","open":1,"high":2,"low":1,"close":2,"volume":100}, ...]}
//	또는 candles 배열만: [{...}, ...]
//	날짜 오름차순이어야 한다 (DB 로더와 동일 규약).
//
// 출력 (stdout): boxes / buy_signals — 키 이름은 기존 py 차트 dict 규약(pos/price/kind/boxtype)을 따른다.
// 매수 신호는 임베드된 기본 전략(rules/strategy1.yaml)으로 평가한다. 매도 룰은 로드하지 않는다.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"RESTGo/box"
	"RESTGo/indicator"
	"RESTGo/rules"
	"RESTGo/stg"
)

type inputCandle struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type input struct {
	Shcode  string        `json:"shcode"`
	Candles []inputCandle `json:"candles"`
}

type outputBox struct {
	Date        string  `json:"date"`
	Pos         int     `json:"pos"`      // 캔들 인덱스 (BoxPosition)
	Price       float64 `json:"price"`    // 원본 가격 (PriceOrigin)
	Kind        int     `json:"kind"`     // 0:Box 1:MainBox 2:DefBox 3:MultiBox
	BoxType     int     `json:"boxtype"`  // 0:Support 1:Resist 2:Unknown
	DefList     []int   `json:"def_list"`
	MainDefLink []int   `json:"main_def_link"`
}

type outputSignal struct {
	Date     string `json:"date"`
	Position int    `json:"position"`
	Reason   string `json:"reason"`
}

type output struct {
	Shcode      string         `json:"shcode"`
	CandleCount int            `json:"candle_count"`
	Boxes       []outputBox    `json:"boxes"`
	BuySignals  []outputSignal `json:"buy_signals"`
}

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fatalf("stdin 읽기 실패: %v", err)
	}

	var in input
	if err := json.Unmarshal(data, &in); err != nil {
		// 객체 형식이 아니면 캔들 배열 단독 형식으로 재시도
		if arrErr := json.Unmarshal(data, &in.Candles); arrErr != nil {
			fatalf("입력 JSON 파싱 실패: %v", err)
		}
	}
	if len(in.Candles) < 6 {
		fatalf("분석에 필요한 최소 캔들 수(6)가 부족합니다: %d", len(in.Candles))
	}

	candles := make([]*box.Candle, len(in.Candles))
	for i, c := range in.Candles {
		candles[i] = &box.Candle{
			Shcode:      in.Shcode,
			Date:        c.Date,
			OpenOrigin:  c.Open,
			HighOrigin:  c.High,
			LowOrigin:   c.Low,
			CloseOrigin: c.Close,
			Volume:      c.Volume,
		}
	}

	indicator.PrepareCandles(candles)

	ruleList, settings, err := stg.ParseRulesWithSettings(rules.Strategy1YAML)
	if err != nil {
		fatalf("임베드 전략(strategy1) 파싱 실패: %v", err)
	}

	result := stg.AnalyzeWithRules(candles, ruleList, settings)

	out := output{
		Shcode:      in.Shcode,
		CandleCount: len(candles),
		Boxes:       make([]outputBox, 0, len(result.BoxList)),
		BuySignals:  make([]outputSignal, 0, len(result.BuySignals)),
	}
	for _, b := range result.BoxList {
		out.Boxes = append(out.Boxes, outputBox{
			Date:        b.Date,
			Pos:         b.BoxPosition,
			Price:       b.PriceOrigin,
			Kind:        b.KindOfBox,
			BoxType:     b.BoxType,
			DefList:     b.DefList,
			MainDefLink: b.MainDefLink,
		})
	}
	for _, s := range result.BuySignals {
		out.BuySignals = append(out.BuySignals, outputSignal{
			Date:     s.Date,
			Position: s.Position,
			Reason:   s.Reason,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(out); err != nil {
		fatalf("출력 JSON 인코딩 실패: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "boxcalc: "+format+"\n", args...)
	os.Exit(1)
}
