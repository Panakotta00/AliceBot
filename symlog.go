package main

import (
	"fmt"
	"math"

	"gonum.org/v1/plot"
)

// SymlogScale bildet Werte ähnlich zu Matplotlibs symlog ab.
// - |x| <= LinThresh: linear
// - |x| >  LinThresh: logarithmisch
// Base: Log-Basis (z. B. 10); bei 0 oder 1 wird natürliche Logarithmus-Basis e verwendet.
// LinScale skaliert die "Höhe" des Übergangsbereichs in Anzeigekoordinaten.
type SymlogScale struct {
	LinThresh float64 // typ. > 0 (z. B. 1, 0.1, ...)
	LinScale  float64 // typ. >= 1 (wie stark die lineare Zone "gestreckt" wird)
	Base      float64 // z. B. 10; 0 oder 1 => e
}

func (s SymlogScale) Normalize(min, max, x float64) float64 {
	if min == max {
		return 0.5
	}
	tmin := s.symlog(min)
	tmax := s.symlog(max)
	tx := s.symlog(x)
	return (tx - tmin) / (tmax - tmin)
}

// symlog-Transformation ähnlich Matplotlib:
// y = sign(x) * ( lin_part ) für |x| <= linthresh
// y = sign(x) * ( linthresh*LinScale + log_base(|x|/linthresh) ) für |x| > linthresh
// Die additive Konstante linthresh*LinScale sorgt dafür, dass außerhalb
// der linearen Zone die y-Werte nahtlos anschließen.
func (s SymlogScale) symlog(x float64) float64 {
	if x == 0 {
		return 0
	}
	linth := s.LinThresh
	if linth <= 0 {
		linth = 1 // Fallback, um Division/Log-Fehler zu vermeiden
	}
	linsc := s.LinScale
	if linsc <= 0 {
		linsc = 1
	}
	base := s.Base
	if base == 0 || base == 1 {
		base = math.E
	}

	ax := math.Abs(x)
	sign := 1.0
	if x < 0 {
		sign = -1.0
	}

	if ax <= linth {
		return sign * (ax * linsc)
	}
	// Logarithmischer Anteil beginnt nach der linearen Zone in Anzeigekoordinaten
	return sign * (linth*linsc + logBase(ax/linth, base))
}

func logBase(v, base float64) float64 {
	if base == math.E {
		return math.Log(v)
	}
	return math.Log(v) / math.Log(base)
}

// Optional: Ticker, der Ticks in linearer Zone um 0 und log-Zonen erzeugt.
type SymlogTicks struct {
	LinThresh float64
	Base      float64 // z. B. 10; 0 oder 1 => e
	Decades   []int   // welche Zehnerpotenzen (oder Basis^k) abbilden (z. B. -6..6)
	Minor     bool    // ob Minor-Ticks erzeugt werden sollen
}

func (t SymlogTicks) Ticks(min, max float64) []plot.Tick {
	var ticks []plot.Tick
	linth := t.LinThresh
	if linth <= 0 {
		linth = 1
	}
	base := t.Base
	if base == 0 || base == 1 {
		base = math.E
	}
	decades := t.Decades
	if len(decades) == 0 {
		// Standard: -6..6
		for k := -6; k <= 6; k++ {
			decades = append(decades, k)
		}
	}

	// 1) Lineare Zone um 0
	addTickIfIn := func(v float64, label string) {
		if v >= min && v <= max {
			ticks = append(ticks, plot.Tick{Value: v, Label: label})
		}
	}
	addMinorIfIn := func(v float64) {
		if v >= min && v <= max {
			ticks = append(ticks, plot.Tick{Value: v, Label: ""})
		}
	}

	// Hauptticks im linearen Bereich: -linth, 0, +linth
	addTickIfIn(-linth, fmt.Sprintf("-%g", linth))
	addTickIfIn(0, "0")
	addTickIfIn(+linth, fmt.Sprintf("%g", linth))

	// Optional ein paar Minor-Ticks in der linearen Zone
	if t.Minor {
		steps := 4
		step := linth / float64(steps)
		for i := 1; i < steps; i++ {
			v := step * float64(i)
			addMinorIfIn(v)
			addMinorIfIn(-v)
		}
	}

	// 2) Log-Zonen: negative und positive Seite
	// Wir erzeugen Major-Ticks bei base^k und optional Minor dazwischen.
	genLogSide := func(sign float64) {
		for _, k := range decades {
			mag := math.Pow(base, float64(k))
			v := sign * linth * mag
			if v >= min && v <= max {
				lbl := fmt.Sprintf("%g", v)
				ticks = append(ticks, plot.Tick{Value: v, Label: lbl})
			}
			if t.Minor {
				// Minor-Ticks zwischen den Dekaden: 2..9 (für Base=10)
				if math.Abs(base-10) < 1e-9 {
					for m := 2; m < 10; m++ {
						vm := sign * linth * float64(m) * math.Pow(10, float64(k))
						addMinorIfIn(vm)
					}
				} else {
					// Für andere Basen: 2..(int(base)-1)
					maxm := int(base)
					if maxm > 2 && maxm < 20 { // nicht übertreiben
						for m := 2; m < maxm; m++ {
							vm := sign * linth * float64(m) * math.Pow(base, float64(k-1))
							addMinorIfIn(vm)
						}
					}
				}
			}
		}
	}
	// Positive Seite
	genLogSide(+1)
	// Negative Seite
	genLogSide(-1)

	// Sortierung nach x-Wert (einige Plot-Renderer erwarten monotone Tick-Reihenfolge)
	if len(ticks) > 1 {
		// simple insertion sort (kleine Menge)
		for i := 1; i < len(ticks); i++ {
			j := i
			for j > 0 && ticks[j-1].Value > ticks[j].Value {
				ticks[j-1], ticks[j] = ticks[j], ticks[j-1]
				j--
			}
		}
	}

	return ticks
}
