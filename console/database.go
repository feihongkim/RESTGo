package console

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
)

type msConn struct {
	db          map[string]*sql.DB
	unavailable map[string]bool // degrade 모드에서 사용 불가로 마킹된 DB 목록
	lock        sync.RWMutex
	healthOnce  sync.Once
}

var (
	MsConn *msConn
	dbOnce sync.Once
)

// initMsConn 싱글턴 객체 반환 (init.go에서 호출)
func initMsConn() *msConn {
	dbOnce.Do(func() {
		MsConn = &msConn{
			db:          make(map[string]*sql.DB),
			unavailable: make(map[string]bool),
		}
	})
	return MsConn
}

// initDBWithParams 는 명시적 서버/DB 파라미터로 DB 연결을 생성한다.
// degrade 폴백 시나리오에서 사용 (예: KIS2가 죽었을 때 tuf 서버로 우회).
func (m *msConn) initDBWithParams(dbname, server, database string) error {
	connStr := fmt.Sprintf("server=%s;user id=%s;password=%s;database=%s;encrypt=disable;trustServerCertificate=true;connection timeout=3",
		server, Env.MSSQL_USER, Env.MSSQL_PASSWORD, database)

	db, err := sql.Open("sqlserver", connStr)
	if err != nil {
		return fmt.Errorf("DB 열기 실패 [%s@%s/%s]: %w", dbname, server, database, err)
	}

	db.SetConnMaxIdleTime(10 * time.Minute)
	db.SetConnMaxLifetime(1 * time.Hour)
	db.SetMaxIdleConns(20)
	db.SetMaxOpenConns(100)

	for i := 0; i < 3; i++ {
		if err := db.Ping(); err == nil {
			m.lock.Lock()
			m.db[dbname] = db
			// 폴백 성공 시 unavailable 마킹 해제 (이전 degrade에서 복구)
			delete(m.unavailable, dbname)
			m.lock.Unlock()
			fmt.Printf("%s [MsConn] [%s] DB 연결 완료 (fallback: %s/%s)\n", GenerateTimestampedString(), dbname, server, database)
			return nil
		}
		fmt.Printf("%s [MsConn] [%s] Ping 실패 (%d/3) @%s/%s, 재시도 중...\n", GenerateTimestampedString(), dbname, i+1, server, database)
		time.Sleep(2 * time.Second)
	}

	_ = db.Close()
	return fmt.Errorf("DB Ping 3회 실패 [%s@%s/%s]", dbname, server, database)
}

// DB 연결 생성 및 등록
func (m *msConn) initDB(dbname string) error {
	var connStr string

	switch dbname {
	case "key":
		connStr = fmt.Sprintf("server=%s;user id=%s;password=%s;database=%s;encrypt=disable;trustServerCertificate=true;connection timeout=3",
			Env.MSSQL_ADDR, Env.MSSQL_USER, Env.MSSQL_PASSWORD, Env.MSSQL_DBKEY)
	case "han":
		// hannam 16년치 테이블(stock_price_kor_d001)의 DISTINCT/범위 쿼리는 3초를 초과할 수 있어
		// han만 read timeout을 60초로 설정 (연구 러너 strategy_study/wdefbox_scan --hannam 등)
		connStr = fmt.Sprintf("server=%s;user id=%s;password=%s;database=%s;encrypt=disable;trustServerCertificate=true;connection timeout=60",
			EnvHan.MSSQL_ADDR, Env.MSSQL_USER, Env.MSSQL_PASSWORD, EnvHan.MSSQL_DBHan)
	case "var":
		connStr = fmt.Sprintf("server=%s;user id=%s;password=%s;database=%s;encrypt=disable;trustServerCertificate=true;connection timeout=3",
			EnvVar.MSSQL_ADDR, Env.MSSQL_USER, Env.MSSQL_PASSWORD, EnvVar.MSSQL_DBVar)
	case "KIS2":
		connStr = fmt.Sprintf("server=%s;user id=%s;password=%s;database=%s;encrypt=disable;trustServerCertificate=true;connection timeout=3",
			EnvKIS.MSSQL_ADDR, Env.MSSQL_USER, Env.MSSQL_PASSWORD, EnvKIS.MSSQL_DBKIS)
	case "tuf":
		connStr = fmt.Sprintf("server=%s;user id=%s;password=%s;database=%s;encrypt=disable;trustServerCertificate=true;connection timeout=10",
			EnvTUF.MSSQL_ADDR, Env.MSSQL_USER, Env.MSSQL_PASSWORD, EnvTUF.MSSQL_DBTUF)
	default:
		return fmt.Errorf("알 수 없는 DB 이름: %s", dbname)
	}

	db, err := sql.Open("sqlserver", connStr)
	if err != nil {
		return fmt.Errorf("DB 열기 실패: %w", err)
	}

	db.SetConnMaxIdleTime(10 * time.Minute)
	db.SetConnMaxLifetime(1 * time.Hour)
	db.SetMaxIdleConns(20)  // 비동기 MERGE worker pool 지원: 10 → 20
	db.SetMaxOpenConns(100) // 성능 최적화: 50 → 100 (배치 MERGE 지원)

	for i := 0; i < 3; i++ {
		if err := db.Ping(); err == nil {
			m.db[dbname] = db
			fmt.Printf("%s [MsConn] [%s] DB 연결 완료\n", GenerateTimestampedString(), dbname)
			return nil
		}
		fmt.Printf("%s [MsConn] [%s] Ping 실패 (%d/3), 재시도 중...\n", GenerateTimestampedString(), dbname, i+1)
		time.Sleep(2 * time.Second)
	}

	_ = db.Close()
	return fmt.Errorf("DB Ping 3회 실패: %s", dbname)
}

func (m *msConn) EnsureConnection(dbname string) error {
	m.lock.RLock()
	db, ok := m.db[dbname]
	defer m.lock.RUnlock()

	if ok {
		if err := db.Ping(); err == nil {
			return nil
		}
		fmt.Printf("%s [MsConn] [%s] Ping 실패, 재연결 시도 중...\n", GenerateTimestampedString(), dbname)
		_ = db.Close()
	}

	// 새 연결 시도
	if err := m.initDB(dbname); err != nil {
		delete(m.db, dbname)
		return err
	}
	return nil
}

// SetUnavailable degrade 모드에서 DB를 사용 불가로 마킹 (예: KIS2 연결 실패 시).
func (m *msConn) SetUnavailable(dbname string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.unavailable[dbname] = true
}

// IsAvailable DB가 사용 가능한지 반환.
func (m *msConn) IsAvailable(dbname string) bool {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return !m.unavailable[dbname]
}

// GetDB DB 인스턴스 안전하게 반환
func (m *msConn) GetDB(dbname string) (*sql.DB, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if m.unavailable[dbname] {
		return nil, fmt.Errorf("DB '%s'는 사용 불가능 상태입니다 (degrade)", dbname)
	}

	db, ok := m.db[dbname]
	if !ok {
		return nil, fmt.Errorf("DB '%s'가 등록되지 않았습니다", dbname)
	}
	return db, nil
}

// Close 모든 DB 연결 종료
func (m *msConn) Close() error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if len(m.db) == 0 {
		fmt.Printf("%s [MsConn] 연결된 DB가 없습니다\n", GenerateTimestampedString())
		return nil
	}

	var lastErr error
	for dbname, db := range m.db {
		if err := db.Close(); err != nil {
			fmt.Printf("%s [MsConn] [%s] DB 연결 종료 실패: %v\n", GenerateTimestampedString(), dbname, err)
			lastErr = err
		} else {
			fmt.Printf("%s [MsConn] [%s] DB 연결 종료\n", GenerateTimestampedString(), dbname)
		}
		delete(m.db, dbname)
	}
	return lastErr
}
