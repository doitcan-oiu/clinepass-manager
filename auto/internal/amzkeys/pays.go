package amzkeys

import "math"

const (
	PayPerAccount  = 5.3
	MaxPaysPerCard = 3
)

func MaxPays(openAmount float64) int {
	if openAmount <= 0 {
		openAmount = DefaultAmount
	}
	n := int(math.Floor(openAmount/PayPerAccount + 1e-9))
	if n > MaxPaysPerCard {
		n = MaxPaysPerCard
	}
	if n < 1 {
		n = 1
	}
	return n
}

func RemainingPays(openAmount float64, payCount int) int {
	n := MaxPays(openAmount) - payCount
	if n < 0 {
		return 0
	}
	return n
}
