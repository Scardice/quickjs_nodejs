package url

import "strings"

var tblEscapeURLQuery = [128]byte{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 1, 0, 0, 1, 1, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 0, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0,
}

var tblEscapeURLQueryParam = [128]byte{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 1, 1, 0,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0,
	0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 1,
	0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0,
}

const upperHex = "0123456789ABCDEF"

func escapeURLQuery(value string) string {
	return escapeURL(value, &tblEscapeURLQuery, false)
}

func escapeSearchParam(value string) string {
	return escapeURL(value, &tblEscapeURLQueryParam, true)
}

func escapeURL(value string, table *[128]byte, spaceToPlus bool) string {
	spaceCount, hexCount := 0, 0
	for i := 0; i < len(value); i++ {
		char := value[i]
		if char > 127 || table[char] == 0 {
			if char == ' ' && spaceToPlus {
				spaceCount++
			} else {
				hexCount++
			}
		}
	}
	if spaceCount == 0 && hexCount == 0 {
		return value
	}

	var builder strings.Builder
	builder.Grow(len(value) + 2*hexCount)
	for i := 0; i < len(value); i++ {
		char := value[i]
		switch {
		case char == ' ' && spaceToPlus:
			builder.WriteByte('+')
		case char > 127 || table[char] == 0:
			builder.WriteByte('%')
			builder.WriteByte(upperHex[char>>4])
			builder.WriteByte(upperHex[char&15])
		default:
			builder.WriteByte(char)
		}
	}
	return builder.String()
}

func isHex(char byte) bool {
	return char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F'
}

func unHex(char byte) byte {
	switch {
	case char >= '0' && char <= '9':
		return char - '0'
	case char >= 'a' && char <= 'f':
		return char - 'a' + 10
	case char >= 'A' && char <= 'F':
		return char - 'A' + 10
	default:
		return 0
	}
}

func unescapeSearchParam(value string) string {
	count := 0
	hasPlus := false
	for index := 0; index < len(value); {
		switch value[index] {
		case '%':
			if index+2 < len(value) && isHex(value[index+1]) && isHex(value[index+2]) {
				count++
				index += 3
				continue
			}
		case '+':
			hasPlus = true
		}
		index++
	}
	if count == 0 && !hasPlus {
		return value
	}

	var builder strings.Builder
	builder.Grow(len(value) - 2*count)
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '%':
			if index+2 < len(value) && isHex(value[index+1]) && isHex(value[index+2]) {
				builder.WriteByte(unHex(value[index+1])<<4 | unHex(value[index+2]))
				index += 2
			} else {
				builder.WriteByte('%')
			}
		case '+':
			builder.WriteByte(' ')
		default:
			builder.WriteByte(value[index])
		}
	}
	return builder.String()
}
