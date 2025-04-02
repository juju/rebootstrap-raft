package main

import (
	"testing"
)

func TestGetJujuPathAsSnap(t *testing.T) {
	command := &rebootstrapCommand{
		asSnap: true,
	}
	result := command.getJujuPath("somepath", true)
	expected := "/var/lib/snapd/hostfs/somepath"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestGetJujuPathNotAsSnap(t *testing.T) {
	command := &rebootstrapCommand{
		asSnap: false,
	}
	result := command.getJujuPath("/otherpath", true)
	expected2 := "/otherpath"
	if result != expected2 {
		t.Errorf("expected %s, got %s", expected2, result)
	}
}

func TestGetJujuPathExtraSlash(t *testing.T) {
	command := &rebootstrapCommand{
		asSnap: true,
	}
	result := command.getJujuPath("///extra/slash", true)
	expected := "/var/lib/snapd/hostfs/extra/slash"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
	result = command.getJujuPath("///extra/slash", false)
	expected = "/extra/slash"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}
