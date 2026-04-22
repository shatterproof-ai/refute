package openrewrite

import (
	"os"
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

func TestJavaTypeFQN(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "packaged class",
			content: "package com.example;\npublic class Greeter {}",
			want:    "com.example.Greeter",
		},
		{
			name:    "default package",
			content: "public class Greeter {}",
			want:    "Greeter",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := writeTempJava(t, tc.content)
			got, err := javaTypeFQN(f)
			if err != nil {
				t.Fatalf("javaTypeFQN: %v", err)
			}
			if got != tc.want {
				t.Errorf("javaTypeFQN: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestJavaMethodPatternPrefix(t *testing.T) {
	content := "package com.example;\npublic class Greeter { public String greet(String n) { return n; } }"
	f := writeTempJava(t, content)
	got, err := javaMethodPatternPrefix(f, "greet")
	if err != nil {
		t.Fatalf("javaMethodPatternPrefix: %v", err)
	}
	const want = "com.example.Greeter greet"
	if got != want {
		t.Errorf("javaMethodPatternPrefix: got %q, want %q", got, want)
	}
}

func writeTempJava(t *testing.T, content string) string {
	t.Helper()
	f := t.TempDir() + "/Tmp.java"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return f
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
