package console

import (
	"testing"
	"time"
)

// go-mssqldb는 DECIMAL/NUMERIC/MONEY를 []byte로 돌려준다. %v를 그대로 쓰면
// 종가 5060이 "[53 48 54 48 46 48 48 48 48]"로 찍힌다 (2026-07-31 실제 발생).
func TestFormatValue(t *testing.T) {
	kst := time.FixedZone("KST", 9*3600)

	cases := []struct {
		name string
		in   interface{}
		want string
	}{
		{"NULL", nil, "NULL"},
		{"DECIMAL(ASCII 숫자열)", []byte("5060.0000"), "5060.0000"},
		{"음수 DECIMAL", []byte("-1234.56"), "-1234.56"},
		{"VARCHAR로 온 []byte", []byte("005930"), "005930"},
		{"한글 []byte", []byte("삼성전자"), "삼성전자"},
		{"진짜 바이너리는 16진수", []byte{0x00, 0x01, 0xFF}, "0x0001ff"},
		{"DATE(자정)", time.Date(2026, 7, 31, 0, 0, 0, 0, kst), "2026-07-31"},
		{"DATETIME", time.Date(2026, 7, 31, 16, 30, 5, 0, kst), "2026-07-31 16:30:05"},
		{"큰 float은 지수표기 금지", float64(26804038), "26804038"},
		{"소수 float", float64(1234.5), "1234.5"},
		{"int64", int64(4298), "4298"},
		{"string", "hello", "hello"},
		{"bool", true, "true"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatValue(c.in); got != c.want {
				t.Errorf("formatValue(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestIsPrintable(t *testing.T) {
	cases := []struct {
		in   []byte
		want bool
	}{
		{[]byte("5060.0000"), true},
		{[]byte("삼성전자"), true},
		{[]byte("a\tb\nc"), true}, // 탭·개행은 허용
		{[]byte{0x00}, false},
		{[]byte{0xFF, 0xFE}, false},
		{[]byte(""), true},
	}
	for _, c := range cases {
		if got := isPrintable(c.in); got != c.want {
			t.Errorf("isPrintable(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"abcdef", 10, "abcdef"},
		{"abcdef", 6, "abcdef"},
		{"abcdef", 5, "ab..."},
		{"abcdef", 3, "abc"},
		{"abcdef", 2, "ab"},
	}
	for _, c := range cases {
		if got := truncate(c.s, c.maxLen); got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.s, c.maxLen, got, c.want)
		}
	}
}
