package main

import (
	"os"
	"strings"
	"testing"
)

func TestGetStatePasswordMissingFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-agents-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)
	command := &rebootstrapCommand{}
	_, err = command.getStatePassword("0", tempDir+"/missing")
	if err == nil {
		t.Errorf("Expected 'file not found' error")
	}
	if !strings.Contains(err.Error(), "no such file") {
		t.Errorf("Expected file not found error, got %v", err)
	}
}

func TestGetStatePassword(t *testing.T) {
	statefile := strings.NewReader(`# format 2.0
tag: machine-0
datadir: /var/lib/juju
transient-datadir: /var/run/juju
logdir: /var/log/juju
metricsspooldir: /var/lib/juju/metricspool
nonce: user-admin:bootstrap
jobs:
- JobManageModel
- JobHostUnits
upgradedToVersion: 2.9.51
statepassword: qKoU8Y/BRQd0shDFA4OdHxlf
controller: controller-e5e3d855-9df1-4e24-8010-0bfa6bc3cbf1
model: model-a03dcd4f-afaf-4c96-8965-f2537e3b4873
apiaddresses:
- 10.1.20.117:17070
- '[fd42::216:3eff:fe69:bb55]:17070'
apipassword: qKoU8Y/BRQd0shDFA4OdHxlf
oldpassword: 8e2e2bdb1e690dcee14c381bbe92baf5
loggingconfig: <root>=INFO; unit=DEBUG
values:
  AGENT_SERVICE_NAME: jujud-machine-0
  CONTAINER_TYPE: ""
  NUMA_CTL_PREFERENCE: "false"
  PROVIDER_TYPE: lxd
agent-logfile-max-size: 100
agent-logfile-max-backups: 2
apiport: 17070
stateport: 37017
mongoversion: 4.4.24/wiredTiger
mongomemoryprofile: default
juju-db-snap-channel: 4.4/stable`)
	command := &rebootstrapCommand{}
	password, err := command.extractStatePassword(statefile)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if password != "qKoU8Y/BRQd0shDFA4OdHxlf" {
		t.Errorf("Expected empty password, got %s", password)
	}
}
