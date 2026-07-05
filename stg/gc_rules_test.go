package stg

import "testing"

// GC 전략 YAML 2종이 로드되고 참조 조건이 전부 레지스트리에 있는지 검증 (2026-07-05)
func TestGCStrategyYAMLsLoad(t *testing.T) {
	for _, path := range []string{"../rules/buy_wdefbox_gc.yaml", "../rules/strategy1_gc.yaml"} {
		rules, _, err := LoadRulesWithSettings(path)
		if err != nil {
			t.Fatalf("%s 로드 실패: %v", path, err)
		}
		if len(rules) == 0 {
			t.Fatalf("%s: 전략 0개", path)
		}
		for _, rule := range rules {
			names := append([]string{}, rule.When...)
			names = append(names, rule.WhenNot...)
			names = append(names, rule.AnyOf...)
			for _, n := range names {
				if _, ok := conditionRegistry[n]; !ok {
					t.Errorf("%s 룰 %s: 미등록 조건 %q", path, rule.Name, n)
				}
			}
			if rule.Trigger != "" && !IsKnownTrigger(rule.Trigger) {
				t.Errorf("%s 룰 %s: 미등록 트리거 %q", path, rule.Name, rule.Trigger)
			}
		}
		found := false
		for _, rule := range rules {
			for _, n := range rule.When {
				if n == "IsGoldenCrossPending" {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%s: IsGoldenCrossPending 게이트가 없음", path)
		}
	}
}
