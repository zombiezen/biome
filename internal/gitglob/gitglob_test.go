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

package gitglob

import "testing"

var parseLineTests = []struct {
	line          string
	want          string
	negate        bool
	directoryOnly bool
	matches       []string
	doesNotMatch  []string
}{
	{line: ``, want: ``},
	{line: ` `, want: ``},
	{line: "\x80", want: ``},
	{line: `# comment`, want: ``},
	{line: `foo.txt`, want: `(^|.*/)foo\.txt$`},
	{line: `\#file`, want: `(^|.*/)#file$`},
	{line: ` foo`, want: `(^|.*/) foo$`},
	{line: `foo `, want: `(^|.*/)foo$`},
	{line: `foo\ `, want: `(^|.*/)foo $`},
	{line: `foo\  `, want: `(^|.*/)foo $`},
	{line: `!foo.txt`, want: `(^|.*/)foo\.txt$`, negate: true},
	{line: `\!foo.txt`, want: `(^|.*/)!foo\.txt$`},
	{line: `\!foo.txt`, want: `(^|.*/)!foo\.txt$`},
	{line: `foo/bar`, want: `^foo/bar$`},
	{line: `/foo/bar`, want: `^foo/bar$`},
	{line: `/foo`, want: `^foo$`},
	{line: `foo/`, want: `(^|.*/)foo$`, directoryOnly: true},
	{line: `doc/frotz/`, want: `^doc/frotz$`, directoryOnly: true},
	{line: `*.txt`, want: `(^|.*/)[^/]*\.txt$`}, // match leading dot
	{line: `a*.txt`, want: `(^|.*/)a[^/]*\.txt$`},
	{line: `.*.txt`, want: `(^|.*/)\.[^/]*\.txt$`},
	{line: `foo?`, want: `(^|.*/)foo[^/]$`},
	{line: `foo[a-zA-Z]`, want: `(^|.*/)foo[a-zA-Z]$`},
	{line: `*foo[/]`, want: ``},
	{line: `foo\[a-zA-Z]`, want: `(^|.*/)foo\[a-zA-Z\]$`},
	{line: `foo[!a-zA-Z]`, want: `(^|.*/)foo[^a-zA-Z/]$`},
	{
		line: `foo[][!]`,
		want: `(^|.*/)foo[\][!]$`,
		matches: []string{
			"foo]",
			"foo[",
			"foo!",
		},
	},
	{line: `foo[]-]`, want: `(^|.*/)foo[\]\-]$`},
	{
		line: `foo[--0]`,
		want: `(^|.*/)foo[-.0]$`,
		matches: []string{
			"foo-",
			"foo.",
			"foo0",
		},
	},
	{
		line: `foo[!--0]bar`,
		want: `(^|.*/)foo[^--0/]bar$`,
		matches: []string{
			"fooxbar",
		},
		doesNotMatch: []string{
			"foo-bar",
			"foo.bar",
			"foo/bar",
			"foo0bar",
		},
	},
	{line: `foo[a-]`, want: `(^|.*/)foo[a\-]$`},
	{line: `foo[c-a]`, want: ``},
	{line: `foo[[?*\]`, want: `(^|.*/)foo[\[?*\\]$`},
	{line: `**/foo`, want: `(^|.*/)foo$`},
	{line: `/**/foo`, want: `(^|.*/)foo$`},
	{line: `**/foo/bar`, want: `(^|.*/)foo/bar$`},
	{line: `abc/**`, want: `^abc/`},
	{line: `a/**/b`, want: `^a/(|.+/)b$`},
	{line: `abc/d**`, want: `^abc/d[^/]*[^/]*$`},
}

func TestParseLine(t *testing.T) {
	for _, test := range parseLineTests {
		gotPattern := ParseLine(test.line)
		got := ""
		if gotPattern.re != nil {
			got = gotPattern.re.String()
		}
		if got != test.want || gotPattern.negate != test.negate || gotPattern.directoryOnly != test.directoryOnly {
			t.Errorf(
				"ParseLine(%q) = {re:%q negate:%t directoryOnly:%t}; "+
					"want {re:%q negate:%t directoryOnly:%t}",
				test.line, got, gotPattern.negate, gotPattern.directoryOnly,
				test.want, test.negate, test.directoryOnly,
			)
		}
		for _, m := range test.matches {
			if !gotPattern.Match(m, 0) {
				t.Errorf("ParseLine(%q).Match(%q, 0) = false; want true", test.line, m)
			}
		}
		for _, m := range test.doesNotMatch {
			if gotPattern.Match(m, 0) {
				t.Errorf("ParseLine(%q).Match(%q, 0) = true; want false", test.line, m)
			}
		}
	}
}
