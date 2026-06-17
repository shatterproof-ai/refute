package openrewrite

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/symbol"
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
		{
			name: "javadoc mentioning class before declaration",
			src:  "package com.example;\n\n/**\n * This class does X and is a useful enum-like thing.\n */\npublic class Greeter {}",
			want: "Greeter",
		},
		{
			name: "line comment mentioning class before declaration",
			src:  "// the class below is the interface entry point\npublic class Entry {}",
			want: "Entry",
		},
		{
			name: "string literal mentioning class before declaration",
			src:  "public class Holder {\n    String s = \"class Imposter {\";\n}",
			want: "Holder",
		},
		{
			name: "block comment with declaration keyword only in comment",
			src:  "/* class Commented */\nfinal class Real {}",
			want: "Real",
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

func TestJavaTypeFQN_IgnoresJavadocPoison(t *testing.T) {
	// A Javadoc comment naming a different "class" word before the real
	// declaration must not poison the resolved FQN (would yield a recipe that
	// matches nothing — a silent no-op).
	src := "package com.example;\n\n/**\n * This class does interesting work.\n */\npublic class Greeter {\n    public String greet(String n) { return n; }\n}\n"
	f := writeTempJava(t, src)
	got, err := javaTypeFQN(f)
	if err != nil {
		t.Fatalf("javaTypeFQN: %v", err)
	}
	if got != "com.example.Greeter" {
		t.Errorf("javaTypeFQN: got %q, want com.example.Greeter", got)
	}
}

func TestStripCommentsAndStrings(t *testing.T) {
	// stripCommentsAndStrings must preserve byte offsets (so a later line scan
	// stays aligned), keep real code verbatim, and blank every comment/string
	// span. Asserting exact spacing is brittle, so check those properties plus a
	// newline-preservation invariant.
	cases := []struct {
		name         string
		src          string
		mustContain  []string // real code that must survive verbatim
		mustNotMatch []string // tokens that only appear inside comments/strings
	}{
		{
			name:         "line comment blanked, newline preserved",
			src:          "a // class X\nb",
			mustContain:  []string{"a ", "\nb"},
			mustNotMatch: []string{"class X"},
		},
		{
			name:         "block comment blanked",
			src:          "a /* class X */ b",
			mustContain:  []string{"a ", " b"},
			mustNotMatch: []string{"class X"},
		},
		{
			name:         "slash inside string is not a comment",
			src:          `x = "a // b /* c";`,
			mustContain:  []string{"x = ", ";"},
			mustNotMatch: []string{"//", "/*"},
		},
		{
			name:         "escaped quote does not end string early",
			src:          `s = "he said \"hi\"" + class;`,
			mustContain:  []string{"s = ", " + class;"},
			mustNotMatch: []string{"he said", "hi"},
		},
		{
			name:         "quote inside line comment is inert",
			src:          `a // "unterminated`,
			mustContain:  []string{"a "},
			mustNotMatch: []string{"unterminated"},
		},
		{
			name:         "unterminated block comment to EOF",
			src:          "code /* tail",
			mustContain:  []string{"code "},
			mustNotMatch: []string{"tail"},
		},
		{
			name:        "char literal with escaped quote then real code",
			src:         `c = '\''; class Real`,
			mustContain: []string{"c = ", "; class Real"},
		},
		{
			name:        "division operator preserved",
			src:         "z = a / b;",
			mustContain: []string{"z = a / b;"},
		},
		{
			name:         "real code after a string is preserved",
			src:          `t = "class Imposter"; class Real {`,
			mustContain:  []string{"t = ", "; class Real {"},
			mustNotMatch: []string{"Imposter"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripCommentsAndStrings(tc.src)
			if len(got) != len(tc.src) {
				t.Fatalf("length changed: got %d, want %d (offsets must be preserved)\n%q", len(got), len(tc.src), got)
			}
			if strings.Count(got, "\n") != strings.Count(tc.src, "\n") {
				t.Errorf("newline count changed: %q", got)
			}
			for _, want := range tc.mustContain {
				if !strings.Contains(got, want) {
					t.Errorf("stripped output %q missing real code %q", got, want)
				}
			}
			for _, poison := range tc.mustNotMatch {
				if strings.Contains(got, poison) {
					t.Errorf("stripped output %q still contains comment/string token %q", got, poison)
				}
			}
		})
	}
}

func TestResolveJar(t *testing.T) {
	t.Run("explicit path that exists is used", func(t *testing.T) {
		jar := filepath.Join(t.TempDir(), "adapter.jar")
		if err := os.WriteFile(jar, []byte("jar"), 0o644); err != nil {
			t.Fatal(err)
		}
		a := &Adapter{jarPath: jar}
		got, err := a.resolveJar(t.TempDir())
		if err != nil {
			t.Fatalf("resolveJar: %v", err)
		}
		if got != jar {
			t.Errorf("resolveJar = %q, want %q", got, jar)
		}
	})

	t.Run("explicit path that is missing names the path", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nope.jar")
		a := &Adapter{jarPath: missing}
		_, err := a.resolveJar(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), missing) {
			t.Fatalf("error = %v, want mention of %q", err, missing)
		}
	})

	t.Run("env override works regardless of go.mod", func(t *testing.T) {
		jar := filepath.Join(t.TempDir(), "env-adapter.jar")
		if err := os.WriteFile(jar, []byte("jar"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(jarEnvVar, jar)
		a := &Adapter{}
		// A workspace with no refute go.mod above it still resolves via the env.
		got, err := a.resolveJar(t.TempDir())
		if err != nil {
			t.Fatalf("resolveJar: %v", err)
		}
		if got != jar {
			t.Errorf("resolveJar = %q, want %q", got, jar)
		}
	})

	t.Run("env override missing names the path and var", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing-env.jar")
		t.Setenv(jarEnvVar, missing)
		a := &Adapter{}
		_, err := a.resolveJar(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), missing) || !strings.Contains(err.Error(), jarEnvVar) {
			t.Fatalf("error = %v, want mention of %q and %q", err, missing, jarEnvVar)
		}
	})
}

func TestBuildRenameParams_RejectsKotlin(t *testing.T) {
	a := &Adapter{workspaceRoot: "/ws"}
	for _, ext := range []string{".kt", ".kts"} {
		t.Run(ext, func(t *testing.T) {
			_, err := a.buildRenameParams(symbol.Location{File: "/ws/Foo" + ext, Name: "foo", Kind: symbol.KindMethod}, "bar")
			if err == nil {
				t.Fatal("expected an error for a Kotlin file, got nil")
			}
			if !errors.Is(err, backend.ErrUnsupported) {
				t.Errorf("error %v is not backend.ErrUnsupported", err)
			}
		})
	}
}
