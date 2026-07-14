package console

import (
	"fmt"
	"log"
	"os"
)

// init 은 패키지의 유일한 init() 함수입니다.
// main() 실행 전에 자동으로 호출되어 모든 초기화를 수행합니다.
func init() {
	// Step 1: Config 로드 (config.yaml 읽기 + 복호화)
	if err := loadConfig(); err != nil {
		fmt.Printf("%s [Init] Config 로딩 실패: %v\n", GenerateTimestampedString(), err)
		os.Exit(1)
	}
	fmt.Printf("%s [Init] Config 로드 완료\n", GenerateTimestampedString())

	// Step 2: MSSQL 연결 초기화
	initMsConn()

	// key DB 연결
	if err := MsConn.EnsureConnection("key"); err != nil {
		log.Fatalf("key DB 연결 실패: %v", err)
	}

	// key DB에서 추가 설정 로드
	db, err := MsConn.GetDB("key")
	if err != nil {
		log.Fatalf("key DB 가져오기 실패: %v", err)
	}

	configs, err := getDBConfigs(db, "DB")
	if err != nil {
		log.Fatalf("DB 설정 가져오기 실패: %v", err)
	}

	for _, config := range configs {
		switch config.KEY_NAME {
		case "ADDR_VAR":
			EnvVar.MSSQL_ADDR = config.VALUE_DATA
		case "DBNAME_VAR":
			EnvVar.MSSQL_DBVar = config.VALUE_DATA
		case "ADDR_HAN":
			EnvHan.MSSQL_ADDR = config.VALUE_DATA
		case "DBNAME_HAN":
			EnvHan.MSSQL_DBHan = config.VALUE_DATA
		case "ADDR_KIS":
			EnvKIS.MSSQL_ADDR = config.VALUE_DATA
		case "DBNAME_KIS":
			EnvKIS.MSSQL_DBKIS = config.VALUE_DATA
		case "ADDR_MONG":
			EnvMong.Mongo_ADDR = config.VALUE_DATA
		case "PORT_MONG":
			EnvMong.Mongo_PORT = config.VALUE_DATA
		case "ADDR_TUF":
			EnvTUF.MSSQL_ADDR = config.VALUE_DATA
		case "DBNAME_TUF":
			EnvTUF.MSSQL_DBTUF = config.VALUE_DATA
		}
	}

	// KIS DB 설정 미등록 시 han과 동일 서버 사용
	if EnvKIS.MSSQL_ADDR == "" {
		EnvKIS.MSSQL_ADDR = EnvHan.MSSQL_ADDR
	}
	if EnvKIS.MSSQL_DBKIS == "" {
		EnvKIS.MSSQL_DBKIS = "KIS2"
	}

	// TUF DB 기본값 (KeyValueStore에 미등록 시)
	if EnvTUF.MSSQL_ADDR == "" {
		EnvTUF.MSSQL_ADDR = "tuf.tail5b4272.ts.net"
	}
	if EnvTUF.MSSQL_DBTUF == "" {
		EnvTUF.MSSQL_DBTUF = "Upbit"
	}

	// 나머지 DB 연결
	if err := MsConn.EnsureConnection("han"); err != nil {
		log.Fatalf("han DB 연결 실패: %v", err)
	}
	if err := MsConn.EnsureConnection("var"); err != nil {
		log.Fatalf("var DB 연결 실패: %v", err)
	}
	if err := MsConn.EnsureConnection("KIS2"); err != nil {
		if os.Getenv("RESTGO_DEGRADE_KIS2") == "true" {
			log.Printf("KIS2 DB 연결 실패, tuf 서버로 폴백 시도: %v", err)
			// 2026-07-11: KIS2 기본 서버 장애 → tuf 서버 주소 + KIS2 데이터베이스명으로 폴백
			if fallbackErr := MsConn.initDBWithParams("KIS2", EnvTUF.MSSQL_ADDR, EnvKIS.MSSQL_DBKIS); fallbackErr != nil {
				log.Printf("KIS2 tuf 폴백도 실패 — KIS2 의존 기능 비활성: %v", fallbackErr)
				MsConn.SetUnavailable("KIS2")
			} else {
				log.Printf("KIS2 tuf 폴백 연결 성공 — 정상 운영")
			}
		} else {
			log.Fatalf("KIS2 DB 연결 실패: %v", err)
		}
	}
	// TUF DB 연결 (비필수: 15분봉 암호화폐 분석 기능에만 사용)
	if err := MsConn.EnsureConnection("tuf"); err != nil {
		log.Printf("tuf DB 연결 실패 (비필수): %v", err)
		// non-fatal: 15분봉 분석 기능만 비활성
	}
	fmt.Printf("%s [Init] MSSQL 연결 초기화 완료\n", GenerateTimestampedString())

	// Step 3: RabbitMQ 세션 초기화
	if err := initRabbitMQSession(); err != nil {
		fmt.Printf("%s [Init] RabbitMQ 세션 초기화 실패: %v\n", GenerateTimestampedString(), err)
		os.Exit(1)
	}
	for _, queueName := range Env.RABBITMQ_QUEUE {
		if err := RabbitMQSession.AddChannelAndQueue(queueName); err != nil {
			fmt.Printf("%s [Init] RabbitMQ 채널/큐 설정 실패 (%s): %v\n", GenerateTimestampedString(), queueName, err)
			os.Exit(1)
		}
	}
	fmt.Printf("%s [Init] RabbitMQ 세션 초기화 완료\n", GenerateTimestampedString())

	// Step 4: 로거 배치 프로세서 시작
	initLogger()
	Log("[Init] 로거 배치 프로세서 시작")

	// Step 5: RabbitMQ 모니터링 고루틴 시작
	go RabbitMQSession.MonitorConnection()
	go RabbitMQSession.HandleReconnection()

	Log("[Init] 모든 초기화 완료 - main() 준비됨")
}
