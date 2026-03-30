package service

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strings"
	"time"

	"github.com/horoshi10v/opi-thermald/internal/collector"
)

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

func renderSummaryChart(title string, temp, cpu, load []float64) ([]byte, error) {
	const (
		width        = 1600
		height       = 1080
		outerPadding = 48
		headerHeight = 110
		footerHeight = 52
		panelGap     = 28
	)

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	bg := color.RGBA{10, 14, 22, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	panelBg := color.RGBA{17, 24, 39, 255}
	grid := color.RGBA{51, 65, 85, 255}
	text := color.RGBA{226, 232, 240, 255}
	muted := color.RGBA{148, 163, 184, 255}
	tempColor := color.RGBA{248, 113, 113, 255}
	cpuColor := color.RGBA{96, 165, 250, 255}
	loadColor := color.RGBA{74, 222, 128, 255}

	fillRect(img, outerPadding, outerPadding, width-outerPadding*2, height-outerPadding*2, panelBg)
	drawText(img, outerPadding+24, outerPadding+20, title, 3, text)
	drawText(img, outerPadding+24, outerPadding+62, "DARK THEME SUMMARY", 2, muted)

	panelTop := outerPadding + headerHeight
	panelWidth := width - outerPadding*2 - 32
	panelHeight := (height - outerPadding*2 - headerHeight - footerHeight - panelGap*2 - 32) / 3
	panelX := outerPadding + 16

	drawSeriesPanel(
		img,
		image.Rect(panelX, panelTop, panelX+panelWidth, panelTop+panelHeight),
		"TEMP C",
		temp,
		tempColor,
		grid,
		text,
		muted,
	)
	drawSeriesPanel(
		img,
		image.Rect(panelX, panelTop+panelHeight+panelGap, panelX+panelWidth, panelTop+panelHeight*2+panelGap),
		"CPU PERCENT",
		cpu,
		cpuColor,
		grid,
		text,
		muted,
	)
	drawSeriesPanel(
		img,
		image.Rect(panelX, panelTop+panelHeight*2+panelGap*2, panelX+panelWidth, panelTop+panelHeight*3+panelGap*2),
		"LOAD1",
		load,
		loadColor,
		grid,
		text,
		muted,
	)

	drawText(img, outerPadding+24, height-outerPadding-28, "EXPORTS RAW DATA TO CSV AND SENDS PNG TO TELEGRAM", 2, muted)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func drawSeriesPanel(img *image.RGBA, rect image.Rectangle, label string, values []float64, lineColor, gridColor, textColor, mutedColor color.Color) {
	fillRect(img, rect.Min.X, rect.Min.Y, rect.Dx(), rect.Dy(), color.RGBA{15, 23, 42, 255})
	drawFrame(img, rect, gridColor)

	headerY := rect.Min.Y + 18
	drawText(img, rect.Min.X+20, headerY, label, 2, textColor)

	minVal, maxVal := seriesBounds(values)
	statsText := fmt.Sprintf("MIN %.1f MAX %.1f", minVal, maxVal)
	drawText(img, rect.Min.X+320, headerY, statsText, 2, mutedColor)

	scaleWidth := 88
	plotRect := image.Rect(rect.Min.X+scaleWidth, rect.Min.Y+62, rect.Max.X-24, rect.Max.Y-22)
	for i := 1; i < 4; i++ {
		y := plotRect.Min.Y + i*plotRect.Dy()/4
		drawLine(img, plotRect.Min.X, y, plotRect.Max.X, y, gridColor)
	}
	drawFrame(img, plotRect, gridColor)
	drawYAxisLabels(img, plotRect, minVal, maxVal, textColor, mutedColor)
	drawSeries(img, plotRect, values, lineColor)
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

func drawYAxisLabels(img *image.RGBA, plotRect image.Rectangle, minVal, maxVal float64, textColor, mutedColor color.Color) {
	steps := 4
	for i := 0; i <= steps; i++ {
		y := plotRect.Max.Y - 1 - i*(plotRect.Dy()-1)/steps
		value := minVal + (maxVal-minVal)*(float64(i)/float64(steps))
		label := fmt.Sprintf("%.1f", value)
		drawText(img, plotRect.Min.X-70, y-7, label, 2, mutedColor)
		drawLine(img, plotRect.Min.X-12, y, plotRect.Min.X-2, y, textColor)
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

func drawText(img *image.RGBA, x, y int, text string, scale int, col color.Color) {
	if scale <= 0 {
		scale = 1
	}

	cursor := x
	for _, r := range strings.ToUpper(text) {
		if r == ' ' {
			cursor += 3 * scale
			continue
		}
		pattern, ok := glyphPattern(r)
		if !ok {
			cursor += 3 * scale
			continue
		}
		for row, line := range pattern {
			for colIdx, ch := range line {
				if ch != '1' {
					continue
				}
				fillRect(img, cursor+colIdx*scale, y+row*scale, scale, scale, col)
			}
		}
		charWidth := len(pattern[0])
		cursor += (charWidth+2)*scale + scale
	}
}

func glyphPattern(r rune) ([]string, bool) {
	if r >= '0' && r <= '9' {
		return digitPatterns[r], true
	}
	pattern, ok := alphaPatterns[r]
	return pattern, ok
}

var digitPatterns = map[rune][]string{
	'0': {"111", "101", "101", "101", "111"},
	'1': {"010", "110", "010", "010", "111"},
	'2': {"111", "001", "111", "100", "111"},
	'3': {"111", "001", "111", "001", "111"},
	'4': {"101", "101", "111", "001", "001"},
	'5': {"111", "100", "111", "001", "111"},
	'6': {"111", "100", "111", "101", "111"},
	'7': {"111", "001", "001", "001", "001"},
	'8': {"111", "101", "111", "101", "111"},
	'9': {"111", "101", "111", "001", "111"},
}

var alphaPatterns = map[rune][]string{
	'A': {"010", "101", "111", "101", "101"},
	'B': {"110", "101", "110", "101", "110"},
	'C': {"011", "100", "100", "100", "011"},
	'D': {"110", "101", "101", "101", "110"},
	'E': {"111", "100", "110", "100", "111"},
	'G': {"011", "100", "101", "101", "011"},
	'H': {"101", "101", "111", "101", "101"},
	'I': {"111", "010", "010", "010", "111"},
	'L': {"100", "100", "100", "100", "111"},
	'M': {"101", "111", "111", "101", "101"},
	'N': {"101", "111", "111", "111", "101"},
	'O': {"111", "101", "101", "101", "111"},
	'P': {"111", "101", "111", "100", "100"},
	'R': {"110", "101", "110", "101", "101"},
	'S': {"011", "100", "010", "001", "110"},
	'T': {"111", "010", "010", "010", "010"},
	'U': {"101", "101", "101", "101", "111"},
	'W': {"101", "101", "111", "111", "101"},
	'Y': {"101", "101", "010", "010", "010"},
	'%': {"10001", "00010", "00100", "01000", "10001"},
	'=': {"000", "111", "000", "111", "000"},
	'-': {"000", "000", "111", "000", "000"},
	'.': {"0", "0", "0", "0", "1"},
	':': {"0", "1", "0", "1", "0"},
	'/': {"001", "001", "010", "100", "100"},
}
