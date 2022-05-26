package util

import (
	"bytes"
	"time"

	"github.com/valyala/fasthttp"
)

// Strftime is a strftime formatter.
type Strftime struct {
	fns []strftimeFunc
}

// NewStrftime creates a new Strftime.
func NewStrftime(format string) *Strftime {
	return NewStrftimeBytes([]byte(format))
}

// NewStrftimeBytes creates a new Strftime.
func NewStrftimeBytes(format []byte) *Strftime {
	p := []byte{'%'}
	var fns []strftimeFunc
	off := 0
	for {
		i := bytes.Index(format[off:], p)
		if i == -1 {
			fns = append(fns, strftimeBytes(format[off:]))
			break
		}
		if i > 0 {
			fns = append(fns, strftimeBytes(format[off:off+i]))
		}
		j := off + i + 1
		if j == len(format) {
			break
		}
		if fn, ok := strftimeFuncs[format[j]]; ok {
			fns = append(fns, fn)
		} else {
			fns = append(fns, strftimeBytes(format[j:j+1]))
		}
		off = j + 1
	}
	return &Strftime{fns: fns}
}

// Format formats the specified time.
func (s *Strftime) Format(t time.Time) string {
	return string(s.FormatBytes(t))
}

// FormatBytes formats the specified time.
func (s *Strftime) FormatBytes(t time.Time) []byte {
	var dst []byte
	return s.AppendBytes(dst, t)
}

// AppendBytes appends format result to dst and returns the extended buffer.
func (s *Strftime) AppendBytes(dst []byte, t time.Time) []byte {
	for _, fn := range s.fns {
		dst = fn(dst, t)
	}
	return dst
}

type strftimeFunc func(dst []byte, t time.Time) []byte

func strftimeBytes(b []byte) strftimeFunc {
	return func(dst []byte, t time.Time) []byte {
		return append(dst, b...)
	}
}

func strftimeGo(layout string) strftimeFunc {
	return func(dst []byte, t time.Time) []byte {
		return t.AppendFormat(dst, layout)
	}
}

var (
	strftimeC = func(dst []byte, t time.Time) []byte {
		return AppendZeroPaddingUint(dst, t.Year()/100, 2)
	}
	strftimeLd = func(dst []byte, t time.Time) []byte {
		return AppendZeroPaddingUint(dst, t.Day(), 2)
	}
	strftimeD = func(dst []byte, t time.Time) []byte {
		dst = strftimeLm(dst, t)
		dst = append(dst, '/')
		dst = strftimeLd(dst, t)
		dst = append(dst, '/')
		dst = strftimeLy(dst, t)
		return dst
	}
	strftimeLe = func(dst []byte, t time.Time) []byte {
		return AppendSpacePaddingUint(dst, t.Day(), 2)
	}
	strftimeF = func(dst []byte, t time.Time) []byte {
		dst = strftimeY(dst, t)
		dst = append(dst, '-')
		dst = strftimeLm(dst, t)
		dst = append(dst, '-')
		dst = strftimeLd(dst, t)
		return dst
	}
	strftimeH = func(dst []byte, t time.Time) []byte {
		return AppendZeroPaddingUint(dst, t.Hour(), 2)
	}
	strftimeI = func(dst []byte, t time.Time) []byte {
		h := t.Hour()
		if h > 12 {
			h -= 12
		}
		return AppendZeroPaddingUint(dst, h, 2)
	}
	strftimeLj = func(dst []byte, t time.Time) []byte {
		return AppendZeroPaddingUint(dst, t.YearDay(), 3)
	}
	strftimeLk = func(dst []byte, t time.Time) []byte {
		return AppendSpacePaddingUint(dst, t.Hour(), 2)
	}
	strftimeLl = func(dst []byte, t time.Time) []byte {
		h := t.Hour()
		if h > 12 {
			h -= 12
		}
		return AppendSpacePaddingUint(dst, h, 2)
	}
	strftimeLu = func(dst []byte, t time.Time) []byte {
		w := int(t.Weekday())
		if w == 0 {
			w = 7
		}
		return fasthttp.AppendUint(dst, w)
	}
	strftimeLm = func(dst []byte, t time.Time) []byte {
		return AppendZeroPaddingUint(dst, int(t.Month()), 2)
	}
	strftimeM = func(dst []byte, t time.Time) []byte {
		return AppendZeroPaddingUint(dst, t.Minute(), 2)
	}
	strftimeLr = func(dst []byte, t time.Time) []byte {
		dst = strftimeI(dst, t)
		dst = append(dst, ':')
		dst = strftimeM(dst, t)
		dst = append(dst, ':')
		dst = strftimeS(dst, t)
		dst = append(dst, ' ')
		dst = t.AppendFormat(dst, "pm")
		return dst
	}
	strftimeR = func(dst []byte, t time.Time) []byte {
		dst = strftimeH(dst, t)
		dst = append(dst, ':')
		dst = strftimeM(dst, t)
		return dst
	}
	strftimeLs = func(dst []byte, t time.Time) []byte {
		return fasthttp.AppendUint(dst, int(t.Unix()))
	}
	strftimeS = func(dst []byte, t time.Time) []byte {
		return AppendZeroPaddingUint(dst, t.Second(), 2)
	}
	strftimeT = func(dst []byte, t time.Time) []byte {
		dst = strftimeH(dst, t)
		dst = append(dst, ':')
		dst = strftimeM(dst, t)
		dst = append(dst, ':')
		dst = strftimeS(dst, t)
		return dst
	}
	strftimeU = func(dst []byte, t time.Time) []byte {
		d := int(t.YearDay()) - int(t.Weekday())
		if d >= 0 {
			return AppendZeroPaddingUint(dst, int(d/7)+1, 2)
		}
		lt := t.AddDate(0, 0, d)
		ld := int(lt.YearDay())
		return AppendZeroPaddingUint(dst, int(ld/7)+1, 2)
	}
	strftimeV = func(dst []byte, t time.Time) []byte {
		_, w := t.ISOWeek()
		return AppendZeroPaddingUint(dst, w, 2)
	}
	strftimeLw = func(dst []byte, t time.Time) []byte {
		return fasthttp.AppendUint(dst, int(t.Weekday()))
	}
	strftimeW = func(dst []byte, t time.Time) []byte {
		w := int(t.Weekday()) - 1
		if w == -1 {
			w = 6
		}
		d := int(t.YearDay()) - w
		if d >= 0 {
			return AppendZeroPaddingUint(dst, int(d/7)+1, 2)
		}
		lt := t.AddDate(0, 0, d)
		ld := int(lt.YearDay())
		return AppendZeroPaddingUint(dst, int(ld/7)+1, 2)
	}
	strftimeLy = func(dst []byte, t time.Time) []byte {
		return AppendZeroPaddingUint(dst, t.Year()%100, 2)
	}
	strftimeY = func(dst []byte, t time.Time) []byte {
		return AppendZeroPaddingUint(dst, t.Year(), 4)
	}
	strftimeLz = func(dst []byte, t time.Time) []byte {
		_, zo := t.Zone()
		if zo < 0 {
			dst = append(dst, '-')
			zo = -zo
		} else {
			dst = append(dst, '+')
		}
		dst = AppendZeroPaddingUint(dst, zo/(60*60), 2)
		dst = append(dst, '0', '0')
		return dst
	}
)

var strftimeFuncs = map[byte]strftimeFunc{
	'a': strftimeGo("Mon"),
	'A': strftimeGo("Monday"),
	'b': strftimeGo("Jan"),
	'B': strftimeGo("January"),
	'c': strftimeGo("Mon Jan _2 15:04:05 2006"),
	'C': strftimeC,
	'd': strftimeLd,
	'D': strftimeD,
	'e': strftimeLe,
	'F': strftimeF,
	'g': strftimeLy,
	'G': strftimeY,
	'h': strftimeGo("Jan"), // NOTE: alias for b
	'H': strftimeH,
	'I': strftimeI,
	'j': strftimeLj,
	'k': strftimeLk,
	'l': strftimeLl,
	'm': strftimeLm,
	'M': strftimeM,
	'n': strftimeBytes([]byte("\n")),
	'p': strftimeGo("pm"),
	'P': strftimeGo("PM"),
	'r': strftimeLr,
	'R': strftimeR,
	's': strftimeLs,
	'S': strftimeS,
	't': strftimeBytes([]byte("\t")),
	'T': strftimeT,
	'u': strftimeLu,
	'U': strftimeU,
	'V': strftimeV,
	'w': strftimeLw,
	'W': strftimeW,
	'x': strftimeGo("01/02/06"),
	'X': strftimeGo("15:04:05"),
	'y': strftimeLy,
	'Y': strftimeY,
	'z': strftimeLz,
	'Z': strftimeGo("MST"),
}
