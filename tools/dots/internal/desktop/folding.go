package desktop

import (
	"strings"
	"unicode"
)

// Full case folding expands these characters into several runes. SimpleFold
// handles all remaining equivalence classes, including final sigma and Kelvin.
// Do not normalize accents: Python str.casefold does not perform NFC or NFKC.
var foldExpansions = map[rune]string{
	'ß': "ss", 'İ': "i\u0307", 'ŉ': "ʼn", 'ǰ': "j\u030c",
	'ΐ': "ι\u0308\u0301", 'ΰ': "υ\u0308\u0301", 'և': "եւ",
	'ẖ': "h\u0331", 'ẗ': "t\u0308", 'ẘ': "w\u030a", 'ẙ': "y\u030a", 'ẚ': "aʾ", 'ẞ': "ss",
	'\u1f50': "υ\u0313", '\u1f52': "υ\u0313\u0300", '\u1f54': "υ\u0313\u0301", '\u1f56': "υ\u0313\u0342",
	'\u1fb2': "ὰι", '\u1fb3': "αι", '\u1fb4': "άι", '\u1fb6': "α\u0342", '\u1fb7': "α\u0342ι", '\u1fbc': "αι",
	'\u1fc2': "ὴι", '\u1fc3': "ηι", '\u1fc4': "ήι", '\u1fc6': "η\u0342", '\u1fc7': "η\u0342ι", '\u1fcc': "ηι",
	'\u1fd2': "ι\u0308\u0300", '\u1fd3': "ι\u0308\u0301", '\u1fd6': "ι\u0342", '\u1fd7': "ι\u0308\u0342",
	'\u1fe2': "υ\u0308\u0300", '\u1fe3': "υ\u0308\u0301", '\u1fe4': "ρ\u0313", '\u1fe6': "υ\u0342", '\u1fe7': "υ\u0308\u0342",
	'\u1ff2': "ὼι", '\u1ff3': "ωι", '\u1ff4': "ώι", '\u1ff6': "ω\u0342", '\u1ff7': "ω\u0342ι", '\u1ffc': "ωι",
	'\ufb00': "ff", '\ufb01': "fi", '\ufb02': "fl", '\ufb03': "ffi", '\ufb04': "ffl", '\ufb05': "st", '\ufb06': "st",
	'\ufb13': "մն", '\ufb14': "մե", '\ufb15': "մի", '\ufb16': "վն", '\ufb17': "մխ",
}

func foldKey(s string) string {
	var b strings.Builder
	simple := func(r rune) {
		lowest := r
		for n := unicode.SimpleFold(r); n != r; n = unicode.SimpleFold(n) {
			if n < lowest {
				lowest = n
			}
		}
		b.WriteRune(lowest)
	}
	for _, r := range s {
		expansion := foldExpansions[r]
		if r >= 0x1f80 && r <= 0x1faf {
			base := rune(0x1f00)
			if r >= 0x1fa0 {
				base = 0x1f60
			} else if r >= 0x1f90 {
				base = 0x1f20
			}
			expansion = string([]rune{base + (r & 7), 'ι'})
		}
		if expansion != "" {
			for _, r := range expansion {
				simple(r)
			}
		} else {
			simple(r)
		}
	}
	return b.String()
}
