package box

// KindOfBox 값 상수
const (
	KindBox      = 0 // 일반 Box (추세 전환점에서 생성)
	KindMainBox  = 1 // MainBox (DefBox와 연결된 주요 저항/지지선)
	KindDefBox   = 2 // DefBox (저항선 재확인 패턴, 매수 신호 핵심)
	KindMultiBox = 3 // MultiBox (여러 MainBox와 연결된 DefBox)
)

// BoxType 값 상수
const (
	BoxTypeSupport    = 0 // 지지선 (하락->상승 전환, 저점)
	BoxTypeResistance = 1 // 저항선 (상승->하락 전환, 고점)
	BoxTypeUnknown    = 2 // 미분류 (초기 상태)
)

// Box 는 추세 전환점 또는 저항/지지선을 나타내는 핵심 구조체
type Box struct {
	Date          string  `json:"date"`
	Price         float64 `json:"price"`         // 스케일된 박스 가격
	PriceOrigin   float64 `json:"priceOrigin"`   // 원본(스케일 전) 가격
	BoxPosition   int     `json:"boxPosition"`   // 박스가 위치한 캔들 인덱스
	CurvePosition int     `json:"curvePosition"` // 곡률 전환이 발생한 캔들 인덱스
	KindOfBox     int     `json:"kindOfBox"`     // 0:일반 1:MainBox 2:DefBox 3:MultiBox
	BoxType       int     `json:"boxType"`       // 0:지지선 1:저항선 2:미분류
	DefList       []int   `json:"defList"`       // DefBox 생성 시 기록된 캔들 Position 목록
	MainDefLink   []int   `json:"mainDefLink"`   // DefBox가 참조하는 MainBox의 BoxList 인덱스 목록
}

// Candle 은 일봉(OHLCV) 및 지표 데이터 구조체
type Candle struct {
	Shcode string `json:"shcode"`
	Hname  string `json:"hname"`
	Date   string `json:"date"`
	Time   string `json:"time"`

	// 원본 가격 (스케일 전)
	OpenOrigin  float64 `json:"OpenT"`
	CloseOrigin float64 `json:"CloseT"`
	HighOrigin  float64 `json:"HighT"`
	LowOrigin   float64 `json:"LowT"`

	// 스케일된 가격 (분석용)
	Open  float64 `json:"-"`
	Close float64 `json:"-"`
	High  float64 `json:"-"`
	Low   float64 `json:"-"`

	Volume float64 `json:"VolumeT"`

	// 이동평균 (원본)
	Ma5Origin   float64 `json:"Ma005"`
	Ma20Origin  float64 `json:"Ma020"`
	Ma60Origin  float64 `json:"Ma060"`
	Ma120Origin float64 `json:"Ma120"`

	// 이동평균 (스케일)
	Ma5   float64 `json:"-"`
	Ma20  float64 `json:"-"`
	Ma60  float64 `json:"-"`
	Ma120 float64 `json:"-"`

	// 거래량 이동평균
	VolMa5  float64 `json:"-"`
	VolMa20 float64 `json:"-"`

	// 기울기 (Gradient)
	Gradient    float64 `json:"gradient"`
	Gradient20  float64 `json:"gradient20"`
	Gradient60  float64 `json:"gradient60"`
	Gradient120 float64 `json:"gradient120"`

	// 곡률 방향: 상승=1, 판단불능=0, 하락=-1
	Curvekey int `json:"curvekey"`

	// ATR
	ATR           float64 `json:"-"`
	ATRPercentage float64 `json:"-"`

	// RSI (period=14)
	RSI float64 `json:"-"`

	// Bollinger Bands (period=20, 2σ, 스케일 기준)
	BollingerUpper float64 `json:"-"`
	BollingerLower float64 `json:"-"`
	BollingerWidth float64 `json:"-"` // (Upper-Lower)/Middle × 100
	BBPercent      float64 `json:"-"` // %B = (Close - Lower) / (Upper - Lower), 0~1+ 범위

	StickCount int `json:"stickCount"`
}
