# 위임 작업: VW2 첫 눌림 전략 매수·매도 샘플 차트 3개

## 요청자와 보고 채널
- Telegram `chat_id=7723743534`
- 시작/완료 보고와 이미지 전송은 `telegram_send`, `telegram_send_photo` 사용
- 메시지는 `[pi]` 접두사 사용

## 목적
현재 미커밋 상태로 연구 중인 VolumeWave 전략 중 아래 최상위 h20 후보의 실제 과거 매수·매도를 차트 3개로 시각화한다.

대상 변형: `M_S2_W15_D01_08_V05_PH_N`
- source: VW2 (`SourceStage=2`)
- VW2 이후 최대 15봉 안의 첫 눌림
- 눌림 깊이 1~8%
- 눌림 평균 및 진입 확인봉 거래량이 VW2 거래량의 50% 이하
- 양봉 + 전일 고가 돌파로 반등 확인
- 구조 필터 없음
- 매수: 확인봉 **다음 거래일 시가**
- 매도: 매수 후 **20봉째 종가**
- 표시 수익률: 왕복 비용 0.30% 차감

## 표본 선정 원칙
체리피킹하지 말고 OOS(`entry_date >= 20220101`) 전체 체결의 비용후 h20 수익률 분포에서 서로 다른 종목의:
1. P90 사례 1개
2. P50 사례 1개
3. P10 사례 1개
을 결정적으로 선정한다.

## 현재 작업 트리 및 관련 파일
기존 VolumeWave 연구 파일은 미커밋 상태이므로 덮어쓰거나 훼손하지 말 것:
- `cond/buy_volume_wave.go`
- `stg/volume_wave_analyze.go`
- `stg/volume_wave_pullback.go`
- `study/volume_wave_matrix.go`

차트 데이터 덤퍼의 초안이 이미 추가되어 있다:
- `study/volume_wave_chart.go`
- `stock/handler.go`의 `volume_wave_charts` route/help

초안을 먼저 읽고 컴파일·논리 오류를 수정한 뒤 사용하라. 기존 전략 YAML, analyzer, registry는 변경 금지.

## 실행 및 시각화
1. Go detector를 실제 사용해 hannam 16년 전 종목을 스캔하고 차트 JSON을 만든다.
   ```bash
   RESTGO_DEGRADE_KIS2=true go run . stock volume_wave_charts --candles 4200 --out zpicture/volume_wave_chart_samples.json
   ```
2. `py/analysis/volume_wave_chart.py`를 추가해 JSON을 차트 PNG 3개로 렌더링한다.
   - 가격 캔들 + MA20
   - 거래량 + 거래량 MA20
   - VW2 돌파일(추격 금지), 첫 눌림 시작, 반등 확인봉
   - 매수(다음 시가), 매도(+20봉 종가)
   - 종목코드, 날짜, 매수가/매도가, 비용후 수익률
   - 조건 설명
3. 현재 컨테이너 Python에 matplotlib가 없으면 호스트 전용 venv를 사용한다.
   - SSH: `feihong@192.168.3.120`
   - venv: `/home/feihong/code/REST/RESTGo/venv/bin/python3`
   - 필요하면 JSON/스크립트를 `/tmp`로 복사하여 렌더링 후 PNG를 `/workspace/zpicture/`로 되가져온다.
4. PNG 3개를 `telegram_send_photo`로 `chat_id=7723743534`에 각각 전송한다.
   - caption에 P90/P50/P10, 종목, 매수일/매도일, 비용후 수익률을 명시
   - 결과가 승자/중앙/패자 표본임을 명시하여 오해 방지

## 검증
- 표본 날짜가 실제 detector signal과 일치하는지 재검증
- 다음 시가 매수 및 +20봉 종가 매도인지 확인
- 수익률 계산: `(exit_close-entry_open)/entry_open*100 - 0.30`
- `RESTGO_DEGRADE_KIS2=true go test ./stg ./study ./stock`
- `go vet ./...`, `go build ./...`, `git diff --check`
- 기존 `rules/strategy1.yaml`, `rules/sell_default.yaml`, `stg/analyzer.go`, registry diff가 0인지 확인

## 산출물
- `zpicture/volume_wave_chart_samples.json`
- `zpicture/volume_wave_pullback_P90.png`
- `zpicture/volume_wave_pullback_P50.png`
- `zpicture/volume_wave_pullback_P10.png`
- `py/analysis/volume_wave_chart.py`

## 제약
- 커밋하지 말 것.
- 기존 전략과 YAML은 수정하지 말 것.
- 수치를 만들지 말고 JSON의 실측값만 표시할 것.
- 차트 3개를 Telegram으로 보낸 후 핵심 검증 결과를 짧게 완료 보고할 것.
