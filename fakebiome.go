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

import (
	"context"
	"fmt"
)

// Fake is a biome that operates in-memory. It uses POSIX-style paths, but
// permits any character to be used as the separator.
type Fake struct {
	// Descriptor is the descriptor that will be returned by Describe.
	Descriptor Descriptor

	// DirsResult is what will be returned by Dirs.
	DirsResult Dirs

	// RunFunc is called to handle the Run method.
	RunFunc func(context.Context, *Invocation) error
}

// Describe returns f.Descriptor.
func (f *Fake) Describe() *Descriptor {
	return &f.Descriptor
}

// Dirs returns f.DirsResult.
func (f *Fake) Dirs() *Dirs {
	return &f.DirsResult
}

// Run calls f.RunFunc. It returns an error if f.RunFunc is nil.
func (f *Fake) Run(ctx context.Context, invoke *Invocation) error {
	if f.RunFunc == nil {
		return fmt.Errorf("fake run: RunFunc not set")
	}
	return f.RunFunc(ctx, invoke)
}

// Close does nothing and returns nil.
func (f *Fake) Close() error {
	return nil
}
