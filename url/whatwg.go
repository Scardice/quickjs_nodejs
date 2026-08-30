package url

import (
	"fmt"
	"strings"

	whatwgurl "github.com/nlnwa/whatwg-url/url"
)

type parsedURL struct {
	value *whatwgurl.Url
}

var laxURLParser = whatwgurl.NewParser(
	whatwgurl.WithLaxHostParsing(),
	whatwgurl.WithPreParseHostFunc(func(url *whatwgurl.Url, host string) string {
		if isSpecialScheme(url.Scheme()) {
			return lowercaseASCII(host)
		}
		return host
	}),
)

func lowercaseASCII(input string) string {
	for index := 0; index < len(input); index++ {
		if input[index] >= 'A' && input[index] <= 'Z' {
			var builder strings.Builder
			builder.Grow(len(input))
			builder.WriteString(input[:index])
			for ; index < len(input); index++ {
				char := input[index]
				if char >= 'A' && char <= 'Z' {
					char += 'a' - 'A'
				}
				builder.WriteByte(char)
			}
			return builder.String()
		}
	}
	return input
}

func hasInvalidPunycodeHost(input string) bool {
	schemeEnd := strings.IndexByte(input, ':')
	if schemeEnd < 0 || !strings.HasPrefix(input[schemeEnd:], "://") {
		return false
	}
	authorityStart := schemeEnd + 3
	authorityEnd := len(input)
	for index := authorityStart; index < len(input); index++ {
		if input[index] == '/' || input[index] == '?' || input[index] == '#' {
			authorityEnd = index
			break
		}
	}
	host := input[authorityStart:authorityEnd]
	if userInfoEnd := strings.LastIndexByte(host, '@'); userInfoEnd >= 0 {
		host = host[userInfoEnd+1:]
	}
	if strings.HasPrefix(host, "[") {
		return false
	}
	if portStart := strings.LastIndexByte(host, ':'); portStart >= 0 {
		host = host[:portStart]
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) >= 4 && strings.EqualFold(label[:4], "xn--") {
			return true
		}
	}
	return false
}

func normalizeURLInput(input string) string {
	start, end := 0, len(input)
	for start < end && input[start] <= 0x20 {
		start++
	}
	for end > start && input[end-1] <= 0x20 {
		end--
	}

	trimmed := input[start:end]
	for index := 0; index < len(trimmed); index++ {
		char := trimmed[index]
		if char <= 0x1F || char == 0x7F {
			var builder strings.Builder
			builder.Grow(len(trimmed) + 2)
			for _, char := range []byte(trimmed) {
				switch char {
				case '\t', '\n', '\r':
					continue
				default:
					if char <= 0x1F || char == 0x7F {
						builder.WriteByte('%')
						builder.WriteByte(upperHex[char>>4])
						builder.WriteByte(upperHex[char&0x0F])
						continue
					}
					builder.WriteByte(char)
				}
			}
			return encodePathCarets(encodeOpaqueTrailingSpace(builder.String()))
		}
	}
	return encodePathCarets(encodeOpaqueTrailingSpace(trimmed))
}

func encodeOpaqueTrailingSpace(input string) string {
	schemeEnd := strings.IndexByte(input, ':')
	if schemeEnd < 0 || strings.HasPrefix(input[schemeEnd:], "://") {
		return input
	}
	pathEnd := len(input)
	for index := schemeEnd + 1; index < len(input); index++ {
		if input[index] == '?' || input[index] == '#' {
			pathEnd = index
			break
		}
	}
	if pathEnd == schemeEnd+1 || input[pathEnd-1] != ' ' {
		return input
	}
	return input[:pathEnd-1] + "%20" + input[pathEnd:]
}

func encodePathCarets(input string) string {
	schemeEnd := strings.IndexByte(input, ':')
	if schemeEnd < 0 || !strings.HasPrefix(input[schemeEnd:], "://") {
		return input
	}
	authorityEnd := len(input)
	for index := schemeEnd + 3; index < len(input); index++ {
		if input[index] == '/' || input[index] == '?' || input[index] == '#' {
			authorityEnd = index
			break
		}
	}
	pathEnd := len(input)
	for index := authorityEnd; index < len(input); index++ {
		if input[index] == '?' || input[index] == '#' {
			pathEnd = index
			break
		}
	}
	path := input[authorityEnd:pathEnd]
	if !strings.ContainsRune(path, '^') {
		return input
	}
	return input[:authorityEnd] + strings.ReplaceAll(path, "^", "%5E") + input[pathEnd:]
}

func hasForbiddenHostControl(input string) bool {
	schemeEnd := strings.IndexByte(input, ':')
	if schemeEnd < 0 || !strings.HasPrefix(input[schemeEnd:], "://") {
		return false
	}
	authorityStart := schemeEnd + 3
	authorityEnd := len(input)
	for index := authorityStart; index < len(input); index++ {
		switch input[index] {
		case '/', '?', '#':
			authorityEnd = index
			index = len(input)
		}
	}
	hostStart := authorityStart
	if userInfoEnd := strings.LastIndexByte(input[authorityStart:authorityEnd], '@'); userInfoEnd >= 0 {
		hostStart += userInfoEnd + 1
	}
	for _, char := range []byte(input[hostStart:authorityEnd]) {
		if char == 0 {
			return true
		}
	}
	return false
}

func parseURL(input string, base *parsedURL, requireAbsolute bool) (*parsedURL, error) {
	var (
		value *whatwgurl.Url
		err   error
	)
	if hasForbiddenHostControl(input) {
		return nil, fmt.Errorf("URL host contains a control character")
	}
	input = normalizeURLInput(input)
	if base == nil {
		value, err = whatwgurl.Parse(input)
		if err != nil && hasInvalidPunycodeHost(input) {
			value, err = laxURLParser.Parse(input)
		}
	} else {
		value, err = base.value.Parse(input)
	}
	if err != nil {
		return nil, err
	}
	if requireAbsolute && value.Scheme() == "" {
		return nil, fmt.Errorf("URL has no scheme")
	}
	return &parsedURL{value: value}, nil
}

func (u *parsedURL) String() string {
	return u.value.Href(false)
}

func (u *parsedURL) scheme() string {
	return u.value.Scheme()
}

func (u *parsedURL) protocol() string {
	return u.value.Protocol()
}

func (u *parsedURL) host() string {
	return u.value.Host()
}

func (u *parsedURL) hostname() string {
	return u.value.Hostname()
}

func (u *parsedURL) port() string {
	return u.value.Port()
}

func (u *parsedURL) username() string {
	return u.value.Username()
}

func (u *parsedURL) password() string {
	return u.value.Password()
}

func (u *parsedURL) pathname() string {
	return u.value.Pathname()
}

func (u *parsedURL) search() string {
	return u.value.Search()
}

func (u *parsedURL) query() string {
	return u.value.Query()
}

func (u *parsedURL) hash() string {
	return u.value.Hash()
}

func (u *parsedURL) origin() string {
	if u.scheme() == "blob" {
		nested, err := parseURL(u.pathname(), nil, true)
		if err == nil && (nested.scheme() == "http" || nested.scheme() == "https") {
			return nested.origin()
		}
		return "null"
	}
	if !isNetworkScheme(u.scheme()) || u.host() == "" {
		return "null"
	}
	return u.scheme() + "://" + u.host()
}

func (u *parsedURL) setHash(value string) {
	u.value.SetHash(value)
}

func (u *parsedURL) setHost(value string) {
	u.value.SetHost(value)
}

func (u *parsedURL) setHostname(value string) {
	u.value.SetHostname(value)
}

func (u *parsedURL) setPassword(value string) {
	u.value.SetPassword(value)
}

func (u *parsedURL) setPathname(value string) {
	u.value.SetPathname(value)
}

func (u *parsedURL) setPort(value string) {
	u.value.SetPort(value)
}

func (u *parsedURL) setProtocol(value string) {
	u.value.SetProtocol(value)
}

func (u *parsedURL) setSearch(value string) {
	u.value.SetSearch(value)
}

func (u *parsedURL) setUsername(value string) {
	u.value.SetUsername(value)
}
