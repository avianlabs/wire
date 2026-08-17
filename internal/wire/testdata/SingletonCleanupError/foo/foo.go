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
	"errors"
	"fmt"
)

type Config struct {
	n int
}

type Foo struct {
	n int
}

type Bar struct {
	foo *Foo
}

type Baz struct {
	n int
}

var fooCalls, bazCalls int

func provideFoo(cfg Config, opts ...string) (*Foo, func(), error) {
	fooCalls++
	foo := &Foo{n: cfg.n}
	fmt.Println("opts", len(opts))
	return foo, func() { fmt.Println("cleanup foo", foo.n) }, nil
}

func provideBar(foo *Foo) *Bar {
	return &Bar{foo: foo}
}

func provideBaz() (*Baz, func(), error) {
	bazCalls++
	return nil, nil, errors.New("baz boom")
}

func main() {
	bar1, release1, err := injectBar(Config{n: 1}, "a", "b")
	fmt.Println(err)
	bar2, release2, _ := injectBar(Config{n: 2})
	fmt.Println(fooCalls)
	fmt.Println(bar1.foo == bar2.foo)
	fmt.Println(bar1.foo.n)
	release1()
	release2()
	bar3, release3, _ := injectBar(Config{n: 3})
	fmt.Println(fooCalls)
	fmt.Println(bar3.foo.n)
	release3()

	_, _, err1 := injectBaz()
	_, _, err2 := injectBaz()
	fmt.Println(err1)
	fmt.Println(err2)
	fmt.Println(bazCalls)
}
