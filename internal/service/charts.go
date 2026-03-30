package service

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"time"

	"github.com/horoshi10v/opi-thermald/internal/collector"
)

type chartSeries struct {
	Label  string
	Values []float64
	Color  color.RGBA
	Unit   string
}

func bucketize(samples []collector.Sample, bucketCount int, valueFn func(collector.Sample) float64) []float64 {
	if len(samples) == 0 || bucketCount <= 0 {
		return nil
	}

	buckets := make([][]float64, bucketCount)
	start := samples[0].Timestamp
	end := samples[len(samples)-1].Timestamp
	span := end.Sub(start)
	if span <= 0 {
		span = time.Second
	}

	for _, sample := range samples {
		offset := sample.Timestamp.Sub(start)
		index := int((float64(offset) / float64(span)) * float64(bucketCount))
		if index >= bucketCount {
			index = bucketCount - 1
		}
		buckets[index] = append(buckets[index], valueFn(sample))
	}

	result := make([]float64, 0, bucketCount)
	var last float64
	var haveLast bool
	for _, bucket := range buckets {
		if len(bucket) == 0 {
			if haveLast {
				result = append(result, last)
			} else {
				result = append(result, 0)
			}
			continue
		}

		var sum float64
		for _, value := range bucket {
			sum += value
		}
		last = sum / float64(len(bucket))
		haveLast = true
		result = append(result, last)
	}

	return result
}

func renderSummaryChart(title, periodLabel string, temp, cpu, mem, load []float64) ([]byte, error) {
	const (
		width        = 1600
		height       = 1320
		outerPadding = 40
		headerHeight = 110
		footerHeight = 56
		panelGap     = 24
	)

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	bg := color.RGBA{8, 12, 20, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	renderer := newTextRenderer()

	panelBg := color.RGBA{15, 23, 42, 255}
	cardBg := color.RGBA{17, 24, 39, 255}
	grid := color.RGBA{55, 65, 81, 255}
	text := color.RGBA{241, 245, 249, 255}
	muted := color.RGBA{148, 163, 184, 255}
	border := color.RGBA{30, 41, 59, 255}

	series := []chartSeries{
		{Label: "Temperature", Values: temp, Color: color.RGBA{248, 113, 113, 255}, Unit: "C"},
		{Label: "CPU", Values: cpu, Color: color.RGBA{96, 165, 250, 255}, Unit: "%"},
		{Label: "RAM", Values: mem, Color: color.RGBA{251, 191, 36, 255}, Unit: "%"},
		{Label: "Load1", Values: load, Color: color.RGBA{74, 222, 128, 255}, Unit: ""},
	}

	drawRoundedRect(img, image.Rect(outerPadding, outerPadding, width-outerPadding, height-outerPadding), 20, cardBg)
	drawRoundedBorder(img, image.Rect(outerPadding, outerPadding, width-outerPadding, height-outerPadding), 20, border)

	renderer.draw(img, outerPadding+28, outerPadding+44, title, 30, text)
	renderer.draw(img, outerPadding+28, outerPadding+78, "Dark theme summary", 18, muted)

	legendX := outerPadding + 760
	for i, item := range series {
		drawLegendBlock(img, renderer, legendX+i*190, outerPadding+28, item.Color, text, muted, item.Label)
	}

	panelTop := outerPadding + headerHeight
	panelWidth := width - outerPadding*2 - 32
	panelHeight := (height - outerPadding*2 - headerHeight - footerHeight - panelGap*3 - 32) / 4
	panelX := outerPadding + 16
	axisLabels := timeAxisLabels(periodLabel)

	for i, item := range series {
		top := panelTop + i*(panelHeight+panelGap)
		rect := image.Rect(panelX, top, panelX+panelWidth, top+panelHeight)
		drawSeriesPanel(img, renderer, rect, item, grid, text, muted, panelBg, axisLabels)
	}

	renderer.draw(img, outerPadding+28, height-outerPadding-18, "Exports raw data to CSV and sends PNG to Telegram", 18, muted)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func drawSeriesPanel(img *image.RGBA, renderer *textRenderer, rect image.Rectangle, series chartSeries, gridColor, textColor, mutedColor, panelBg color.Color, axisLabels [3]string) {
	drawRoundedRect(img, rect, 16, panelBg)
	drawRoundedBorder(img, rect, 16, gridColor)

	headerY := rect.Min.Y + 30
	renderer.draw(img, rect.Min.X+24, headerY, series.Label, 22, textColor)

	minVal, maxVal := seriesBounds(series.Values)
	statsText := fmt.Sprintf("Min %.1f%s   Max %.1f%s", minVal, series.Unit, maxVal, series.Unit)
	renderer.draw(img, rect.Min.X+220, headerY, statsText, 16, mutedColor)

	scaleWidth := 100
	plotRect := image.Rect(rect.Min.X+scaleWidth, rect.Min.Y+56, rect.Max.X-26, rect.Max.Y-42)

	for i := 1; i < 4; i++ {
		y := plotRect.Min.Y + i*plotRect.Dy()/4
		drawLine(img, plotRect.Min.X, y, plotRect.Max.X, y, gridColor)
	}
	drawFrame(img, plotRect, gridColor)
	drawYAxisLabels(img, renderer, plotRect, minVal, maxVal, series.Unit, mutedColor, textColor)
	drawSeries(img, plotRect, series.Values, series.Color)
	drawXAxisLabels(img, renderer, plotRect, axisLabels, mutedColor)
}

func drawSeries(img *image.RGBA, rect image.Rectangle, values []float64, col color.Color) {
	if len(values) < 2 {
		return
	}

	minVal, maxVal := seriesBounds(values)
	if maxVal == minVal {
		maxVal += 1
	}

	lastX, lastY := 0, 0
	for i, value := range values {
		x := rect.Min.X
		if len(values) > 1 {
			x += i * (rect.Dx() - 1) / (len(values) - 1)
		}
		norm := (value - minVal) / (maxVal - minVal)
		y := rect.Max.Y - 1 - int(norm*float64(rect.Dy()-1))
		fillRect(img, x-3, y-3, 7, 7, col)
		if i > 0 {
			drawLineThick(img, lastX, lastY, x, y, 2, col)
		}
		lastX, lastY = x, y
	}
}

func drawYAxisLabels(img *image.RGBA, renderer *textRenderer, plotRect image.Rectangle, minVal, maxVal float64, unit string, mutedColor, tickColor color.Color) {
	steps := 4
	for i := 0; i <= steps; i++ {
		y := plotRect.Max.Y - 1 - i*(plotRect.Dy()-1)/steps
		value := minVal + (maxVal-minVal)*(float64(i)/float64(steps))
		label := fmt.Sprintf("%.1f%s", value, unit)
		renderer.draw(img, plotRect.Min.X-86, y+6, label, 14, mutedColor)
		drawLine(img, plotRect.Min.X-12, y, plotRect.Min.X-2, y, tickColor)
	}
}

func drawXAxisLabels(img *image.RGBA, renderer *textRenderer, plotRect image.Rectangle, labels [3]string, mutedColor color.Color) {
	renderer.draw(img, plotRect.Min.X, plotRect.Max.Y+26, labels[0], 14, mutedColor)
	midX := plotRect.Min.X + plotRect.Dx()/2 - renderer.width(labels[1], 14)/2
	renderer.draw(img, midX, plotRect.Max.Y+26, labels[1], 14, mutedColor)
	rightX := plotRect.Max.X - renderer.width(labels[2], 14)
	renderer.draw(img, rightX, plotRect.Max.Y+26, labels[2], 14, mutedColor)
}

func drawLegendBlock(img *image.RGBA, renderer *textRenderer, x, y int, lineColor, textColor, mutedColor color.Color, label string) {
	drawRoundedRect(img, image.Rect(x, y, x+160, y+42), 10, color.RGBA{10, 14, 22, 255})
	fillRect(img, x+12, y+12, 16, 16, lineColor)
	renderer.draw(img, x+40, y+20, label, 16, textColor)
	renderer.draw(img, x+40, y+36, "Series", 14, mutedColor)
}

func timeAxisLabels(periodLabel string) [3]string {
	switch periodLabel {
	case "weekly":
		return [3]string{"7D AGO", "3.5D", "NOW"}
	default:
		return [3]string{"00:00", "12:00", "NOW"}
	}
}

func seriesBounds(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}

	minVal := values[0]
	maxVal := values[0]
	for _, value := range values[1:] {
		if value < minVal {
			minVal = value
		}
		if value > maxVal {
			maxVal = value
		}
	}
	return minVal, maxVal
}

func drawRoundedRect(img *image.RGBA, rect image.Rectangle, radius int, col color.Color) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if insideRoundedRect(x, y, rect, radius) {
				img.Set(x, y, col)
			}
		}
	}
}

func drawRoundedBorder(img *image.RGBA, rect image.Rectangle, radius int, col color.Color) {
	outer := rect
	inner := image.Rect(rect.Min.X+1, rect.Min.Y+1, rect.Max.X-1, rect.Max.Y-1)
	for y := outer.Min.Y; y < outer.Max.Y; y++ {
		for x := outer.Min.X; x < outer.Max.X; x++ {
			if insideRoundedRect(x, y, outer, radius) && !insideRoundedRect(x, y, inner, max(radius-1, 0)) {
				img.Set(x, y, col)
			}
		}
	}
}

func insideRoundedRect(x, y int, rect image.Rectangle, radius int) bool {
	if radius <= 0 {
		return image.Pt(x, y).In(rect)
	}
	if x < rect.Min.X || x >= rect.Max.X || y < rect.Min.Y || y >= rect.Max.Y {
		return false
	}

	left := rect.Min.X + radius
	right := rect.Max.X - radius - 1
	top := rect.Min.Y + radius
	bottom := rect.Max.Y - radius - 1

	if (x >= left && x <= right) || (y >= top && y <= bottom) {
		return true
	}

	var cx, cy int
	switch {
	case x < left && y < top:
		cx, cy = left, top
	case x > right && y < top:
		cx, cy = right, top
	case x < left && y > bottom:
		cx, cy = left, bottom
	default:
		cx, cy = right, bottom
	}

	dx := x - cx
	dy := y - cy
	return dx*dx+dy*dy <= radius*radius
}

func drawFrame(img *image.RGBA, rect image.Rectangle, col color.Color) {
	drawLine(img, rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y, col)
	drawLine(img, rect.Min.X, rect.Max.Y-1, rect.Max.X, rect.Max.Y-1, col)
	drawLine(img, rect.Min.X, rect.Min.Y, rect.Min.X, rect.Max.Y, col)
	drawLine(img, rect.Max.X-1, rect.Min.Y, rect.Max.X-1, rect.Max.Y, col)
}

func fillRect(img *image.RGBA, x, y, w, h int, col color.Color) {
	r := image.Rect(x, y, x+w, y+h).Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	draw.Draw(img, r, &image.Uniform{col}, image.Point{}, draw.Src)
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, col color.Color) {
	dx := int(math.Abs(float64(x1 - x0)))
	dy := -int(math.Abs(float64(y1 - y0)))
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	errVal := dx + dy

	for {
		if image.Pt(x0, y0).In(img.Bounds()) {
			img.Set(x0, y0, col)
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * errVal
		if e2 >= dy {
			errVal += dy
			x0 += sx
		}
		if e2 <= dx {
			errVal += dx
			y0 += sy
		}
	}
}

func drawLineThick(img *image.RGBA, x0, y0, x1, y1, thickness int, col color.Color) {
	if thickness <= 1 {
		drawLine(img, x0, y0, x1, y1, col)
		return
	}
	radius := thickness / 2
	for dx := -radius; dx <= radius; dx++ {
		for dy := -radius; dy <= radius; dy++ {
			drawLine(img, x0+dx, y0+dy, x1+dx, y1+dy, col)
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
