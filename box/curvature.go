package box

// AnalyzeCurvature 는 현재 캔들의 곡률(추세 방향)을 분석하고 필요 시 Box를 생성
// 반환: 새 CurveKey (상승=1, 하락=-1, 유지=이전값)
func AnalyzeCurvature(ctx *TradingContext) int {
	pos := ctx.Position
	if pos < 2 {
		return 0
	}

	cur := ctx.CandleList[pos]
	prev1 := ctx.CandleList[pos-1]
	prev2 := ctx.CandleList[pos-2]
	prevCurveKey := prev1.Curvekey

	switch {
	case prevCurveKey > 0:
		if ShouldReverseToBearish(cur, prev1, prev2) {
			highest := FindHighestPrice(ctx.CandleList, ctx.Exposition, pos)
			if highest.Price > 0 {
				AddHighBox(ctx.CandleList, &ctx.BoxList, highest.Position, highest.Price, pos, highest.PriceOrigin)
			}
			return prevCurveKey * -1
		}
		return prevCurveKey

	case prevCurveKey < 0:
		if ShouldReverseToBullish(cur, prev1, prev2) {
			lowest := FindLowestPrice(ctx.CandleList, ctx.Exposition, pos)
			if lowest.Price < 1e9 {
				AddLowBox(ctx.CandleList, &ctx.BoxList, lowest.Position, lowest.Price, pos, lowest.PriceOrigin)
			}
			return prevCurveKey * -1
		}
		return prevCurveKey
	}

	return prevCurveKey
}

// CheckAndCreateDefBox 는 상승 추세에서 DefBox 생성 조건을 확인하고 생성
func CheckAndCreateDefBox(ctx *TradingContext, damageLimit int) {
	if len(ctx.BoxList) < 2 || ctx.Position < 1 {
		return
	}

	prevCurveKey := ctx.CandleList[ctx.Position-1].Curvekey
	if prevCurveKey <= 0 {
		return
	}

	cur := ctx.CandleList[ctx.Position]
	prev := ctx.CandleList[ctx.Position-1]

	if !IsDefBoxEntryCondition(ctx.BoxList, cur, prev) {
		return
	}

	boxIndex := 0
	for _, b := range ctx.BoxList {
		if b.BoxPosition < ctx.Position {
			boxIndex++
		}
	}
	if boxIndex <= 0 {
		return
	}

	exposition := ctx.BoxList[boxIndex-1].CurvePosition
	prices := FindDefBoxPrices(ctx.CandleList, exposition, ctx.Position)

	for i := 0; i < boxIndex-1; i++ {
		b := ctx.BoxList[i]

		if !ShouldCreateDefBox(b, prices, ctx.CandleList, ctx.Position, damageLimit) {
			continue
		}

		if b.KindOfBox != KindDefBox {
			b.KindOfBox = KindMainBox
		}
		b.DefList = append(b.DefList, ctx.Position)

		existingIdx := FindExistingDefBoxAtPosition(ctx.BoxList, prices.HighPosition)

		if existingIdx >= 0 {
			existing := ctx.BoxList[existingIdx]
			alreadyLinked := false
			for _, linked := range existing.MainDefLink {
				if linked == i {
					alreadyLinked = true
					break
				}
			}
			if !alreadyLinked {
				existing.MainDefLink = append(existing.MainDefLink, i)
			}
		} else if ShouldUpdateDefBox(ctx.BoxList, prices.HighPosition, b.Price) {
			priceOrigin := b.PriceOrigin
			if priceOrigin <= 0 {
				priceOrigin = b.Price
			}
			CreateDefBox(&ctx.BoxList, ctx.CandleList, prices.HighPosition, b.Price, ctx.Position, i, priceOrigin)
		}

		ctx.DefChecker++
	}
}

// CalculateExposition 는 Curvekey가 변경될 때 새 Exposition 위치를 계산
func CalculateExposition(lastBox *Box) int {
	if lastBox.BoxPosition+1 >= lastBox.CurvePosition-3 {
		return lastBox.BoxPosition + 1
	}
	return lastBox.CurvePosition - 2
}
