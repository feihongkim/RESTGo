package box

// BoxPriceResult 는 구간 내 최고/최저가 검색 결과
type BoxPriceResult struct {
	Price       float64
	PriceOrigin float64
	Position    int
	Date        string
}

// FindHighestPrice 는 [startPos, endPos) 구간에서 최고가(High) 위치를 반환
func FindHighestPrice(candles []*Candle, startPos, endPos int) BoxPriceResult {
	result := BoxPriceResult{Price: 0.0}
	for i := startPos; i < endPos && i < len(candles); i++ {
		if candles[i].High > result.Price {
			result.Price = candles[i].High
			result.PriceOrigin = candles[i].HighOrigin
			result.Position = i
			result.Date = candles[i].Date
		}
	}
	return result
}

// FindLowestPrice 는 [startPos, endPos) 구간에서 최저가(Low) 위치를 반환
func FindLowestPrice(candles []*Candle, startPos, endPos int) BoxPriceResult {
	result := BoxPriceResult{Price: 1e9, PriceOrigin: 1e9}
	for i := startPos; i < endPos && i < len(candles); i++ {
		if candles[i].Low < result.Price {
			result.Price = candles[i].Low
			result.PriceOrigin = candles[i].LowOrigin
			result.Position = i
			result.Date = candles[i].Date
		}
	}
	return result
}

// FindMaxClose 는 [startPos, endPos] 구간에서 최고 종가(Close)를 반환
func FindMaxClose(candles []*Candle, startPos, endPos int) float64 {
	maxClose := 0.0
	for i := startPos; i <= endPos && i < len(candles); i++ {
		if candles[i].Close > maxClose {
			maxClose = candles[i].Close
		}
	}
	return maxClose
}

// FindMaxOpen 은 [startPos, endPos] 구간에서 최고 시가(Open)를 반환
func FindMaxOpen(candles []*Candle, startPos, endPos int) float64 {
	maxOpen := 0.0
	for i := startPos; i <= endPos && i < len(candles); i++ {
		if candles[i].Open > maxOpen {
			maxOpen = candles[i].Open
		}
	}
	return maxOpen
}

// DefBoxPrices 는 DefBox 가격 계산에 필요한 고가/종가/시가/위치
type DefBoxPrices struct {
	HighBox      float64
	CloseBox     float64
	OpenBox      float64
	HighPosition int
}

// FindDefBoxPrices 는 구간 내 DefBox 계산에 필요한 가격 정보 반환
func FindDefBoxPrices(candles []*Candle, startPos, endPos int) DefBoxPrices {
	result := DefBoxPrices{}
	start := startPos - 1
	if start < 0 {
		start = 0
	}
	for j := start; j <= endPos && j < len(candles); j++ {
		if candles[j].High > result.HighBox {
			result.HighBox = candles[j].High
			result.HighPosition = j
		}
		if candles[j].Close > result.CloseBox {
			result.CloseBox = candles[j].Close
		}
		if candles[j].Open > result.OpenBox {
			result.OpenBox = candles[j].Open
		}
	}
	return result
}
