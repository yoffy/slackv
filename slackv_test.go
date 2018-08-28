package main

import "net/http"
import "testing"

func TestErrorEquals(t *testing.T) {
	_, err1 := http.Get("https://test.example.com/api/rtm.start")
	_, err2 := http.Get("https://test.example.com/api/rtm.start")
	if !errorEquals(err1, err2) {
		t.Errorf("err1=%s err2=%s\n", err1.Error(), err2.Error())
	}
}

func TestUnescape1(t *testing.T) {
	g_IdNameMap = map[string]string{"G01234": "test_group"}
	expected := "#test_group foo"
	result := unescape("<#G01234|test_group> foo")
	if result != expected {
		t.Errorf("expected \"%s\", but \"%s\"\n", expected, result)
	}
	result = unescape("<#G01234> foo")
	if result != expected {
		t.Errorf("expected \"%s\", but \"%s\"\n", expected, result)
	}
}

func TestUnescape2(t *testing.T) {
	g_IdNameMap = map[string]string{"U01234": "test_user"}
	expected := "@test_user foo"
	result := unescape("<@U01234|test_user> foo")
	if result != expected {
		t.Errorf("expected \"%s\", but \"%s\"\n", expected, result)
	}
	result = unescape("<@U01234> foo")
	if result != expected {
		t.Errorf("expected \"%s\", but \"%s\"\n", expected, result)
	}
}

func TestUnescape3(t *testing.T) {
	g_IdNameMap = map[string]string{}
	expected := "@here foo"
	result := unescape("<!here|here> foo")
	if result != expected {
		t.Errorf("expected \"%s\", but \"%s\"\n", expected, result)
	}
	result = unescape("<!here> foo")
	if result != expected {
		t.Errorf("expected \"%s\", but \"%s\"\n", expected, result)
	}
}

func TestUnescape4(t *testing.T) {
	g_IdNameMap = map[string]string{"S1A2B3C4D": "hoge-piyo"}
	expected := "@hoge-piyo foo"
	result := unescape("<!subteam^S1A2B3C4D|@hoge-piyo> foo")
	if result != expected {
		t.Errorf("expected \"%s\", but \"%s\"\n", expected, result)
	}
	result = unescape("@hoge-piyo foo")
	if result != expected {
		t.Errorf("expected \"%s\", but \"%s\"\n", expected, result)
	}
}
