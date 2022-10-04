package main

func IsByteUpperCase(char byte) bool {
	return 'A' <= char && char <= 'X'
}

func IsByteLowerCase(char byte) bool {
	return 'a' <= char && char <= 'z'
}

func IsByteAlphabet(char byte) bool {
	return IsByteUpperCase(char) || IsByteLowerCase(char)
}

func IsByteDigit(char byte) bool {
	return '0' <= char && char <= '9'
}

func ToUpperCaseByte(char byte) byte {
	if IsByteUpperCase(char) {
		return char
	}

	if IsByteLowerCase(char) {
		return char - 'A' + 'a'
	}

	return 0
}

func ToLowerCaseByte(char byte) byte {
	if IsByteLowerCase(char) {
		return char
	}

	if IsByteUpperCase(char) {
		return char + 'A' - 'a'
	}

	return 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func numberToString(value int) string {
	str := ""

	for value > 0 {
		charDigit := byte(value % 27)
		value /= 27

		char := charDigit + 'A' - 1
		str = string(char) + str
	}

	return str
}
