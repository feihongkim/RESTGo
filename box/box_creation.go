package box

// CreateBox 는 새 Box 생성
func CreateBox(date string, boxPosition int, price float64, curvePosition int, boxType, kindOfBox int, priceOrigin float64) *Box {
	origin := priceOrigin
	if origin <= 0 {
		origin = price
	}
	return &Box{
		Date:          date,
		BoxPosition:   boxPosition,
		Price:         price,
		PriceOrigin:   origin,
		CurvePosition: curvePosition,
		BoxType:       boxType,
		KindOfBox:     kindOfBox,
		DefList:       []int{},
		MainDefLink:   []int{},
	}
}

func isDuplicateBox(boxList []*Box, kindOfBox, boxPosition int, price float64) bool {
	if len(boxList) == 0 {
		return false
	}
	last := boxList[len(boxList)-1]
	return last.KindOfBox == kindOfBox &&
		last.BoxPosition == boxPosition &&
		last.Price == price
}

// AddHighBox 는 상승->하락 전환 시 고점 저항선 박스 생성 및 추가
func AddHighBox(candles []*Candle, boxList *[]*Box, boxDay int, boxPrice float64, position int, priceOrigin float64) {
	if len(*boxList) > 0 {
		last := (*boxList)[len(*boxList)-1]
		// 직전이 DefBox이고 같은 위치/가격이면 추가 안 함 (Python 동일 로직)
		if last.KindOfBox == KindDefBox && last.BoxPosition == boxDay && last.Price == boxPrice {
			return
		}
	}
	if isDuplicateBox(*boxList, KindBox, boxDay, boxPrice) {
		return
	}
	box := CreateBox(candles[boxDay].Date, boxDay, boxPrice, position, BoxTypeResistance, KindBox, priceOrigin)
	*boxList = append(*boxList, box)
}

// AddLowBox 는 하락->상승 전환 시 저점 지지선 박스 생성 및 추가
func AddLowBox(candles []*Candle, boxList *[]*Box, lowestPos int, boxPrice float64, position int, priceOrigin float64) {
	if isDuplicateBox(*boxList, KindBox, lowestPos, boxPrice) {
		return
	}
	box := CreateBox(candles[lowestPos].Date, lowestPos, boxPrice, position, BoxTypeSupport, KindBox, priceOrigin)
	*boxList = append(*boxList, box)
}

// CreateDefBox 는 DefBox를 생성하고 BoxList에 추가
func CreateDefBox(boxList *[]*Box, candles []*Candle, highPosition int, boxPrice float64, curvePosition, mainBoxIndex int, priceOrigin float64) {
	origin := priceOrigin
	if origin <= 0 {
		origin = boxPrice
	}
	defBox := &Box{
		Date:          candles[highPosition].Date,
		BoxPosition:   highPosition,
		Price:         boxPrice,
		PriceOrigin:   origin,
		CurvePosition: curvePosition,
		BoxType:       BoxTypeResistance,
		KindOfBox:     KindDefBox,
		DefList:       []int{},
		MainDefLink:   []int{},
	}
	mainBox := (*boxList)[mainBoxIndex]
	if mainBox.KindOfBox == KindDefBox {
		defBox.MainDefLink = append(defBox.MainDefLink, mainBoxIndex)
	} else {
		defBox.MainDefLink = []int{mainBoxIndex}
	}
	*boxList = append(*boxList, defBox)
}

// FindExistingDefBoxAtPosition 는 BoxList에서 동일한 BoxPosition을 가진 DefBox 인덱스를 반환 (-1이면 없음)
func FindExistingDefBoxAtPosition(boxList []*Box, highPosition int) int {
	for j := len(boxList) - 1; j >= 0; j-- {
		if boxList[j].KindOfBox == KindDefBox && boxList[j].BoxPosition == highPosition {
			return j
		}
	}
	return -1
}
