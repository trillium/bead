package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestCreateInvocationBodyOnStdin(t *testing.T) {
	argv, stdin, has := createInvocation([]string{"create", "t"}, "line1\nline2\n\n", true)
	want := []string{"create", "t", "--body-file", "-", "--dry-run"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %v, want %v", argv, want)
	}
	if !has || stdin != "line1\nline2\n" {
		t.Fatalf("stdin = %q has=%v", stdin, has)
	}
}

func TestCreateInvocationNoBody(t *testing.T) {
	in := []string{"create", "t"}
	argv, stdin, has := createInvocation(in, "  \n", false)
	if has || stdin != "" || !reflect.DeepEqual(argv, []string{"create", "t"}) {
		t.Fatalf("got %v %q %v", argv, stdin, has)
	}
	if len(in) != 2 {
		t.Fatal("input args mutated")
	}
}

func TestParseStoresJSON(t *testing.T) {
	got, err := parseStoresJSON([]byte("notice\n[{\"name\":\"scratch\",\"about\":\"a\",\"path\":\"/p\"},{\"name\":\"\"}]"))
	if err != nil || len(got) != 1 || got[0] != (store{Name: "scratch", Path: "/p", About: "a"}) {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := parseStoresJSON([]byte("oops")); err == nil {
		t.Fatal("want error without array")
	}
}

func TestChooseStores(t *testing.T) {
	remote := func() ([]store, error) { return []store{{Name: "r"}}, nil }
	local := func() ([]store, error) { return []store{{Name: "l"}}, nil }
	bad := func() ([]store, error) { return nil, errors.New("ssh down") }
	empty := func() ([]store, error) { return nil, nil }
	cases := []struct {
		name   string
		client bool
		fetch  func() ([]store, error)
		want   string
	}{
		{"client uses remote", true, remote, "r"},
		{"host uses local", false, remote, "l"},
		{"client falls back on error", true, bad, "l"},
		{"client falls back on empty", true, empty, "l"},
	}
	for _, c := range cases {
		got, err := chooseStores(c.client, c.fetch, local)
		if err != nil || got[0].Name != c.want {
			t.Errorf("%s: got %v %v", c.name, got, err)
		}
	}
}
