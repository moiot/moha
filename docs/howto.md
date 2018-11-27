### HowTo

#### Debug

Setup `etcd` environment

```
$ make up
Creating network "dockercompose_default" with the default driver
Creating dockercompose_etcd1_1 ... done
Creating dockercompose_etcd0_1 ... done
Creating dockercompose_etcd2_1 ... done
$ docker ps
CONTAINER ID        IMAGE                 COMMAND                  CREATED             STATUS              PORTS                               NAMES
1cb642a22e3c        quay.io/coreos/etcd   "/usr/local/bin/etcd…"   12 seconds ago      Up 15 seconds       2380/tcp, 0.0.0.0:32773->2379/tcp   dockercompose_etcd0_1
1c6ed8524f5f        quay.io/coreos/etcd   "/usr/local/bin/etcd…"   12 seconds ago      Up 15 seconds       2380/tcp, 0.0.0.0:32771->2379/tcp   dockercompose_etcd2_1
2c9d7a485321        quay.io/coreos/etcd   "/usr/local/bin/etcd…"   12 seconds ago      Up 16 seconds       2380/tcp, 0.0.0.0:32772->2379/tcp   dockercompose_etcd1_1
```

Start process

```
./bin/mysql-agent -etcd-urls "http://127.0.0.1:32771/,http://127.0.0.1:32772/,http://127.0.0.1:32773/"
```

Node information change

Suppose `Node ID` is `mock-node`, data in etcd should like this.

```
$ etcdctl --endpoints="127.0.0.1:32771" get --prefix mysql-servers
mysql-servers/mock-node/alive
{"File":"","Pos":"","GTID":""}
mysql-servers/mock-node/object
{"NodeID":"mock-node","Host":"test","IsAlive":false,"LatestPos":{"File":"","Pos":"","GTID":""}}
```

Node in etcd after process exit.

```
$ etcdctl --endpoints="127.0.0.1:32771" get --prefix mysql-servers
mysql-servers/offline/mock-node
{"File":"","Pos":"","GTID":""}
```