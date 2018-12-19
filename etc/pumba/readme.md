# What is Pumba?

Pumba is for doing the Chaos Monkey Testing，mainly on network partitioning simulation



### How to build 

```bash
# create required folder
cd $GOPATH
mkdir -p src/github.com/alexei-led && cd src/github.com/alexei-led

# clone pumba
git clone git@github.com:alexei-led/pumba.git
cd pumba 

# build docker image
docker build -t pumba -f Dockerfile .

```

### Run through Docker Image
```bash
docker run -v /var/run/docker.sock:/var/run/docker.sock pumba pumba  netem -d 1m delay --time 3000 re2:^mysql
```
