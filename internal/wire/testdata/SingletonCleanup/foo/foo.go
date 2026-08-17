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

type Bar struct {
	foo *Foo
}

var fooCalls int

func provideFoo() (*Foo, func()) {
	fooCalls++
	foo := &Foo{n: fooCalls}
	return foo, func() { fmt.Println("cleanup foo", foo.n) }
}

func provideBar(foo *Foo) *Bar {
	return &Bar{foo: foo}
}

func main() {
	bar1, release1 := injectBar()
	bar2, release2 := injectBar()
	fmt.Println(fooCalls)
	fmt.Println(bar1.foo == bar2.foo)
	release1()
	fmt.Println("released one")
	release2()
	bar3, release3 := injectBar()
	fmt.Println(fooCalls)
	fmt.Println(bar3.foo.n)
	release3()
}
