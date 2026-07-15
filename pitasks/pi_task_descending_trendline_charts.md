# 위임 작업: 하락추세선 돌파 전략 샘플 차트 5개

## 요청자/보고
- Telegram chat_id: `7723743534`
- 시작/완료 메시지 `[pi]` 접두사
- 완성 PNG 5개를 `telegram_send_photo`로 직접 전송

## 목적
현재 미커밋 연구 중인 `DescendingTrendlineBreakout` detector의 실제 OOS 패턴을 차트로 검증한다.

관련 파일:
- `stg/descending_trendline_analyze.go`
- `stg/descending_trendline_analyze_test.go`
- `study/descending_trendline_study.go`
- 결과: `zpicture/descending_trendline_study.json`

기존 전략/YAML/analyzer/registry 수정 금지. 현재 미커밋 변경 보존. 커밋 금지.

## 대상 설정
차트 가독성과 h20 탐색 상위였던 아래 고정 설정을 사용:
- Variant: `S08_D03_B60_W20`
- Mode: `STRUCTURE`
- support tolerance: 8%
- minimum R1→R2 resistance drop: 3%
- minimum pattern duration: 60 bars
- maximum pattern duration: 180 bars
- S2 confirmation 후 breakout wait: 20 bars
- no MA/volume gate
- buy: breakout confirmation 다음 봉 시가
- display exit: buy 후 20봉 종가
- return: 왕복비용 0.30% 차감

## 표본 선정
체리피킹 방지를 위해 OOS(entry_date >= 20220101) 전체 유효 체결의 h20 비용후 수익률 분포에서 서로 다른 종목의:
1. P90
2. P75
3. P50
4. P25
5. P10
각 1개를 결정적으로 선택한다.

기업행위 비조정 데이터 필터는 study와 동일하게 적용:
- R1부터 entry까지, entry부터 h20까지 인접 종가 비율이 0.5 미만 또는 1.5 초과면 제외.

## 구현
1. 필요하면 `study/descending_trendline_chart.go` 및 CLI `descending_trendline_charts`를 add-only로 추가하여 차트 JSON을 생성.
2. JSON에는 각 표본의 candles window, R1/S1/R2/S2 BoxPosition/CurvePosition, 추세선, 수평 floor, breakout, next-open buy, +20 close sell, net return 포함.
3. `py/analysis/descending_trendline_chart.py`를 추가해 PNG 5개 생성.
4. 컨테이너에 matplotlib가 없으면 white host venv 사용:
   `/home/feihong/code/REST/RESTGo/venv/bin/python3`
5. 산출물:
   - `zpicture/descending_trendline_chart_samples.json`
   - `zpicture/descending_trendline_P90.png`
   - `zpicture/descending_trendline_P75.png`
   - `zpicture/descending_trendline_P50.png`
   - `zpicture/descending_trendline_P25.png`
   - `zpicture/descending_trendline_P10.png`

## 차트 필수 표시
- 일봉 candle
- MA20, MA60
- R1/S1/R2/S2의 실제 pivot와 label
- R1-R2 하락추세선(돌파일까지 연장)
- S1/S2 평균 수평 지지선
- breakout 확인봉
- BUY: 다음 봉 시가
- SELL: +20봉 종가
- 종목코드, 날짜, 가격, 비용후 h20 수익률
- P90/P75/P50/P25/P10 표본임을 명시

## 검증
- signal이 실제 `DescendingTrendlineAnalyze` 결과인지 재검증
- Box의 CurvePosition이 breakout 이전인지 확인
- trendline 계산이 R1/R2를 통과하는지 확인
- buy=fire+1 open, sell=entry+20 close
- net=(sell/buy-1)*100-0.30
- `RESTGO_DEGRADE_KIS2=true go test ./stg ./study ./stock`
- `go vet ./...`, `go build ./...`, `git diff --check`
- 기존 strategy1/YAML/analyzer/registry diff 0

## 완료 보고
각 PNG caption에 percentile, 종목, R1/S1/R2/S2 기간, buy/sell 날짜, 비용후 h20 수익률을 표시. 이 5개가 승자부터 패자까지의 분포 표본이며 전략 성과가 기각됐다는 점도 마지막 메시지에 명시한다.
