package openrewrite

import (
	"testing"
)

func TestParseJavaPackage(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "standard package",
			src:  "package com.example;\n\npublic class Foo {}",
			want: "com.example",
		},
		{
			name: "no package",
			src:  "public class Foo {}",
			want: "",
		},
		{
			name: "package with leading whitespace",
			src:  "  package org.test.pkg;\n\nclass Bar {}",
			want: "org.test.pkg",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseJavaPackage(tc.src)
			if got != tc.want {
				t.Errorf("parseJavaPackage: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseJavaClass(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "public class",
			src:  "package com.example;\npublic class Greeter {\n    public String greet(String name) { return name; }\n}",
			want: "Greeter",
		},
		{
			name: "interface",
			src:  "public interface Nameable {}",
			want: "Nameable",
		},
		{
			name: "enum",
			src:  "public enum Color { RED, GREEN, BLUE }",
			want: "Color",
		},
		{
			name: "generic class",
			src:  "public class Box<T> {}",
			want: "Box",
		},
		{
			name: "class with extends",
			src:  "public class Foo extends Bar {}",
			want: "Foo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseJavaClass(tc.src)
			if got != tc.want {
				t.Errorf("parseJavaClass: got %q, want %q", got, tc.want)
			}
		})
	}
}
