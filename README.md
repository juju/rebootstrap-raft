# Overview

`rebootstrap-raft` creates an empty raft cluster directory with server
configuration based on the current replicaset members. It's intended
as an emergency utility to resolve situations where the raft store has
been corrupted somehow.

For safety it won't overwrite raft data - any existing raft directory
`/var/lib/juju/raft` will need to be removed or renamed. The new raft
directory won't have any lease information in it, but if the juju
controller is part of an HA cluster and the other nodes still have
lease information it will be replicated to this one when the
controller agent is started.

# Installing

```
go get github.com/juju/rebootstrap-raft/...
```

# Running

Copy the `rebootstrap-raft` binary to the controller machine where you
need to recreate the raft directory. You'll need the machine's MongoDB
password - you can get it from the agent config file by running on the
machine:

```
sudo grep statepassword /var/lib/juju/agents/machine-*/agent.conf  | cut -d' ' -f2
```

Stop the controller agent by running:
```
sudo systemctl stop jujud-machine-<id>.service
```

Move the existing raft directory out of the way, then run:

```
sudo rebootstrap-raft --machine-id <id> --password <mongo-password>
```

Then restart the controller agent:

```
sudo systemctl start jujud-machine-<id>.service
```
