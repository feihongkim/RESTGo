package stock

import (
	"RESTGo/box"
	"encoding/json"
	"testing"
)

func bars(dates ...string) []box.LSBar {
	out := make([]box.LSBar, 0, len(dates))
	for i, d := range dates {
		out = append(out, box.LSBar{Date: d, Open: float64(i + 1), High: float64(i + 2),
			Low: float64(i), Close: float64(i + 1), Volume: float64(100 * (i + 1))})
	}
	return out
}

func TestWindowEndingAt(t *testing.T) {
	all := bars("20260101", "20260102", "20260105", "20260106", "20260107")

	tests := []struct {
		name      string
		date      string
		n         int
		wantFirst string
		wantLast  string
		wantLen   int
	}{
		{"정확히 맞는 창", "20260106", 2, "20260105", "20260106", 2},
		{"요청보다 데이터가 적으면 있는 만큼", "20260102", 5, "20260101", "20260102", 2},
		{"최신일 기준", "20260107", 3, "20260105", "20260107", 3},
		// 휴장일을 넘겨도 그 이하의 마지막 거래일까지 잘라야 한다.
		{"거래일이 아닌 날짜는 그 이하로 절단", "20260104", 2, "20260101", "20260102", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := windowEndingAt(all, tc.date, tc.n)
			if len(got) != tc.wantLen {
				t.Fatalf("길이 = %d, 기대 %d", len(got), tc.wantLen)
			}
			if got[0].Date != tc.wantFirst || got[len(got)-1].Date != tc.wantLast {
				t.Errorf("창 = %s~%s, 기대 %s~%s",
					got[0].Date, got[len(got)-1].Date, tc.wantFirst, tc.wantLast)
			}
		})
	}

	if got := windowEndingAt(all, "20251231", 5); got != nil {
		t.Errorf("데이터 이전 날짜는 nil이어야 함, got %v", got)
	}
	if got := windowEndingAt(nil, "20260101", 5); got != nil {
		t.Errorf("빈 입력은 nil이어야 함, got %v", got)
	}
}

func TestVirtualBarNormalizesMissingHighLow(t *testing.T) {
	// 장 초반·거래 부진 종목은 고가/저가/시가가 0으로 올 수 있다. 그대로 봉을 만들면
	// low=0이 되어 ATR·%B 등 지표가 통째로 붕괴한다.
	q := box.LSQuote{Shcode: "005930", Price: 71500, Open: 0, High: 0, Low: 0, Volume: 1234}
	b := q.VirtualBar("20260731")
	if b.Open != 71500 || b.High != 71500 || b.Low != 71500 || b.Close != 71500 {
		t.Errorf("0값 보정 실패: %+v", b)
	}
	if b.Date != "20260731" || b.Volume != 1234 {
		t.Errorf("날짜/거래량 불일치: %+v", b)
	}
}

func TestVirtualBarKeepsPriceInsideRange(t *testing.T) {
	// 현재가가 스냅샷의 고/저 범위를 벗어나면(수신 시점 차이) 봉이 모순된다 —
	// 종가가 고가보다 높거나 저가보다 낮은 캔들이 생기지 않아야 한다.
	q := box.LSQuote{Price: 8000, Open: 7000, High: 7900, Low: 6900, Volume: 10}
	b := q.VirtualBar("20260731")
	if b.High < b.Close {
		t.Errorf("고가(%v)가 종가(%v)보다 낮음", b.High, b.Close)
	}

	q2 := box.LSQuote{Price: 6000, Open: 7000, High: 7900, Low: 6900, Volume: 10}
	b2 := q2.VirtualBar("20260731")
	if b2.Low > b2.Close {
		t.Errorf("저가(%v)가 종가(%v)보다 높음", b2.Low, b2.Close)
	}
}

// TestBarsToJSONTypes 는 소비자가 메시지 전체를 버리는 두 가지 타입 실수를 막는다:
// 날짜가 숫자이거나 가격이 문자열이면 parseVirtualMsg가 실패한다 (spec §2·§8-4).
func TestBarsToJSONTypes(t *testing.T) {
	raw, err := json.Marshal(barsToJSON(bars("20260101", "20260102")))
	if err != nil {
		t.Fatalf("마샬 실패: %v", err)
	}
	var decoded [][]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("언마샬 실패: %v", err)
	}
	for i, bar := range decoded {
		if len(bar) != 6 {
			t.Fatalf("bar[%d] 원소 %d개, 6개여야 함", i, len(bar))
		}
		if _, ok := bar[0].(string); !ok {
			t.Errorf("bar[%d][0] 날짜가 문자열이 아님: %T", i, bar[0])
		}
		for j := 1; j <= 5; j++ {
			if _, ok := bar[j].(float64); !ok {
				t.Errorf("bar[%d][%d] 숫자가 아님: %T", i, j, bar[j])
			}
		}
	}
}

// TestPublishMsgOmitsModeForLive 는 실전 발행이 mode를 절대 넣지 않음을 고정한다.
// mode="eod"가 실려 나가면 소비자가 확정봉으로 태깅해 LIVE 불가침 규약이 깨진다.
func TestPublishMsgOmitsModeForLive(t *testing.T) {
	live := publishMsg{V: 1, Shcode: "005930", Bars: barsToJSON(bars("20260101"))}
	raw, _ := json.Marshal(live)
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("언마샬 실패: %v", err)
	}
	if _, present := m["mode"]; present {
		t.Errorf("실전 메시지에 mode가 있으면 안 됨: %s", raw)
	}

	hist := publishMsg{V: 1, Shcode: "005930", Mode: "eod", Bars: barsToJSON(bars("20260101"))}
	raw2, _ := json.Marshal(hist)
	_ = json.Unmarshal(raw2, &m)
	if m["mode"] != "eod" {
		t.Errorf("역사 메시지의 mode는 eod여야 함: %s", raw2)
	}
}

func TestResolveReplayDates(t *testing.T) {
	all := map[string][]box.LSBar{
		"A": bars("20260101", "20260102", "20260105"),
		"B": bars("20260102", "20260105", "20260106"),
	}
	if got := resolveReplayDates(all, "", "", ""); len(got) != 1 || got[0] != "20260106" {
		t.Errorf("기본값은 최신 거래일 하나여야 함: %v", got)
	}
	if got := resolveReplayDates(all, "20260103", "", ""); len(got) != 1 || got[0] != "20260102" {
		t.Errorf("as-of는 그 이하 가장 가까운 거래일이어야 함: %v", got)
	}
	got := resolveReplayDates(all, "", "20260102", "20260105")
	if len(got) != 2 || got[0] != "20260102" || got[1] != "20260105" {
		t.Errorf("from~to 범위가 틀림: %v", got)
	}
}
