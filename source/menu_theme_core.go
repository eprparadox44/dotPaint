package main

import "unicode"

func menuMnemonic(text string) rune {
	runes := []rune(text)
	for i := 0; i+1 < len(runes); i++ {
		if runes[i] != '&' {
			continue
		}
		if runes[i+1] == '&' {
			i++
			continue
		}
		return unicode.ToUpper(runes[i+1])
	}
	return 0
}
