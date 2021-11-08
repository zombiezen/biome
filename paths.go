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

package biome

import (
	slashpath "path"

	"zombiezen.com/go/biome/internal/windowspath"
)

// JoinPath joins any number of path elements into a single path.
// The result is cleaned as if by path/filepath.Clean, however, if the
// argument list is empty or all its elements are empty, JoinPath
// returns an empty string.
func JoinPath(desc *Descriptor, elem ...string) string {
	if desc.OS == Windows {
		return windowspath.Join(elem...)
	}
	return slashpath.Join(elem...)
}

// IsAbsPath reports whether the path is absolute.
func IsAbsPath(desc *Descriptor, path string) bool {
	if desc.OS == Windows {
		return windowspath.IsAbs(path)
	}
	return slashpath.IsAbs(path)
}

// CleanPath returns the shortest path name equivalent to path by purely
// lexical processing. It uses the same algorithm as path/filepath.Clean.
func CleanPath(desc *Descriptor, path string) string {
	if path == "" {
		// JoinPath will return an empty string, which does not match Clean.
		return "."
	}
	return JoinPath(desc, path)
}

// AbsPath returns an absolute representation of path. If the path is not absolute
// it will be joined with the biome's working directory to turn it into an absolute
// path. The absolute path name for a given file is not guaranteed to be unique.
// AbsPath calls Clean on the result.
func AbsPath(bio Biome, path string) string {
	desc := bio.Describe()
	if IsAbsPath(desc, path) {
		return CleanPath(desc, path)
	}
	return JoinPath(desc, bio.Dirs().Work, path)
}

// FromSlash returns the result of replacing each slash ('/') character in path
// with a separator character. Multiple slashes are replaced by multiple separators.
func FromSlash(desc *Descriptor, path string) string {
	switch desc.OS {
	case Windows:
		return windowspath.FromSlash(path)
	default:
		return path
	}
}
