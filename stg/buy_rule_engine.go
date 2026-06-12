package stg

import (
	"RESTGo/box"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
)

// ConditionFn 은 매수 조건 평가 함수 타입
type ConditionFn func(ctx *box.TradingContext, s Settings) bool

// RuleConfig 는 YAML에서 읽어들이는 전략 정의
type RuleConfig struct {
	Name        string   `yaml:"name"`
	DefCount    int      `yaml:"def_count"`     // 정확히 일치 (0 = 무관)
	DefCountMin int      `yaml:"def_count_min"` // 이상 조건 (0 = 무관)
	When        []string `yaml:"when"`          // AND 조건
	AnyOf       []string `yaml:"any_of"`        // OR 조건 (하나 이상)
	WhenNot     []string `yaml:"when_not"`      // 반드시 false여야 하는 조건
	Signal      string   `yaml:"signal"`
}

// strategiesFile 은 YAML 파일 최상위 구조
type strategiesFile struct {
	Strategies []RuleConfig `yaml:"strategies"`
}

// LoadRules 는 YAML 파일에서 전략 목록을 읽어 반환
func LoadRules(path string) ([]RuleConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg strategiesFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg.Strategies, nil
}

// EvaluateRules 는 등록된 전략 목록에 대해 순서대로 평가하고
// 첫 번째로 매칭된 전략의 신호 이름을 반환. 없으면 "".
// C# BuyDecisionProcessor.Options의 LastBuySignalPosition_StrategyN 포팅:
// 같은 DefBox 구간에서 이미 발화한 전략은 건너뛴다 (중복 신호 방지).
// 기록은 DefBox 변경 시 analyzer의 ResetBuySignalPositions()로 초기화된다.
func EvaluateRules(rules []RuleConfig, ctx *box.TradingContext, s Settings) (string, string) {
	for _, rule := range rules {
		// 전략별 중복 신호 방지: 이번 DefBox에서 이미 발화한 전략은 스킵
		// (C#과 동일하게 다음 전략은 계속 평가됨)
		if _, fired := ctx.LastBuySignalPosition[rule.Name]; fired {
			continue
		}

		// DefCount 정확 일치 필터
		if rule.DefCount > 0 && ctx.DefCount != rule.DefCount {
			continue
		}
		// DefCount 최솟값 필터
		if rule.DefCountMin > 0 && ctx.DefCount < rule.DefCountMin {
			continue
		}

		// when: 모두 true여야 함
		passed := true
		for _, name := range rule.When {
			fn, ok := conditionRegistry[name]
			if !ok {
				fmt.Printf("[rule] 미등록 조건: %s\n", name)
				passed = false
				break
			}
			if !fn(ctx, s) {
				passed = false
				break
			}
		}
		if !passed {
			continue
		}

		// when_not: 모두 false여야 함
		for _, name := range rule.WhenNot {
			fn, ok := conditionRegistry[name]
			if !ok {
				fmt.Printf("[rule] 미등록 조건: %s\n", name)
				passed = false
				break
			}
			if fn(ctx, s) {
				passed = false
				break
			}
		}
		if !passed {
			continue
		}

		// any_of: 하나 이상 true여야 함
		if len(rule.AnyOf) > 0 {
			anyPassed := false
			for _, name := range rule.AnyOf {
				fn, ok := conditionRegistry[name]
				if ok && fn(ctx, s) {
					anyPassed = true
					break
				}
			}
			if !anyPassed {
				continue
			}
		}

		// 발화 위치 기록 → 같은 DefBox에서 이 전략의 재발화 차단
		ctx.LastBuySignalPosition[rule.Name] = ctx.Position
		return rule.Signal, rule.Name
	}
	return "", ""
}

// conditionRegistry 는 조건명 → 평가 함수 매핑
var conditionRegistry = map[string]ConditionFn{}

// RegisterCondition 은 조건명과 함수를 등록
func RegisterCondition(name string, fn ConditionFn) {
	conditionRegistry[name] = fn
}

// ConditionRegistryGet 은 조건명으로 함수를 조회 (테스트/검증용)
func ConditionRegistryGet(name string) (ConditionFn, bool) {
	fn, ok := conditionRegistry[name]
	return fn, ok
}
