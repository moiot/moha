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