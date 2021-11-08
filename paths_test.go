// Copyright 2021 Ross Light
// Copyright 2020 YourBase Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package biome

import "testing"

func TestJoinPath(t *testing.T) {
	tests := []struct {
		elem []string
		os   string
		want string
	}{
		{elem: []string{}, os: Linux, want: ""},
		{elem: []string{"", ""}, os: Linux, want: ""},

		{elem: []string{"a", "b", "c"}, os: Linux, want: "a/b/c"},
		{elem: []string{"a", "b/c"}, os: Linux, want: "a/b/c"},
		{elem: []string{"a", ""}, os: Linux, want: "a"},
		{elem: []string{"", "a"}, os: Linux, want: "a"},

		{elem: []string{"a", "b", "c"}, os: Windows, want: `a\b\c`},
		{elem: []string{"a", "b\\c"}, os: Windows, want: `a\b\c`},
		{elem: []string{"a", ""}, os: Windows, want: "a"},
		{elem: []string{"", "a"}, os: Windows, want: "a"},

		{elem: []string{"a", "b/c"}, os: Windows, want: `a\b\c`},
	}
	for _, test := range tests {
		got := JoinPath(&Descriptor{OS: test.os}, test.elem...)
		if got != test.want {
			t.Errorf("JoinPath({OS: %q}, %q...) = %q; want %q", test.os, test.elem, got, test.want)
		}
	}
}

func TestCleanPath(t *testing.T) {
	tests := []struct {
		path string
		os   string
		want string
	}{
		{path: "", os: Linux, want: "."},
		{path: "a/c", os: Linux, want: "a/c"},
		{path: "a//c", os: Linux, want: "a/c"},
		{path: "a/c/.", os: Linux, want: "a/c"},
		{path: "a/c/b/..", os: Linux, want: "a/c"},
		{path: "/../a/c", os: Linux, want: "/a/c"},
		{path: "/../a/b/../././/c", os: Linux, want: "/a/c"},
		{path: "", os: Linux, want: "."},

		{path: `a\c`, os: Windows, want: `a\c`},
		{path: `a\\c`, os: Windows, want: `a\c`},
		{path: `a\c\.`, os: Windows, want: `a\c`},
		{path: `a\c\b\..`, os: Windows, want: `a\c`},
		{path: `\..\a\c`, os: Windows, want: `\a\c`},
		{path: `\..\a\b\..\.\.\\c`, os: Windows, want: `\a\c`},
		{path: "", os: Windows, want: "."},

		{path: "a/c", os: Windows, want: `a\c`},
		{path: "a//c", os: Windows, want: `a\c`},
		{path: "a/c/.", os: Windows, want: `a\c`},
		{path: "a/c/b/..", os: Windows, want: `a\c`},
		{path: "/../a/c", os: Windows, want: `\a\c`},
		{path: "/../a/b/../././/c", os: Windows, want: `\a\c`},
	}
	for _, test := range tests {
		got := CleanPath(&Descriptor{OS: test.os}, test.path)
		if got != test.want {
			t.Errorf("CleanPath({OS: %q}, %q) = %q; want %q", test.os, test.path, got, test.want)
		}
	}
}
