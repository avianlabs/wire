// Copyright 2018 The Wire Authors
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
)

type Foo struct {
	n int
}

type FooBar struct {
	foo *Foo
}

type FooBaz struct {
	foo *Foo
}

var fooCalls int

func provideFoo() *Foo {
	fooCalls++
	return &Foo{n: 42}
}

func provideFooBar(foo *Foo) *FooBar {
	return &FooBar{foo: foo}
}

func provideFooBaz(foo *Foo) *FooBaz {
	return &FooBaz{foo: foo}
}

func main() {
	bar := injectFooBar()
	baz := injectFooBaz()
	fmt.Println(fooCalls)
	fmt.Println(bar.foo == baz.foo)
	fmt.Println(bar.foo.n)
}
