package amzkeys

import "strings"

const (
	TestAppID      = "2026240003167464"
	TestAppKey     = "8802afc63e8108109dbcbe4805950c131ed6ca7a"
	TestPrivateKey = "MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDKuHKxtidRBfEVi4K4pyqLunx7EtqifgiK6XhTjV725T8hLHrZ02F1zehH65tXmB2O7JXpcuYaALbq3UdgAyTCnLa/uPMEoHfJrYScL+zMO3eL/cmKcm3US/lY44GrutG0GRlrgIp3idFUw05D7ERUgJWRm+IdihStwMIyRxRgCE6yWfZVPNt9YAkv3CiJILjIJZ2U023n7bOiBlU7KOKcRYwoOKsZJ+CR93XcK2PT3dASUh/MU7T0i/HtMT5+3sNochVVeE5wb9oZR2Dp1320SqrVW8Cy+TfZF9f6XJTnDfLPl3U79ehs1zWF7lLlGtRaaTpi1Ls+/LTqvGYvxjvZAgMBAAECggEAaflKXARxQTXt9elciNM6tpjigiQ1D0T7ikLKmEMLJd5pxhnOjxillkPx7ccJCh2HNjQPml5qU6WT+et7aIG8MdBi6I7y27RXaqK+9DdJfuqHcDNXrHxtVdHxo7orC286OP/1/fDQcfUl7T28KF3WyqX9ioUHC5InhT2DR21JBXni+biqD0ZrEBY8YJmgs6tj70KPME/o0hvkwPzIKdwyz0XdQgLfXJ2gnDjUrkhpxl4ZHaFHzxphIVoLmZKeHijb4Td/knNIUJwMdPhFznj+tMhBNuFAqAtJRmuCAuc8pHeTjpenUCPq8J7po/hlAMs5fzQuG0akNF1Ibp5A+YepQQKBgQD7IutCk9wAc4jceyomXID+ui+1FOZDCawvsIPPXTouphWqArQChFE0i2oAOUV7aFDllliCvYEvnW93Q4EJC/KfjxxYW56vWEsCm8zzWDmjsMKf6hH/IBtNi2MF47YOu8mgYVI7kZk7gaQwW4AfUI+FzFwLIau+Z/Sih+/LqsqJpQKBgQDOpX5GqSJ9UUQG9vHINa38oKreSTx0aP7msLqObOOjlh5Vo486n4+0xO9t3y0abwS+wjJNZo9ysQYVykA3eb6p2ymtACpNI6YDsEEcIWRduMG2srgzJHyldMufcDf/Zr6ZGiseK0eFV3GXTUQUmzUt5dpkq56TP8/BPOE65+lLJQKBgQDb86ZnJkceYhIxQPIWrRgRgZI9H/PYLQWQsyGOoZFOwAnjYAC235qb0ariTUbMof2QR1B4GW+m+1Vf+FBwUvWJx/bEcGIYItV3kGs9ijzZX/vlwUVH1J/1F6p/wwN1/gTGodY68/doBdB+xfT9+DnrrqPC4BeiaTv6ieJ817YSPQKBgASvf/N+Nkf9Jbu6sbTGctF9myI7KuHA17bHXxOHqIm3B60Nblv37jw9Eui83LrytXrV0Gos3yfMl8S6t0YKvqX/UCyaCluBaWw//Nn0b+AoJkxMNR0DwMfHpC5TTxG9dKjoDP48IP0HBI5XtCl7c3M8+Py7X3cbRUyuYrUBOSr5AoGAUysypdeMw4MfAfNcr3d8/qDgONDZv8xuOahGo1MNcjzt+LR4ixA894wAV4k6O4JTUUJ3SKMlFxda9re7U6wUiuG9sjO3z2GgfPZm/pUpbadSEaMvOK9NQzYJA2XYvXiBo0EzZobn/50XaOcW/efwPtrt33eAxmuMFgLkA4CkgAA="
)

func IsTestHost(host string) bool {
	h := strings.ToLower(strings.TrimRight(strings.TrimSpace(host), "/"))
	if h == "" {
		return true
	}
	return strings.Contains(h, "testapi.amzkeys.com")
}

func Resolve(host, appID, appKey, privateKey string) (string, string, string, string) {
	host = strings.TrimRight(strings.TrimSpace(host), "/")
	if host == "" {
		host = DefaultHost
	}
	if IsTestHost(host) {
		return host, TestAppID, TestAppKey, TestPrivateKey
	}
	return host, strings.TrimSpace(appID), strings.TrimSpace(appKey), strings.TrimSpace(privateKey)
}
