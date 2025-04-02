// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	"github.com/hashicorp/raft-boltdb"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v2"
	"github.com/juju/replicaset"
	"gopkg.in/yaml.v3"
)

const rebootstrapDoc = `

Recreate an empty raft cluster directory with server configuration
based on the current replicaset members. This should be run as root on
a controller machine while the machine agent is stopped. For safety it
won't run if there's a raft directory already.

You can determine the password for Juju's MongoDB by looking in the
machine agent's configuration file using the following command:

     sudo grep statepassword /var/lib/juju/agents/machine-*/agent.conf  | cut -d' ' -f2

`

// jujuMachineKey is the key for the replset member tag where we
// store the member's corresponding machine id.
const jujuMachineKey = "juju-machine-id"

var logger = loggo.GetLogger("rebootstrap-raft")

type rebootstrapCommand struct {
	cmd.CommandBase
	verbose   bool
	dryRun    bool
	raftDir   string
	apiPort   int
	machineID string
	hostname  string
	mongoPort string
	ssl       bool
	password  string
}

// Info is part of cmd.Command.
func (c *rebootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "rebootstrap-raft",
		Args:    "--machine-id <id> --password <password>",
		Purpose: "Recreate a juju raft cluster directory.",
		Doc:     strings.TrimSpace(rebootstrapDoc),
	}
}

// SetFlags is part of cmd.Command.
func (c *rebootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.BoolVar(&c.verbose, "verbose", false, "show debug logging")
	f.BoolVar(&c.dryRun, "dry-run", false, "build the configuration but don't bootstrap raft")
	f.StringVar(&c.raftDir, "raft-dir", "/var/lib/juju/raft", "raft directory location")
	f.StringVar(&c.machineID, "machine-id", "", "ID of this Juju controller machine")
	f.IntVar(&c.apiPort, "api-port", 17070, "the API port of the Juju controller")
	f.StringVar(&c.hostname, "hostname", "localhost", "the hostname of the Juju MongoDB server")
	f.StringVar(&c.mongoPort, "mongo-port", "37017", "the port of the Juju MongoDB server")
	f.BoolVar(&c.ssl, "ssl", true, "use SSL to connect to MongoDB ")
	f.StringVar(&c.password, "password", "", "password for connecting to MongoDB")
}

// Init is part of cmd.Command.
func (c *rebootstrapCommand) Init(args []string) error {
	if c.verbose || c.dryRun {
		logger.SetLogLevel(loggo.DEBUG)
	}
	return c.CommandBase.Init(args)
}

// Run is part of cmd.Command.
func (c *rebootstrapCommand) Run(ctx *cmd.Context) error {
	_, err := os.Stat(c.raftDir)
	if err == nil && !c.dryRun {
		return errors.Errorf("raft directory %q already exists - remove it first to show your commitment", c.raftDir)
	}

	if c.machineID == "" {
		c.machineID, err = c.getMachineID("/var/lib/juju/agents")
		if err != nil {
			return errors.Annotate(err, "getting machine IDs")
		}
		logger.Infof("Got machine ID: %s", c.machineID)
	}

	if c.password == "" {
		c.password, err = c.getStatePassword(c.machineID, "/var/lib/juju/agents")
		if err != nil {
			return errors.Annotate(err, "getting state password")
		}
		logger.Infof("Got state password")
	}

	members, err := c.getReplicaSetMembers()
	if err != nil {
		return errors.Annotate(err, "getting replica set members")
	}
	logger.Infof("Got replica set members.")

	raftServers, err := makeRaftServers(members, c.apiPort)
	if err != nil {
		return errors.Annotate(err, "constructing raft server configuration")
	}
	logger.Infof("Raft server info:")
	for _, server := range raftServers.Servers {
		logger.Infof("%#v", server)
	}

	if c.dryRun {
		logger.Infof("dry-run specified - stopping")
		return nil
	}
	return errors.Trace(c.bootstrapRaft(raftServers))
}

func (c *rebootstrapCommand) getStatePassword(machineID string, agentDirectory string) (string, error) {
	agentConfigPath := fmt.Sprintf(agentDirectory + "/machine-%s/agent.conf", machineID)
	file, err := os.Open(agentConfigPath)
	if err != nil {
		return "", errors.Annotatef(err, "opening agent config file %q", agentConfigPath)
	}
	defer file.Close()
	password, err := c.extractStatePassword(file)
	if err != nil {
		return "", errors.Annotatef(err, "extracting state password from %q", agentConfigPath)
	}
	return password, nil
}

func (c *rebootstrapCommand) extractStatePassword(file io.Reader) (string, error) {
	var config map[string]interface{}
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return "", errors.Annotatef(err, "decoding yaml")
	}

	password, ok := config["statepassword"].(string)
	if !ok || password == "" {
		return "", errors.Errorf("statepassword not found")
	}

	return password, nil
}

func (c *rebootstrapCommand) getMachineID(agentDirectory string) (string, error) {
	entries, err := os.ReadDir(agentDirectory)
	if err != nil {
		return "", errors.Annotatef(err, "reading directory %q", agentDirectory)
	}
	var machineIDs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), "machine-") {
			machineIDString := strings.TrimPrefix(entry.Name(), "machine-")
			if _, err := strconv.Atoi(machineIDString); err == nil {
				machineIDs = append(machineIDs, machineIDString)
			}
		}
	}
	if len(machineIDs) != 1 {
		return "", errors.Errorf("expected exactly one machine ID, got %d", len(machineIDs))
	}
	return machineIDs[0], nil
}

func (c *rebootstrapCommand) getReplicaSetMembers() ([]replicaset.Member, error) {
	session, err := c.dial()
	if err != nil {
		return nil, errors.Annotate(err, "connecting to MongoDB")
	}
	defer session.Close()
	return replicaset.CurrentMembers(session)
}

func (c *rebootstrapCommand) bootstrapRaft(servers raft.Configuration) error {
	_, transport := raft.NewInmemTransport(raft.ServerAddress("notused"))
	defer transport.Close()

	logStore, err := NewLogStore(c.raftDir)
	if err != nil {
		return errors.Annotate(err, "making log store")
	}

	snapshotStore, err := NewSnapshotStore(c.raftDir, 2)
	if err != nil {
		return errors.Annotate(err, "making snapshot store")
	}

	config, err := makeRaftConfig(c.machineID)
	if err != nil {
		return errors.Annotate(err, "making raft config")
	}

	err = raft.BootstrapCluster(config, logStore, logStore, snapshotStore, transport, servers)

	if err != nil {
		return errors.Annotate(err, "bootstrapping raft cluster")
	}
	logger.Infof("Raft cluster store bootstrapped in %q.", c.raftDir)
	return nil
}

func makeRaftConfig(machineID string) (*raft.Config, error) {
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(machineID)
	// Having ShutdownOnRemove true means that the raft node also
	// stops when it's demoted if it's the leader.
	raftConfig.ShutdownOnRemove = false

	appLogger := hclog.New(&hclog.LoggerOptions{
		Name:  "rebootstrap-raft",
		Level: hclog.LevelFromString("DEBUG"),
	})
	raftConfig.Logger = appLogger

	if err := raft.ValidateConfig(raftConfig); err != nil {
		return nil, errors.Annotate(err, "validating raft config")
	}
	return raftConfig, nil
}

func makeRaftServers(members []replicaset.Member, apiPort int) (raft.Configuration, error) {
	var empty raft.Configuration
	var servers []raft.Server
	for _, member := range members {
		id, ok := member.Tags[jujuMachineKey]
		if !ok {
			return empty, errors.NotFoundf("juju machine id for replset member %d", member.Id)
		}
		baseAddress, _, err := net.SplitHostPort(member.Address)
		if err != nil {
			return empty, errors.Annotatef(err, "getting base address for replset member %d", member.Id)
		}
		apiAddress := net.JoinHostPort(baseAddress, strconv.Itoa(apiPort))
		suffrage := raft.Voter
		if member.Votes != nil && *member.Votes < 1 {
			suffrage = raft.Nonvoter
		}
		server := raft.Server{
			ID:       raft.ServerID(id),
			Address:  raft.ServerAddress(apiAddress),
			Suffrage: suffrage,
		}
		servers = append(servers, server)
	}
	return raft.Configuration{Servers: servers}, nil
}

// NewLogStore opens a boltDB logstore in the specified directory. If
// the directory doesn't already exist it'll be created.
func NewLogStore(dir string) (*raftboltdb.BoltStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.Trace(err)
	}
	logs, err := raftboltdb.New(raftboltdb.Options{
		Path: filepath.Join(dir, "logs"),
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to create bolt store for raft logs")
	}
	return logs, nil
}

// NewSnapshotStore opens a file-based snapshot store in the specified
// directory. If the directory doesn't exist it'll be created.
func NewSnapshotStore(
	dir string,
	retain int,
) (raft.SnapshotStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.Trace(err)
	}
	appLogger := hclog.New(&hclog.LoggerOptions{
		Name:  "rebootstrap-raft",
		Level: hclog.LevelFromString("DEBUG"),
	})

	snaps, err := raft.NewFileSnapshotStoreWithLogger(dir, retain, appLogger)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create file snapshot store")
	}
	return snaps, nil
}

func (c *rebootstrapCommand) dial() (*mgo.Session, error) {
	info := &mgo.DialInfo{
		Addrs:    []string{net.JoinHostPort(c.hostname, c.mongoPort)},
		Database: "admin",
		Username: fmt.Sprintf("machine-%s", c.machineID),
		Password: c.password,
	}
	if c.ssl {
		info.DialServer = dialSSL
	}
	session, err := mgo.DialWithInfo(info)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func dialSSL(addr *mgo.ServerAddr) (net.Conn, error) {
	c, err := net.Dial("tcp", addr.String())
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	cc := tls.Client(c, tlsConfig)
	if err := cc.Handshake(); err != nil {
		return nil, err
	}
	return cc, nil
}

func runCommand(args []string) int {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		logger.Errorf("creating context: %v", err)
		return 2
	}
	return cmd.Main(&rebootstrapCommand{}, ctx, args)
}

func main() {
	os.Exit(runCommand(os.Args[1:]))
}
