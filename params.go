package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var paramSpecPattern = regexp.MustCompile(`^@([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$`)
var relativeTimePattern = regexp.MustCompile(`^(now|today)\s*(?:([+-])\s*(\d+(?:\.\d+)?)\s*([smhdw]))?$`)
var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}([ T]\d{2}:\d{2}(:\d{2})?)?`)
var intPattern = regexp.MustCompile(`^-?\d+$`)
var floatPattern = regexp.MustCompile(`^-?\d+\.\d+$`)

func inferParamType(value string) string {
	lower := strings.ToLower(value)
	switch {
	case len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'"):
		return "text"
	case relativeTimePattern.MatchString(lower), datePattern.MatchString(value):
		return "timestamp"
	case intPattern.MatchString(value):
		return "int"
	case floatPattern.MatchString(value):
		return "float"
	case lower == "true" || lower == "false":
		return "bool"
	default:
		return "text"
	}
}

func parseParamSpec(spec string) (Param, error) {
	match := paramSpecPattern.FindStringSubmatch(strings.TrimSpace(spec))
	if match == nil {
		return Param{}, fmt.Errorf("parámetro %q: usa @Nombre = valor", spec)
	}
	value := strings.TrimSpace(match[2])
	return Param{Name: match[1], Type: inferParamType(value), Value: value}, nil
}

func formatParam(p Param) string {
	return "@" + p.Name + " = " + p.Value
}

func unquote(value string) string {
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'")
	}
	return value
}

func resolveTime(value string, now time.Time) (time.Time, error) {
	value = unquote(strings.TrimSpace(value))
	match := relativeTimePattern.FindStringSubmatch(strings.ToLower(value))
	if match == nil {
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"} {
			if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("fecha %q no válida: usa 2026-10-20, 2026-10-20 13:00:00, now-24h o today-7d", value)
	}
	base := now
	if match[1] == "today" {
		base = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}
	if match[2] == "" {
		return base, nil
	}
	amount, _ := strconv.ParseFloat(match[3], 64)
	units := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour}
	offset := time.Duration(amount * float64(units[match[4]]))
	if match[2] == "-" {
		offset = -offset
	}
	return base.Add(offset), nil
}

func convertParam(p Param, now time.Time) (any, error) {
	value := strings.TrimSpace(p.Value)
	switch inferParamType(value) {
	case "int":
		return strconv.ParseInt(value, 10, 64)
	case "float":
		return strconv.ParseFloat(value, 64)
	case "bool":
		return strconv.ParseBool(strings.ToLower(value))
	case "timestamp":
		return resolveTime(value, now)
	default:
		return unquote(value), nil
	}
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func skipQuoted(sql string, i int, quote byte) int {
	for j := i + 1; j < len(sql); j++ {
		if sql[j] == quote {
			if j+1 < len(sql) && sql[j+1] == quote {
				j++
				continue
			}
			return j + 1
		}
	}
	return len(sql)
}

func skipDollarQuoted(sql string, i int) (int, bool) {
	end := strings.IndexByte(sql[i+1:], '$')
	if end < 0 {
		return i, false
	}
	tag := sql[i : i+end+2]
	for _, c := range []byte(tag[1 : len(tag)-1]) {
		if !isIdentPart(c) {
			return i, false
		}
	}
	closing := strings.Index(sql[i+len(tag):], tag)
	if closing < 0 {
		return len(sql), true
	}
	return i + len(tag) + closing + len(tag), true
}

func renderSQL(sql string, params []Param, overrides map[string]string, now time.Time) (string, []any, error) {
	byName := map[string]Param{}
	for _, p := range params {
		byName[strings.ToLower(p.Name)] = p
	}
	for name, value := range overrides {
		key := strings.ToLower(strings.TrimPrefix(name, "@"))
		p, ok := byName[key]
		if !ok {
			return "", nil, fmt.Errorf("@%s no está definido como parámetro", strings.TrimPrefix(name, "@"))
		}
		p.Value = value
		byName[key] = p
	}

	var out strings.Builder
	positions := map[string]int{}
	var args []any
	for i := 0; i < len(sql); {
		c := sql[i]
		switch {
		case c == '\'' || c == '"':
			end := skipQuoted(sql, i, c)
			out.WriteString(sql[i:end])
			i = end
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			end := strings.IndexByte(sql[i:], '\n')
			if end < 0 {
				end = len(sql) - i
			}
			out.WriteString(sql[i : i+end])
			i += end
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				end = len(sql) - i - 2
			} else {
				end += 2
			}
			out.WriteString(sql[i : i+2+end])
			i += 2 + end
		case c == '$':
			if end, ok := skipDollarQuoted(sql, i); ok {
				out.WriteString(sql[i:end])
				i = end
			} else {
				out.WriteByte(c)
				i++
			}
		case c == '@' && i+1 < len(sql) && isIdentStart(sql[i+1]) && (i == 0 || !isIdentPart(sql[i-1])):
			j := i + 1
			for j < len(sql) && isIdentPart(sql[j]) {
				j++
			}
			name := sql[i+1 : j]
			p, ok := byName[strings.ToLower(name)]
			if !ok {
				return "", nil, fmt.Errorf("el SQL usa @%s pero no está definido como parámetro", name)
			}
			if j < len(sql) && sql[j] == '.' {
				out.WriteString(pgx.Identifier{unquote(strings.TrimSpace(p.Value))}.Sanitize())
			} else if position, seen := positions[strings.ToLower(name)]; seen {
				out.WriteString("$" + strconv.Itoa(position))
			} else {
				value, err := convertParam(p, now)
				if err != nil {
					return "", nil, fmt.Errorf("@%s: %w", name, err)
				}
				args = append(args, value)
				positions[strings.ToLower(name)] = len(args)
				out.WriteString("$" + strconv.Itoa(len(args)))
			}
			i = j
		default:
			out.WriteByte(c)
			i++
		}
	}
	return out.String(), args, nil
}
