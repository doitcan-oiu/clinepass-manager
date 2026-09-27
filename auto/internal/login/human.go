package login

import (
	"math"
	rand "math/rand/v2"

	"github.com/mxschmitt/playwright-go"
)

type point struct {
	X float64
	Y float64
}

func humanClick(page playwright.Page, loc playwright.Locator) error {
	box, err := loc.BoundingBox()
	if err != nil || box == nil || box.Width < 2 || box.Height < 2 {
		return loc.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(8000)})
	}
	from := point{X: 48 + rand.Float64()*220, Y: 36 + rand.Float64()*140}
	to := point{
		X: box.X + box.Width*(0.28+rand.Float64()*0.44),
		Y: box.Y + box.Height*(0.28+rand.Float64()*0.44),
	}
	for _, p := range bezierPoints(from, to, 14+rand.IntN(10)) {
		if err := page.Mouse().Move(p.X, p.Y); err != nil {
			return loc.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(8000)})
		}
		sleep(2 + rand.IntN(6))
	}
	sleep(15 + rand.IntN(25))
	if err := page.Mouse().Down(); err != nil {
		return loc.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(8000)})
	}
	sleep(10 + rand.IntN(20))
	if err := page.Mouse().Up(); err != nil {
		return loc.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(8000)})
	}
	sleep(20 + rand.IntN(30))
	return nil
}

func humanType(page playwright.Page, loc playwright.Locator, value string) error {
	if err := loc.Fill(value, playwright.LocatorFillOptions{Timeout: playwright.Float(12000)}); err == nil {
		return nil
	}
	if err := humanClick(page, loc); err != nil {
		return err
	}
	_ = page.Keyboard().Press("Control+a")
	_ = page.Keyboard().Press("Backspace")
	return loc.Type(value, playwright.LocatorTypeOptions{
		Timeout: playwright.Float(12000),
		Delay:   playwright.Float(18),
	})
}

func humanScroll(page playwright.Page) {
	_ = page.Mouse().Wheel(0, 360+rand.Float64()*180)
}

func bezierPoints(from, to point, steps int) []point {
	if steps < 6 {
		steps = 6
	}
	c1 := point{
		X: from.X + (to.X-from.X)*(0.25+rand.Float64()*0.2) + (rand.Float64()-0.5)*80,
		Y: from.Y + (to.Y-from.Y)*(0.15+rand.Float64()*0.2) + (rand.Float64()-0.5)*60,
	}
	c2 := point{
		X: from.X + (to.X-from.X)*(0.65+rand.Float64()*0.2) + (rand.Float64()-0.5)*80,
		Y: from.Y + (to.Y-from.Y)*(0.55+rand.Float64()*0.2) + (rand.Float64()-0.5)*60,
	}
	out := make([]point, 0, steps+1)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		u := 1 - t
		out = append(out, point{
			X: u*u*u*from.X + 3*u*u*t*c1.X + 3*u*t*t*c2.X + t*t*t*to.X,
			Y: u*u*u*from.Y + 3*u*u*t*c1.Y + 3*u*t*t*c2.Y + t*t*t*to.Y,
		})
	}
	if last := out[len(out)-1]; math.Abs(last.X-to.X) > 0.5 || math.Abs(last.Y-to.Y) > 0.5 {
		out = append(out, to)
	}
	return out
}
