package hub

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// ValidateProxy deliberately rejects values the upstream could silently ignore.
func ValidateProxy(raw string) error {
	u, err := url.Parse(raw)
	bad := errors.New("Прокси: нужен URL http(s):// или socks5(h):// с хостом и портом, без пути, query и fragment")
	if err != nil || u == nil || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return bad
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return bad
	}
	if u.Hostname() == "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return bad
	}
	if strings.ContainsAny(u.Hostname(), "\\%") {
		return bad
	}
	if strings.Contains(u.Hostname(), ":") && net.ParseIP(u.Hostname()) == nil {
		return bad
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return bad
	}
	if u.User != nil {
		if p, _ := u.User.Password(); p == "***" {
			return errors.New("Введите настоящий пароль прокси; маска не является паролем")
		}
	}
	return nil
}

func maskProxy(raw string) string {
	if raw == "" || raw == "direct" || raw == "none" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "адрес скрыт"
	}
	if u.User != nil {
		if _, ok := u.User.Password(); ok {
			u.User = url.UserPassword(u.User.Username(), "***")
		}
	}
	return u.String()
}

func proxyValue(mode string, value *string, old route) (string, error) {
	switch mode {
	case "inherit":
		return "", nil
	case "direct":
		return "direct", nil
	case "custom":
		if value == nil {
			if old.Confirmed && old.Mode == "custom" {
				return old.URL, nil
			}
			return "", errors.New("Укажите адрес собственного прокси")
		}
		if err := ValidateProxy(*value); err != nil {
			return "", err
		}
		return *value, nil
	default:
		return "", errors.New("Неизвестный режим прокси")
	}
}
