# 本地开发

#### 开发环境配置
首先执行 `make init`，在本地安装依赖
会做下面几件事
- check and install `dep`
- check and install `golint`
- check and install `goimports`
- add git commit hook
- prepare testing framework

现在开发环境就配置好了。

#### 本地测试环境搭建

```bash
make env-up
```

会在本地启动一个三节点的 etcd 集群，并开启鉴权，默认用户名/密码为 `root`/`root`

#### 在本地运行修改后的代码
```bash
make reload-agents
```

#### 清除本地环境的数据并停止所有相关实例
```bash
make clean-data
```

#### 在本地进行单元测试
```bash
make test
```

#### 在本地进行 e2e 测试
```bash
make integration-test
```

#### 打包镜像
如果想在本地进行镜像打包，可以参考下面步骤。

**第一次**打包镜像的时候需要执行下面命令，配置基础镜像
```bash
docker pull gcc:8.1.0
docker pull golang:1.11.0
docker pull quay.io/coreos/etcd:v3.3.2
docker build etc/etcd-image/v3.3.2/ -t moiot/etcd:v3.3.2
docker build etc/mysql-image/5.7.22-pmm/ -t moiot/mysql:5.7.22-pmm
```

之后可以执行 `make release` 查看指导

执行 `python release.py {version}` 就会在本地的 repo 打一个名字为 "{version}" 的 tag 并推送到 git 仓库。
如果 repo 是 gitlab 的话会执行 CI 来 build 镜像并 push。详情可见 [.gitlab-ci.yml](../.gitlab-ci.yml)
否则需要手工执行
```
    # 编译
    make docker-agent
    # build 镜像
    docker build -t $docker_image ./etc/docker-compose/agent
    # 上传镜像至仓库
    docker push $docker_image
    # clean up
    docker image rm $docker_image
    docker image prune -f
```