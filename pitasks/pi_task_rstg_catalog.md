# 작업: r_stg R 전략 라이브러리 인벤토리·분류 카탈로그 (Phase 0 — 읽기 전용)

## 배경
/workspace/r_stg/ 는 사용자의 2015~16년경 R 전략 라이브러리 (RODBC → hannam DB, 지금과 동일 DB).
목표: 어떤 전략들이 있는지 분류 → 사용자가 이식 우선순위를 정할 수 있는 카탈로그 작성.
이후 단계: 우선순위 확정 → 로직 추출해 trigger_scan으로 hannam 16년 측정 → 유효분만 Go 포팅.

## Telegram 진행 보고 (필수)
telegram_send로 chat_id 7723743534 에 `[pi]` 접두사로 시작/중간(전략 절반 완료 시)/완료 보고.

## 주의
- **한글 인코딩**: .r/.txt 주석이 EUC-KR(CP949) — `iconv -f cp949 -t utf-8 파일` 로 변환해 읽어라.
  변환 실패 시 utf-8 그대로 시도. 주석이 전략 의도의 핵심 단서다.
- **읽기 전용**: r_stg 내 어떤 파일도 수정·삭제 금지. 리포지토리 코드도 수정 금지.
- xlsm/xlsx/docx는 내용 파싱 시도하지 말고 파일명만 카탈로그에 기록 (참고자료 표시).
- st0/st1 같은 날짜 폴더는 구조만 파악 (몇 개 샘플만 열어 산출물 성격 확인).
- .Rout 파일은 과거 실행 출력 — 결과 수치가 남아 있으면 인용하라.

## 대상
1. 루트 .r 파일 전부 (~18개): chck.entry.r, rinos_type.r, ict_kinds.r, exp1_* 2종,
   def_brk_then_retreat.r, longtail/nlongtail/plongtail_test.r, oversold_candidate.r,
   upjong_candidate.r, stg0.r, expert_filter.r, 해외선물bongcnt.r, aggregate_minutes.r,
   merge_query_gForeign.r, save_to_gForeign_5.r, foreign_*.r, ffff.r
2. 전략2 ~ 전략16 폴더 각각: 내부 txt/sql/R 파일 읽고 전략 내용 파악 (png는 개수만)
3. 기타 폴더: st0, st1, longtrend, second_bet, slippage model, au3 — 성격 1줄씩
4. ps1 스크립트들 — 운영 자동화로 분류만 (전략 아님)

## 카탈로그 작성 — `zpicture/r_stg_catalog.md`
전략 하나당:
| 항목 | 내용 |
- 이름/위치 | 가설·로직 1줄 (코드·주석 근거 인용) | 데이터 소스 (국내 일봉 stock_price_kor_d001 /
  분봉 stock_tbl3 등 / 해외) | 사용 지표·조건 요약 | 완성도 (완성 스크리너 / 실험 / 메모 수준) |
  RESTGo 기존 기능과의 겹침 (예: def_brk_then_retreat ↔ DefBoxBreakoutFailure) |
  이식 추천도 상/중/하 + 근거 1줄

마지막에 요약 표: 전체 전략 수 / 국내 일봉 기반 몇 개 / 분봉 필요 몇 개 / 해외 몇 개 /
추천도 상 목록. **모르는 것은 "불명"으로 표기 — 추측으로 채우지 말 것.**

## 마무리
- chown 1000:1000 zpicture/r_stg_catalog.md
- 마지막 응답: 전략 총수, 분류 요약(일봉/분봉/해외/메모), 이식 추천 상위 5개와 근거 1줄씩

## 제약
- 이 지시서는 삭제하지 말 것.
