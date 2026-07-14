package stg

import (
	"RESTGo/box"
	"testing"
)

func TestMainBoxRetestTouchedAllowsFalseBreakAndRecovery(t *testing.T) {
	cfg := DefaultMainBoxRetestConfig()
	c := &box.Candle{LowOrigin: 96, CloseOrigin: 100}
	if !mainBoxRetestTouched(c, 100, cfg) {
		t.Fatal("6% 이내 장중 이탈·종가 회복을 거부")
	}
	c.LowOrigin = 93
	if mainBoxRetestTouched(c, 100, cfg) {
		t.Fatal("과도한 장중 이탈을 허용")
	}
	c.LowOrigin = 98
	c.CloseOrigin = 98
	if mainBoxRetestTouched(c, 100, cfg) {
		t.Fatal("종가 미회복을 허용")
	}
}

func TestMainBoxRetestUndercutAndLongWickModes(t *testing.T) {
	cfg := DefaultMainBoxRetestConfig()
	cfg.RetestMode = MainBoxRetestModeUndercutReclaim
	c := &box.Candle{OpenOrigin: 101, LowOrigin: 99, CloseOrigin: 102}
	if !mainBoxRetestTouched(c, 100, cfg) {
		t.Fatal("undercut+reclaim rejected")
	}
	c.LowOrigin = 100
	if mainBoxRetestTouched(c, 100, cfg) {
		t.Fatal("touch without undercut accepted")
	}
	c.LowOrigin = 99
	cfg.RetestMode = MainBoxRetestModeLongWickReclaim
	cfg.MinLowerWickBody = 1.5
	if !mainBoxRetestTouched(c, 100, cfg) {
		t.Fatal("long lower wick reclaim rejected")
	}
}

func TestMainBoxRetestReboundIsBullishPrevHighEdge(t *testing.T) {
	cfg := DefaultMainBoxRetestConfig()
	candles := []*box.Candle{{HighOrigin: 102, CloseOrigin: 99}, {OpenOrigin: 100, HighOrigin: 105, CloseOrigin: 103}}
	if !mainBoxRetestRebound(candles, 1, 100, cfg) {
		t.Fatal("양봉 전일고가 돌파 반등을 거부")
	}
	candles[1].CloseOrigin = 101
	if mainBoxRetestRebound(candles, 1, 100, cfg) {
		t.Fatal("전일고가 미돌파를 허용")
	}
}

func TestMainBoxRetestMA5Rebound(t *testing.T) {
	cfg := DefaultMainBoxRetestConfig()
	candles := []*box.Candle{{CloseOrigin: 99, Ma5Origin: 100}, {OpenOrigin: 99, CloseOrigin: 102, Ma5Origin: 101}}
	if !mainBoxRetestMA5Rebound(candles, 1, 100, cfg) {
		t.Fatal("MA5 bullish reclaim rejected")
	}
}

func TestMakeMainBoxRetestSignalUsesNextOpen(t *testing.T) {
	c := []*box.Candle{{Date: "B", HighOrigin: 110}, {Date: "R", LowOrigin: 98}, {Date: "F", HighOrigin: 104}, {Date: "E", OpenOrigin: 103}}
	cy := &mainBoxRetestCycle{breakoutPos: 0, defIdx: 2, mainIdx: 1, line: 100, maxHigh: 120, resistancePos: -1, supportPos: -1}
	s := makeMainBoxRetestSignal(MainBoxRetestGroupA, "X", c, cy, 1, 2)
	if s.EntryPos != 3 || s.EntryDate != "E" || s.EntryPriceOrigin != 103 {
		t.Fatalf("next-open 체결 불일치: %+v", s)
	}
}
