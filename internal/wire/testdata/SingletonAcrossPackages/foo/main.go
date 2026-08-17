// Copyright 2026 The Wire Authors
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

package main

import (
	"fmt"

	"example.com/bar"
	"example.com/inj1"
	"example.com/inj2"
)

func main() {
	f1 := inj1.Foo()
	f2 := inj2.Foo()
	fmt.Println("same foo:", f1 == f2)
	fmt.Println("foo calls:", f1.Calls)
	c1 := inj1.Client(bar.Config{Prefix: "first"})
	c2 := inj2.Client(bar.Config{Prefix: "second"})
	fmt.Println("same client:", c1 == c2)
	fmt.Println("client prefix:", c1.Prefix)
}
