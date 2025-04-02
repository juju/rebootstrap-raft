package main

import (
	"os"
	"strings"
	"testing"
)

type dirEntry struct {
	name  string
	isDir bool
}

func populateAgentDir(tempDir string, dirEntries []dirEntry) (string, error) {
	var err error
	for _, entry := range dirEntries {
		if entry.isDir {
			err = os.Mkdir(tempDir+"/"+entry.name, 0755)
		} else {
			_, err = os.Create(tempDir + "/" + entry.name)
		}
		if err != nil {
			break
		}
	}
	return tempDir, err
}

func TestMissingAgentDirectory(t *testing.T) {
	agentDir, err := populateAgentDir(t.TempDir(), []dirEntry{})
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s", err)
	}
	command := &rebootstrapCommand{}
	// In order to test whether we catch a missing directory, we create a valid agentDir
	// and then reference a missing directory inside it.
	_, err = command.getMachineID(agentDir + "/missing")
	if !strings.Contains(err.Error(), "no such file or directory") {
		t.Errorf("Expected an error for missing agent directory, got %s", err)
	}
}

func TestGetID(t *testing.T) {
	agentDir, err := populateAgentDir(t.TempDir(), []dirEntry{
		{name: "machine-0", isDir: true},
	})
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s", err)
	}
	command := &rebootstrapCommand{}
	machineID, err := command.getMachineID(agentDir)
	if err != nil {
		t.Errorf("Expected no error, got %s", err)
	}
	if machineID != "0" {
		t.Errorf("Expected machine ID to be '0', got %s", machineID)
	}
}

func TestMultipleMachineIDs(t *testing.T) {
	agentDir, err := populateAgentDir(t.TempDir(), []dirEntry{
		{name: "machine-0", isDir: true},
		{name: "machine-1", isDir: true},
	})
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s", err)
	}
	command := &rebootstrapCommand{}
	machineID, err := command.getMachineID(agentDir)
	if err == nil {
		t.Errorf("Expected an error for multiple machine IDs, got %s", machineID)
	}
	if err.Error() != "expected exactly one machine ID, got 2" {
		t.Errorf("Expected error 'multiple machine IDs found', got %s", err)
	}
}

func TestGetIDWithFile(t *testing.T) {
	agentDir, err := populateAgentDir(t.TempDir(), []dirEntry{
		{name: "machine-0", isDir: true},
		{name: "machine-1", isDir: false},
	})
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s", err)
	}
	command := &rebootstrapCommand{}
	machineID, err := command.getMachineID(agentDir)
	if err != nil {
		t.Errorf("Expected no error, got %s", err)
	}
	if machineID != "0" {
		t.Errorf("Expected machine ID to be '0', got %s", machineID)
	}
}
