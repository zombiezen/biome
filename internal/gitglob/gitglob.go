// Copyright 2021 Ross Light
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//		 https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

/*
Package gitglob provides functions to match paths using gitignore-style patterns.
See https://git-scm.com/docs/gitignore#_pattern_format for syntax.
*/
package gitglob

import (
	"io/fs"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Pattern is the representation of a compiled glob pattern. A Pattern is safe
// for concurrent use by multiple goroutines. The zero value is a Pattern that
// does not match any paths.
type Pattern struct {
	re            *regexp.Regexp
	line          string
	negate        bool
	directoryOnly bool
}

// ParseLine compiles a single pattern.
func ParseLine(line string) Pattern {
	if !utf8.ValidString(line) {
		return Pattern{}
	}
	if strings.HasPrefix(line, "#") {
		return Pattern{}
	}
	line = trimRight(line)
	if line == "" {
		// Blank.
		return Pattern{}
	}
	orig := line
	if strings.HasPrefix(line, `\#`) {
		line = line[1:]
	}
	negate := false
	if strings.HasPrefix(line, "!") {
		negate = true
		line = line[1:]
	} else if strings.HasPrefix(line, `\!`) {
		line = line[1:]
	}
	rooted := strings.HasPrefix(line, "/")
	if rooted {
		line = line[1:]
	}
	directoryOnly := strings.HasSuffix(line, "/")
	if directoryOnly {
		line = line[:len(line)-1]
	}

	tokens := lexPattern(line)
	if len(tokens) == 0 {
		// Syntax error.
		return Pattern{}
	}
	// Determine whether we are actually rooted.
	if len(tokens) > 0 && tokens[0].typ == doubleStar {
		rooted = false
		tokens = tokens[1:]
	} else if !rooted {
		// Search for a separator in the middle of the pattern.
		for _, tok := range tokens {
			if tok.typ == literal && strings.Contains(tok.s, "/") {
				rooted = true
				break
			}
		}
	}
	isPrefix := len(tokens) > 0 && tokens[len(tokens)-1].typ == doubleStar
	if isPrefix {
		tokens = tokens[:len(tokens)-1]
	}
	re := new(strings.Builder)
	if rooted {
		re.WriteString("^")
	} else {
		re.WriteString("(^|.*/)")
	}
	for _, tok := range tokens {
		switch tok.typ {
		case literal:
			re.WriteString(regexp.QuoteMeta(tok.s))
		case star:
			re.WriteString(`[^/]*`)
		case questionMark:
			re.WriteString(`[^/]`)
		case characterClass:
			if !convertCharacterClass(re, tok.s) {
				// Syntax error in character class.
				return Pattern{}
			}
		case doubleStar:
			re.WriteString(`(|.+/)`)
		default:
			panic("unhandled pattern element")
		}
	}
	if !isPrefix {
		re.WriteString(`$`)
	}
	return Pattern{
		re:            regexp.MustCompile(re.String()),
		line:          orig,
		negate:        negate,
		directoryOnly: directoryOnly,
	}
}

// convertCharacterClass converts a glob character class
// into an RE2 character class and appends to re.
//
// It returns false if the character class contains any syntax errors.
func convertCharacterClass(re *strings.Builder, cc string) bool {
	l := lexer{s: cc[1 : len(cc)-1]}
	re.WriteString(`[`)
	negated := l.consume('!')
	if negated {
		re.WriteString(`^`)
	}
	prev := utf8.RuneError
	switch c := l.peek(); c {
	case '^', '[':
		re.WriteByte('\\')
		re.WriteRune(c)
		l.next()
		prev = c
	case '-':
		re.WriteString(`-`)
		l.next()
		prev = c
	}
	for {
		c, ok := l.next()
		if !ok {
			break
		}
		switch c {
		case '\\', ']':
			re.WriteByte('\\')
			re.WriteRune(c)
			prev = c
		case '-':
			end, ok := l.next()
			if !ok {
				// A trailing hyphen represents itself.
				re.WriteString(`\-`)
				break
			}
			if prev == utf8.RuneError || end == utf8.RuneError || prev > end {
				return false
			}
			if !negated && prev <= '/' && '/' <= end {
				// Expand range to exclude slash.
				for c := prev + 1; c <= end; c++ {
					switch c {
					case '/':
						// skip
					case '\\', ']', '-':
						re.WriteByte('\\')
						fallthrough
					default:
						re.WriteRune(c)
					}
				}
			} else {
				re.WriteByte('-')
				re.WriteRune(end)
			}
		default:
			re.WriteRune(c)
			prev = c
		}
	}
	if negated {
		re.WriteString(`/`)
	}
	re.WriteString(`]`)
	return true
}

// Match reports whether the given slash-separated path matches the pattern.
// If io/fs.ValidPath reports false, then Match will report false.
func (pat Pattern) Match(path string, mode fs.FileMode) bool {
	return pat.re != nil &&
		(mode.IsDir() || !pat.directoryOnly) &&
		fs.ValidPath(path) &&
		pat.re.MatchString(path)
}

// IsNegated reports whether the pattern starts with an exclamation point ('!').
// In gitignore for example, such a pattern indicates any matching file excluded
// by a previous pattern will become included again.
func (pat Pattern) IsNegated() bool {
	return pat.negate
}

// String returns the string passed to ParseLine with any trailing space removed.
func (pat Pattern) String() string {
	return pat.line
}

const (
	literal = iota
	star
	doubleStar
	questionMark
	characterClass
)

type token struct {
	typ int
	s   string
}

func appendLiteral(tokens []token, lit string) []token {
	if lit != "" {
		tokens = append(tokens, token{literal, lit})
	}
	return tokens
}

func lexPattern(pat string) []token {
	l := &lexer{s: pat}
	var tokens []token
	for literalStart := 0; ; {
		// Start of path component.
		if before := l.pos; l.consumeString("**/") {
			tokens = appendLiteral(tokens, pat[literalStart:before])
			tokens = append(tokens, token{doubleStar, "**/"})
			literalStart = l.pos
			continue
		}
		if l.remaining() == "**" {
			// Trailing double asterisk.
			tokens = appendLiteral(tokens, pat[literalStart:l.pos])
			tokens = append(tokens, token{doubleStar, "**"})
			return tokens
		}

		// Path component is not a double-asterisk: interpret character by character.
	component:
		for {
			before := l.pos
			c, ok := l.next()
			if !ok {
				tokens = appendLiteral(tokens, pat[literalStart:])
				return tokens
			}
			switch c {
			case '/':
				break component
			case '*':
				tokens = appendLiteral(tokens, pat[literalStart:before])
				tokens = append(tokens, token{star, "*"})
				literalStart = l.pos
			case '?':
				tokens = appendLiteral(tokens, pat[literalStart:before])
				tokens = append(tokens, token{questionMark, "?"})
				literalStart = l.pos
			case '[':
				tokens = appendLiteral(tokens, pat[literalStart:before])
				l.consume('!')
				for first := true; ; first = false {
					c, ok := l.next()
					if !ok || c == '/' {
						// Unterminated character class.
						return nil
					}
					if !first && c == ']' {
						break
					}
				}
				tokens = append(tokens, token{characterClass, pat[before:l.pos]})
				literalStart = l.pos
			case '\\':
				tokens = appendLiteral(tokens, pat[literalStart:before])
				literalStart = l.pos
				c, ok := l.next()
				if !ok {
					// Backslash at end of pattern. Use literally.
					tokens = append(tokens, token{literal, `\`})
					return tokens
				}
				if c == '/' {
					break component
				}
			}
		}
	}
}

type lexer struct {
	s   string
	pos int
}

func (l *lexer) eof() bool {
	return l.pos >= len(l.s)
}

func (l *lexer) remaining() string {
	if l.eof() {
		return ""
	}
	return l.s[l.pos:]
}

func (l *lexer) peek() rune {
	c, _ := utf8.DecodeRuneInString(l.remaining())
	return c
}

func (l *lexer) next() (_ rune, ok bool) {
	c, size := utf8.DecodeRuneInString(l.remaining())
	l.pos += size
	return c, size > 0
}

func (l *lexer) consume(want rune) bool {
	return l.consumeAny(string(want))
}

func (l *lexer) consumeAny(want string) bool {
	got, size := utf8.DecodeRuneInString(l.remaining())
	if !strings.ContainsRune(want, got) {
		return false
	}
	l.pos += size
	return true
}

func (l *lexer) consumeString(want string) bool {
	if !strings.HasPrefix(l.remaining(), want) {
		return false
	}
	l.pos += len(want)
	return true
}

func trimRight(s string) string {
	for end, prevEnd := len(s), len(s); end > 0; {
		c, size := utf8.DecodeLastRuneInString(s[:end])
		if c == '\\' {
			return s[:prevEnd]
		}
		if !unicode.IsSpace(c) {
			return s[:end]
		}
		prevEnd = end
		end -= size
	}
	return ""
}
