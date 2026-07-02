package study

import "math"

// ── 통계 헬퍼 ─────────────────────────────────────────────

func meanFloats(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func varianceFloats(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := meanFloats(xs)
	sum := 0.0
	for _, x := range xs {
		d := x - m
		sum += d * d
	}
	return sum / float64(len(xs)-1)
}

func stdFloats(xs []float64) float64 {
	return math.Sqrt(varianceFloats(xs))
}

func medianFloats(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	cp := make([]float64, len(xs))
	copy(cp, xs)
	sortFloats(cp)
	n := len(cp)
	if n%2 == 1 {
		return cp[n/2]
	}
	return (cp[n/2-1] + cp[n/2]) / 2
}

func sortFloats(xs []float64) {
	if len(xs) < 2 {
		return
	}
	quickSortFloats(xs, 0, len(xs)-1)
}

func quickSortFloats(xs []float64, lo, hi int) {
	if lo >= hi {
		return
	}
	pivot := xs[(lo+hi)/2]
	i, j := lo, hi
	for i <= j {
		for xs[i] < pivot {
			i++
		}
		for xs[j] > pivot {
			j--
		}
		if i <= j {
			xs[i], xs[j] = xs[j], xs[i]
			i++
			j--
		}
	}
	if lo < j {
		quickSortFloats(xs, lo, j)
	}
	if i < hi {
		quickSortFloats(xs, i, hi)
	}
}

func welchTStatFloats(g1, g2 []float64) float64 {
	n1, n2 := float64(len(g1)), float64(len(g2))
	if n1 < 2 || n2 < 2 {
		return 0
	}
	m1, m2 := meanFloats(g1), meanFloats(g2)
	v1, v2 := varianceFloats(g1), varianceFloats(g2)
	se := math.Sqrt(v1/n1 + v2/n2)
	if se == 0 {
		return 0
	}
	return (m1 - m2) / se
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// percentile 은 정렬된 슬라이스의 q-percentile (0~100)
func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * q / 100.0)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
