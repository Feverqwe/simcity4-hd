package sc4

import (
	"image"
	"math"
)

const lanczosPrecision = 22

type lanczosKernel struct {
	start   int
	weights []int
}

// Pillow uses a separable three-lobe Lanczos filter, normalized at the image
// edges and rounded to 22-bit fixed-point coefficients before sampling.
func lanczosKernels(input, output int) []lanczosKernel {
	kernels := make([]lanczosKernel, output)
	ratio := float64(input) / float64(output)
	filterScale := max(1, ratio)
	support := 3 * filterScale
	for i := range kernels {
		center := (float64(i) + 0.5) * ratio
		start := max(0, int(center-support+0.5))
		end := min(input, int(center+support+0.5))
		coefficients := make([]float64, end-start)
		var sum float64
		for j := range coefficients {
			x := (float64(start+j) - center + 0.5) / filterScale
			coefficients[j] = sinc(x) * sinc(x/3)
			sum += coefficients[j]
		}
		weights := make([]int, len(coefficients))
		for j, coefficient := range coefficients {
			weights[j] = int(math.Round(coefficient / sum * (1 << lanczosPrecision)))
		}
		kernels[i] = lanczosKernel{start, weights}
	}
	return kernels
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	x *= math.Pi
	return math.Sin(x) / x
}

func sampleLanczos(kernel lanczosKernel, at func(int) byte) byte {
	value := 1 << (lanczosPrecision - 1)
	for i, weight := range kernel.weights {
		value += weight * int(at(kernel.start+i))
	}
	return byte(min(255, max(0, value>>lanczosPrecision)))
}

func restoreLanczosAlpha(source, result *image.NRGBA) {
	xKernels := lanczosKernels(source.Rect.Dx(), result.Rect.Dx())
	yKernels := lanczosKernels(source.Rect.Dy(), result.Rect.Dy())
	stride := len(xKernels)
	horizontal := make([]byte, stride*source.Rect.Dy())
	for y := range source.Rect.Dy() {
		for x, kernel := range xKernels {
			horizontal[y*stride+x] = sampleLanczos(kernel, func(i int) byte {
				return source.NRGBAAt(i, y).A
			})
		}
	}
	for y, kernel := range yKernels {
		for x := range xKernels {
			result.Pix[result.PixOffset(x, y)+3] = sampleLanczos(kernel, func(i int) byte {
				return horizontal[i*stride+x]
			})
		}
	}
}

func halfLanczosRGB(source *image.NRGBA) *image.NRGBA {
	width, height := source.Rect.Dx(), source.Rect.Dy()
	xKernels := lanczosKernels(width, width/2)
	yKernels := lanczosKernels(height, height/2)
	horizontal := image.NewNRGBA(image.Rect(0, 0, width/2, height))
	for y := range height {
		for x, kernel := range xKernels {
			target := horizontal.Pix[horizontal.PixOffset(x, y):]
			for channel := range 3 {
				target[channel] = sampleLanczos(kernel, func(i int) byte {
					return source.Pix[source.PixOffset(i, y)+channel]
				})
			}
		}
	}
	result := image.NewNRGBA(image.Rect(0, 0, width/2, height/2))
	for y, kernel := range yKernels {
		for x := range width / 2 {
			target := result.Pix[result.PixOffset(x, y):]
			for channel := range 3 {
				target[channel] = sampleLanczos(kernel, func(i int) byte {
					return horizontal.Pix[horizontal.PixOffset(x, i)+channel]
				})
			}
			target[3] = 255
		}
	}
	return result
}
