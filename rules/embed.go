// Package rules 는 전략 YAML 파일을 바이너리에 임베드한다.
//
// boxcalc처럼 파일시스템에 rules/ 디렉토리가 없는 독립 바이너리가
// 기본 전략(strategy1)을 코드 복제 없이 사용할 수 있게 한다.
// YAML 원본은 이 디렉토리의 파일이 유일한 소스다.
package rules

import _ "embed"

// Strategy1YAML 은 기본 매수 전략 rules/strategy1.yaml의 원본 바이트.
//
//go:embed strategy1.yaml
var Strategy1YAML []byte
