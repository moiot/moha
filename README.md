# mysql-agent

[中文文档](README-cn.md)

### What is mysql-agent?

mysql-agent provides the high availability(HA) for cross-region MySQL clusters, 
by automating master failover and fast master switch.

- __High Availability__

    Master failure detection and automating master failover enables master switch in a few seconds.
    
- __Brain Split Omission__

    By configuring proper lease ttl, mysql-agent enables at most one master in the cluster at any time.
    
- __Cross AZ Topology__

    Leveraging etcd as the service discovery, mysql-agent prevent its implementation from VIP,
    so that the cross-AZ MySQL cluster is able to be build via mysql-agent.

- __Manual Master/Slave Switch__

    Besides the automated failover as described below, mysql-agent provides the ability to switch master manually.


### Quick Start


#### Development Environment Setup

```
make init
```
make init  does the following
- check and install `dep`
- check and install `golint`
- check and install `goimports`
- add git commit hook
- prepare testing framework

now you can do development on your local machine


#### Local Machine Running Environment Setup

```bash
make env-up
```

#### Create Local Machine Running Agents, AKA Test Code on Local Machine
```bash
make docker-agent
make start-agents
```

#### Local Machine Running Environment Destroy

```bash
make clean-data
```

### Roadmap
[Roadmap](docs/roadmap.md)


### License
This project is under the Apache 2.0 license. See the [LICENSE](LICENSE) file for details.

### Acknowledgments
* Thanks [rxi](https://github.com/rxi) for the lightweight log framework
* Thanks [juju/errors](https://github.com/juju/errors) for the error handling framework
