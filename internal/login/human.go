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
		sleep(4 + rand.IntN(12))
	}
	sleep(40 + rand.IntN(90))
	if err := page.Mouse().Down(); err != nil {
		return loc.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(8000)})
	}
	sleep(30 + rand.IntN(70))
	if err := page.Mouse().Up(); err != nil {
		return loc.Click(playwright.LocatorClickOptions{Timeout: playwright.Float(8000)})
	}
	sleep(80 + rand.IntN(140))
	return nil
}

func humanType(page playwright.Page, loc playwright.Locator, value string) error {
	if err := humanClick(page, loc); err != nil {
		return err
	}
	sleep(120 + rand.IntN(180))
	_ = page.Keyboard().Press("Control+a")
	sleep(30 + rand.IntN(40))
	_ = page.Keyboard().Press("Backspace")
	for _, r := range value {
		if err := page.Keyboard().Type(string(r), playwright.KeyboardTypeOptions{
			Delay: playwright.Float(float64(36 + rand.IntN(70))),
		}); err != nil {
			return loc.Type(value, playwright.LocatorTypeOptions{
				Timeout: playwright.Float(12000),
				Delay:   playwright.Float(55),
			})
		}
		sleep(18 + rand.IntN(55))
		if rand.IntN(11) == 0 {
			sleep(160 + rand.IntN(220))
		}
	}
	sleep(200 + rand.IntN(180))
	return nil
}

func humanIdleAuthkit(page playwright.Page, log Logger) {
	if log != nil {
		log("在 AuthKit 停留，让 Radar 采集页面信号")
	}
	for i := 0; i < 3; i++ {
		x := 80 + rand.Float64()*400
		y := 80 + rand.Float64()*240
		_ = page.Mouse().Move(x, y)
		sleep(350 + rand.IntN(400))
	}
	sleep(1200 + rand.IntN(800))
}

func humanScroll(page playwright.Page) {
	for i := 0; i < 4+rand.IntN(3); i++ {
		_ = page.Mouse().Wheel(0, 280+rand.Float64()*220)
		sleep(90 + rand.IntN(110))
	}
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
